package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jankremlacek/monitor/internal/config"
	"github.com/jankremlacek/monitor/internal/db"
)

// Server is the web backend.
type Server struct {
	db            *db.DB
	config        *config.WebConfig
	server        *http.Server
	watcherClient *http.Client
}

// NewServer creates a new web server.
func NewServer(database *db.DB, cfg *config.WebConfig) (*Server, error) {
	s := &Server{
		db:            database,
		config:        cfg,
		watcherClient: &http.Client{Timeout: 30 * time.Second},
	}
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: s.routes(),
	}
	return s, nil
}

// callWatcher makes a request to the watcher API.
func (s *Server) callWatcher(ctx context.Context, method, path string) (*http.Response, error) {
	url := s.config.WatcherURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	return s.watcherClient.Do(req)
}

// Run starts the web server.
func (s *Server) Run(ctx context.Context) error {
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
