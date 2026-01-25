package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jandubois/monitor/internal/db"
)

// Event types for watcher events.
const (
	EventRegistered             = "registered"
	EventTokenChanged           = "token_changed"
	EventConnectionLost         = "connection_lost"
	EventConnectionRestored     = "connection_restored"
	EventProbeVersionUpgrade    = "probe_version_upgrade"
	EventProbeVersionDowngrade  = "probe_version_downgrade"
	EventWatcherVersionChanged  = "watcher_version_changed"
)

// Event severities.
const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// WatcherEvent represents an event in the watcher_events table.
type WatcherEvent struct {
	ID             int            `json:"id"`
	WatcherID      int            `json:"watcher_id"`
	WatcherName    string         `json:"watcher_name,omitempty"`
	Timestamp      string         `json:"timestamp"`
	EventType      string         `json:"event_type"`
	Severity       string         `json:"severity"`
	Summary        string         `json:"summary"`
	Details        map[string]any `json:"details,omitempty"`
	Acknowledged   bool           `json:"acknowledged"`
	AcknowledgedAt *string        `json:"acknowledged_at,omitempty"`
}

// insertWatcherEvent inserts a new watcher event into the database.
func (s *Server) insertWatcherEvent(ctx context.Context, watcherID int, eventType, severity, summary string, details map[string]any) error {
	now := time.Now().UTC().Format(db.SQLiteTimeFormat)

	var detailsJSON *string
	if details != nil {
		b, _ := json.Marshal(details)
		s := string(b)
		detailsJSON = &s
	}

	_, err := s.db.DB().ExecContext(ctx, `
		INSERT INTO watcher_events (watcher_id, timestamp, event_type, severity, summary, details)
		VALUES (?, ?, ?, ?, ?, ?)
	`, watcherID, now, eventType, severity, summary, detailsJSON)

	if err != nil {
		slog.Error("failed to insert watcher event", "event_type", eventType, "watcher_id", watcherID, "error", err)
		return err
	}

	slog.Info("watcher event created", "event_type", eventType, "severity", severity, "watcher_id", watcherID)
	return nil
}

// hasRecentConnectionLostEvent checks if there's an unacknowledged connection_lost event
// for this watcher without a subsequent connection_restored event.
func (s *Server) hasRecentConnectionLostEvent(ctx context.Context, watcherID int) bool {
	var count int
	err := s.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM watcher_events
		WHERE watcher_id = ?
		  AND event_type = ?
		  AND timestamp > COALESCE(
		      (SELECT MAX(timestamp) FROM watcher_events
		       WHERE watcher_id = ? AND event_type = ?),
		      '1970-01-01'
		  )
	`, watcherID, EventConnectionLost, watcherID, EventConnectionRestored).Scan(&count)

	return err == nil && count > 0
}

// monitorWatcherConnections runs a background goroutine to detect stale watchers.
func (s *Server) monitorWatcherConnections(ctx context.Context, threshold time.Duration) {
	ticker := time.NewTicker(threshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkStaleWatchers(ctx, threshold)
		}
	}
}

// checkStaleWatchers checks for watchers that haven't sent a heartbeat recently.
func (s *Server) checkStaleWatchers(ctx context.Context, threshold time.Duration) {
	cutoff := time.Now().Add(-threshold).UTC().Format(db.SQLiteTimeFormat)

	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT w.id, w.name, w.last_seen_at
		FROM watchers w
		WHERE w.approved = 1
		  AND w.last_seen_at < ?
		  AND NOT EXISTS (
		      SELECT 1 FROM watcher_events we
		      WHERE we.watcher_id = w.id
		        AND we.event_type = ?
		        AND we.timestamp > COALESCE(
		            (SELECT MAX(timestamp) FROM watcher_events
		             WHERE watcher_id = w.id AND event_type = ?),
		            '1970-01-01'
		        )
		  )
	`, cutoff, EventConnectionLost, EventConnectionRestored)
	if err != nil {
		slog.Error("failed to check stale watchers", "error", err)
		return
	}

	// Collect results first to avoid SQLite locking issues during iteration
	type staleWatcher struct {
		id         int
		name       string
		lastSeenAt string
	}
	var staleWatchers []staleWatcher
	for rows.Next() {
		var sw staleWatcher
		if err := rows.Scan(&sw.id, &sw.name, &sw.lastSeenAt); err != nil {
			continue
		}
		staleWatchers = append(staleWatchers, sw)
	}
	rows.Close()

	// Now insert events after the result set is closed
	for _, sw := range staleWatchers {
		_ = s.insertWatcherEvent(ctx, sw.id, EventConnectionLost, SeverityError,
			"No heartbeat received",
			map[string]any{
				"last_seen_at": sw.lastSeenAt,
				"threshold":    threshold.String(),
			})
	}
}
