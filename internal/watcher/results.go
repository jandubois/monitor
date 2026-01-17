package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/jankremlacek/monitor/internal/db"
	"github.com/jankremlacek/monitor/internal/notify"
	"github.com/jankremlacek/monitor/internal/probe"
)

// DBResultWriter persists probe results to PostgreSQL and triggers notifications.
type DBResultWriter struct {
	db         *db.DB
	dispatcher *notify.Dispatcher
}

// NewDBResultWriter creates a new database result writer.
func NewDBResultWriter(database *db.DB, dispatcher *notify.Dispatcher) *DBResultWriter {
	return &DBResultWriter{
		db:         database,
		dispatcher: dispatcher,
	}
}

// WriteResult stores a probe result and triggers notifications on status change.
func (w *DBResultWriter) WriteResult(ctx context.Context, cfg *ProbeConfig, result *probe.Result, scheduledAt, executedAt time.Time, durationMs int) error {
	// Get previous status before writing new result
	prevStatus, _ := w.getPreviousStatus(ctx, cfg.ID)

	// Write result
	_, err := w.db.Pool().Exec(ctx, `
		INSERT INTO probe_results (probe_config_id, status, message, metrics, data, duration_ms, scheduled_at, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, cfg.ID, result.Status, result.Message, result.Metrics, result.Data, durationMs, scheduledAt, executedAt)
	if err != nil {
		return err
	}

	// Check for status change and notify
	if w.dispatcher != nil && len(cfg.NotificationChannels) > 0 && prevStatus != result.Status {
		slog.Info("status change detected",
			"probe", cfg.Name,
			"old_status", prevStatus,
			"new_status", result.Status,
		)
		w.dispatcher.NotifyStatusChange(ctx, cfg.NotificationChannels, &notify.StatusChange{
			ProbeName: cfg.Name,
			OldStatus: prevStatus,
			NewStatus: result.Status,
			Message:   result.Message,
		})
	}

	return nil
}

func (w *DBResultWriter) getPreviousStatus(ctx context.Context, configID int) (probe.Status, error) {
	var status string
	err := w.db.Pool().QueryRow(ctx, `
		SELECT status FROM probe_results
		WHERE probe_config_id = $1
		ORDER BY executed_at DESC
		LIMIT 1
	`, configID).Scan(&status)
	if err != nil {
		return "", err
	}
	return probe.Status(status), nil
}
