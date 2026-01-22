package web

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is a type for context keys used in this package.
type contextKey string

const (
	// watcherIDKey is the context key for the authenticated watcher's ID.
	watcherIDKey contextKey = "watcherID"
	// watcherNameKey is the context key for the authenticated watcher's name.
	watcherNameKey contextKey = "watcherName"
)

// WatcherIDFromContext returns the watcher ID from the request context.
func WatcherIDFromContext(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(watcherIDKey).(int)
	return id, ok
}

// WatcherNameFromContext returns the watcher name from the request context.
func WatcherNameFromContext(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(watcherNameKey).(string)
	return name, ok
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token != s.config.AuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireWatcherAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Look up watcher by token
		var watcherID int
		var watcherName string
		var approved int
		err := s.db.DB().QueryRowContext(r.Context(),
			`SELECT id, name, approved FROM watchers WHERE token = ?`, token,
		).Scan(&watcherID, &watcherName, &approved)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if watcher is approved
		if approved == 0 {
			http.Error(w, "watcher not approved", http.StatusForbidden)
			return
		}

		// Store watcher info in context
		ctx := context.WithValue(r.Context(), watcherIDKey, watcherID)
		ctx = context.WithValue(ctx, watcherNameKey, watcherName)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
