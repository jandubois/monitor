package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jankremlacek/monitor/internal/config"
)

const Version = "1.0.0"

// Watcher schedules and executes probes.
type Watcher struct {
	config    *config.WatcherConfig
	client    *Client
	discovery *Discovery

	scheduler *Scheduler
	executor  *Executor

	mu       sync.Mutex
	shutdown bool
}

// New creates a new Watcher instance.
func New(cfg *config.WatcherConfig) (*Watcher, error) {
	client := NewClient(cfg.PushURL, cfg.AuthToken)
	executor := NewExecutor(cfg.MaxConcurrent, cfg.ProbesDir)
	executor.SetResultWriter(NewHTTPResultWriter(client, cfg.Name))
	scheduler := NewScheduler(client, executor, cfg.Name)
	discovery := NewDiscovery(cfg.ProbesDir)

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
	// Discover probes
	probeTypes, err := w.discovery.DiscoverAll(ctx)
	if err != nil {
		slog.Warn("probe discovery failed", "error", err)
	} else {
		slog.Info("probe discovery complete", "count", len(probeTypes))
	}

	// Register with web service
	regReq := &RegisterRequest{
		Name:       w.config.Name,
		Version:    Version,
		ProbeTypes: probeTypes,
	}
	resp, err := w.client.Register(ctx, regReq)
	if err != nil {
		return fmt.Errorf("failed to register with web service: %w", err)
	}
	slog.Info("registered with web service", "watcher_id", resp.WatcherID, "probe_types", resp.RegisteredProbes)

	// Start heartbeat
	go w.heartbeatLoop(ctx)

	// Start scheduler
	go w.scheduler.Run(ctx)

	// Start API server (minimal, for debugging)
	server := w.createAPIServer()
	serverErr := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf(":%d", w.config.APIPort)
		slog.Info("watcher API listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

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

	mux.HandleFunc("GET /health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("POST /reload", func(rw http.ResponseWriter, r *http.Request) {
		if err := w.scheduler.Reload(r.Context()); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(`{"status":"reloaded"}`))
	})

	mux.HandleFunc("POST /discover", func(rw http.ResponseWriter, r *http.Request) {
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
		fmt.Fprintf(rw, `{"status":"discovered","count":%d}`, len(probeTypes))
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.APIPort),
		Handler: mux,
	}
}
