package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jankremlacek/monitor/internal/config"
	"github.com/jankremlacek/monitor/internal/db"
	"github.com/jankremlacek/monitor/internal/notify"
)

// Watcher schedules and executes probes.
type Watcher struct {
	db         *db.DB
	config     *config.WatcherConfig
	dispatcher *notify.Dispatcher

	scheduler *Scheduler
	executor  *Executor

	mu       sync.Mutex
	shutdown bool
}

// New creates a new Watcher instance.
func New(database *db.DB, cfg *config.WatcherConfig) (*Watcher, error) {
	dispatcher := notify.NewDispatcher(database.Pool())
	executor := NewExecutor(cfg.MaxConcurrent, cfg.ProbesDir)
	executor.SetResultWriter(NewDBResultWriter(database, dispatcher))
	scheduler := NewScheduler(database, executor)

	return &Watcher{
		db:         database,
		config:     cfg,
		dispatcher: dispatcher,
		scheduler:  scheduler,
		executor:   executor,
	}, nil
}

// Run starts the watcher service.
func (w *Watcher) Run(ctx context.Context) error {
	// Load notification channels
	if err := w.dispatcher.LoadChannels(ctx); err != nil {
		slog.Warn("failed to load notification channels", "error", err)
	}

	// Start heartbeat
	go w.heartbeatLoop(ctx)

	// Start scheduler
	go w.scheduler.Run(ctx)

	// Start API server
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
	w.updateHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.updateHeartbeat(ctx)
		}
	}
}

func (w *Watcher) updateHeartbeat(ctx context.Context) {
	_, err := w.db.Pool().Exec(ctx, `
		INSERT INTO watcher_heartbeat (id, last_seen_at, watcher_version)
		VALUES (1, NOW(), $1)
		ON CONFLICT (id) DO UPDATE SET last_seen_at = NOW(), watcher_version = $1
	`, "1.0.0")
	if err != nil {
		slog.Error("failed to update heartbeat", "error", err)
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

	mux.HandleFunc("POST /trigger/{config_id}", func(rw http.ResponseWriter, r *http.Request) {
		configID := r.PathValue("config_id")
		if err := w.scheduler.TriggerImmediate(r.Context(), configID); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(`{"status":"triggered"}`))
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.APIPort),
		Handler: mux,
	}
}
