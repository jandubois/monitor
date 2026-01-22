package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jandubois/monitor/internal/config"
	"github.com/jandubois/monitor/internal/db"
	"github.com/jandubois/monitor/internal/notify"
)

// Server is the web backend.
type Server struct {
	db         *db.DB
	config     *config.WebConfig
	server     *http.Server
	dispatcher *notify.Dispatcher
}

// NewServer creates a new web server.
func NewServer(database *db.DB, cfg *config.WebConfig) (*Server, error) {
	dispatcher := notify.NewDispatcher(database.DB())

	s := &Server{
		db:         database,
		config:     cfg,
		dispatcher: dispatcher,
	}
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: s.routes(),
	}
	return s, nil
}

// Run starts the web server.
func (s *Server) Run(ctx context.Context) error {
	// Load notification channels
	if err := s.dispatcher.LoadChannels(ctx); err != nil {
		slog.Error("failed to load notification channels", "error", err)
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("web server listening", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down web server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Health check (no auth)
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Push API (used by watchers and external systems, with auth)
	mux.Handle("POST /api/push/register", s.requireAuth(http.HandlerFunc(s.handlePushRegister)))
	mux.Handle("POST /api/push/heartbeat", s.requireAuth(http.HandlerFunc(s.handlePushHeartbeat)))
	mux.Handle("POST /api/push/result", s.requireAuth(http.HandlerFunc(s.handlePushResult)))
	mux.Handle("POST /api/push/alert", s.requireAuth(http.HandlerFunc(s.handlePushAlert)))
	mux.Handle("GET /api/push/configs/{watcher}", s.requireAuth(http.HandlerFunc(s.handlePushGetConfigs)))

	// Watchers API
	mux.Handle("GET /api/watchers", s.requireAuth(http.HandlerFunc(s.handleListWatchers)))
	mux.Handle("GET /api/watchers/{id}", s.requireAuth(http.HandlerFunc(s.handleGetWatcher)))
	mux.Handle("DELETE /api/watchers/{id}", s.requireAuth(http.HandlerFunc(s.handleDeleteWatcher)))
	mux.Handle("PUT /api/watchers/{id}/paused", s.requireAuth(http.HandlerFunc(s.handleSetWatcherPaused)))

	// API routes (with auth)
	mux.Handle("GET /api/status", s.requireAuth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /api/probe-types", s.requireAuth(http.HandlerFunc(s.handleListProbeTypes)))
	mux.Handle("POST /api/probe-types/discover", s.requireAuth(http.HandlerFunc(s.handleDiscoverProbeTypes)))
	mux.Handle("GET /api/probe-configs", s.requireAuth(http.HandlerFunc(s.handleListProbeConfigs)))
	mux.Handle("POST /api/probe-configs", s.requireAuth(http.HandlerFunc(s.handleCreateProbeConfig)))
	mux.Handle("GET /api/probe-configs/{id}", s.requireAuth(http.HandlerFunc(s.handleGetProbeConfig)))
	mux.Handle("PUT /api/probe-configs/{id}", s.requireAuth(http.HandlerFunc(s.handleUpdateProbeConfig)))
	mux.Handle("DELETE /api/probe-configs/{id}", s.requireAuth(http.HandlerFunc(s.handleDeleteProbeConfig)))
	mux.Handle("POST /api/probe-configs/{id}/run", s.requireAuth(http.HandlerFunc(s.handleRunProbeConfig)))
	mux.Handle("PUT /api/probe-configs/{id}/enabled", s.requireAuth(http.HandlerFunc(s.handleSetProbeEnabled)))
	mux.Handle("GET /api/results", s.requireAuth(http.HandlerFunc(s.handleQueryResults)))
	mux.Handle("GET /api/results/{config_id}", s.requireAuth(http.HandlerFunc(s.handleGetResults)))
	mux.Handle("GET /api/results/stats", s.requireAuth(http.HandlerFunc(s.handleResultStats)))
	mux.Handle("GET /api/notification-channels", s.requireAuth(http.HandlerFunc(s.handleListNotificationChannels)))
	mux.Handle("POST /api/notification-channels", s.requireAuth(http.HandlerFunc(s.handleCreateNotificationChannel)))
	mux.Handle("PUT /api/notification-channels/{id}", s.requireAuth(http.HandlerFunc(s.handleUpdateNotificationChannel)))
	mux.Handle("DELETE /api/notification-channels/{id}", s.requireAuth(http.HandlerFunc(s.handleDeleteNotificationChannel)))
	mux.Handle("POST /api/notification-channels/{id}/test", s.requireAuth(http.HandlerFunc(s.handleTestNotificationChannel)))

	// Serve static files for everything else (React SPA)
	mux.Handle("/", staticHandler())

	return mux
}
