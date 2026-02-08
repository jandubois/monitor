package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jandubois/monitor/internal/config"
)

const Version = "1.0.0"

// Watcher schedules and executes probes.
type Watcher struct {
	config    *config.WatcherConfig
	client    *Client
	discovery *Discovery

	scheduler *Scheduler
	executor  *Executor
}

// New creates a new Watcher instance.
func New(cfg *config.WatcherConfig) (*Watcher, error) {
	client := NewClient(cfg.PushURL, cfg.AuthToken)
	executor := NewExecutor(cfg.MaxConcurrent, cfg.ProbesDir)
	executor.SetResultWriter(NewHTTPResultWriter(client, cfg.Name))
	scheduler := NewScheduler(client, executor, cfg.Name)
	discovery := NewDiscovery(cfg.ProbesDir, cfg.ExtraProbes)

	return &Watcher{
		config:    cfg,
		client:    client,
		discovery: discovery,
		scheduler: scheduler,
		executor:  executor,
	}, nil
}

// Run starts the watcher service.
func (w *Watcher) Run(ctx context.Context) error {
	// Start API server first (needed for callback URL verification during registration)
	addr := fmt.Sprintf(":%d", w.config.APIPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	slog.Info("watcher API listening", "addr", addr)

	server := w.createAPIServer()
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Discover probes
	probeTypes, err := w.discovery.DiscoverAll(ctx)
	if err != nil {
		slog.Warn("probe discovery failed", "error", err)
	} else {
		slog.Info("probe discovery complete", "count", len(probeTypes))
	}

	// Register with web service (retry for up to 2 minutes to allow for
	// macOS Local Network Privacy prompt)
	regReq := &RegisterRequest{
		Name:        w.config.Name,
		Version:     Version,
		Token:       w.config.AuthToken,
		CallbackURL: w.config.CallbackURL,
		ProbeTypes:  probeTypes,
	}
	var resp *RegisterResponse
	for attempt := 1; attempt <= 12; attempt++ {
		resp, err = w.client.Register(ctx, regReq)
		if err == nil {
			break
		}
		if attempt == 12 {
			return fmt.Errorf("failed to register with web service after 2 minutes: %w", err)
		}
		slog.Warn("registration failed, retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	if resp.Approved {
		slog.Info("registered with web service", "watcher_id", resp.WatcherID, "probe_types", resp.RegisteredProbes)
	} else {
		slog.Warn("registered with web service (pending approval)", "watcher_id", resp.WatcherID, "probe_types", resp.RegisteredProbes)
	}

	// Start heartbeat
	go w.heartbeatLoop(ctx)

	// Start scheduler
	go w.scheduler.Run(ctx)

	select {
	case <-ctx.Done():
		slog.Info("shutting down watcher")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}

func (w *Watcher) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initial heartbeat
	w.sendHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sendHeartbeat(ctx)
		}
	}
}

func (w *Watcher) sendHeartbeat(ctx context.Context) {
	err := w.client.Heartbeat(ctx, &HeartbeatRequest{
		Name:    w.config.Name,
		Version: Version,
	})
	if err != nil {
		slog.Error("failed to send heartbeat", "error", err)
	}
}

func (w *Watcher) createAPIServer() *http.Server {
	mux := http.NewServeMux()

	// Health endpoint is public (includes token hash for callback URL verification)
	mux.HandleFunc("GET /health", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		hash := sha256.Sum256([]byte(w.config.AuthToken))
		tokenHash := hex.EncodeToString(hash[:])
		resp := fmt.Sprintf(`{"status":"ok","name":%q,"token_hash":%q}`, w.config.Name, tokenHash)
		_, _ = rw.Write([]byte(resp))
	})

	// Protected endpoints require authentication
	mux.HandleFunc("POST /reload", w.requireAuth(func(rw http.ResponseWriter, r *http.Request) {
		if err := w.scheduler.Reload(r.Context()); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(`{"status":"reloaded"}`))
	}))

	mux.HandleFunc("POST /trigger/{id}", w.requireAuth(func(rw http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := w.scheduler.TriggerImmediate(r.Context(), id); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(`{"status":"triggered"}`))
	}))

	mux.HandleFunc("POST /discover", w.requireAuth(func(rw http.ResponseWriter, r *http.Request) {
		probeTypes, err := w.discovery.DiscoverAll(r.Context())
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// Re-register with web service
		regReq := &RegisterRequest{
			Name:       w.config.Name,
			Version:    Version,
			ProbeTypes: probeTypes,
		}
		if _, err := w.client.Register(r.Context(), regReq); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(rw, `{"status":"discovered","count":%d}`, len(probeTypes))
	}))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.APIPort),
		Handler: mux,
	}
}

// requireAuth wraps a handler to require Bearer token authentication.
func (w *Watcher) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(rw, "authorization required", http.StatusUnauthorized)
			return
		}

		const prefix = "Bearer "
		if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
			http.Error(rw, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		token := auth[len(prefix):]
		if token != w.config.AuthToken {
			http.Error(rw, "invalid token", http.StatusUnauthorized)
			return
		}

		next(rw, r)
	}
}
