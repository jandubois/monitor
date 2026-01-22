package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jandubois/monitor/internal/db"
	"github.com/jandubois/monitor/internal/notify"
	"github.com/jandubois/monitor/internal/probe"
)

// RegisterRequest is sent by watchers on startup.
type RegisterRequest struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	CallbackURL string              `json:"callback_url,omitempty"`
	ProbeTypes  []RegisterProbeType `json:"probe_types"`
}

// RegisterProbeType describes a probe type available on a watcher.
type RegisterProbeType struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description"`
	Arguments      map[string]any `json:"arguments"`
	ExecutablePath string         `json:"executable_path"`
	Subcommand     string         `json:"subcommand,omitempty"`
}

// HeartbeatRequest is sent periodically by watchers.
type HeartbeatRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ResultRequest is sent by watchers when a probe completes.
type ResultRequest struct {
	Watcher       string         `json:"watcher"`
	ProbeConfigID int            `json:"probe_config_id"`
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	Metrics       map[string]any `json:"metrics"`
	Data          map[string]any `json:"data"`
	DurationMs    int            `json:"duration_ms"`
	NextRun       string         `json:"next_run,omitempty"`
	ScheduledAt   time.Time      `json:"scheduled_at"`
	ExecutedAt    time.Time      `json:"executed_at"`
}

// AlertRequest is sent by external systems.
type AlertRequest struct {
	Source  string         `json:"source"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// ProbeConfigResponse is returned to watchers fetching their configs.
type ProbeConfigResponse struct {
	ID             int            `json:"id"`
	ProbeTypeName  string         `json:"probe_type_name"`
	ProbeVersion   string         `json:"probe_version"`
	ExecutablePath string         `json:"executable_path"`
	Subcommand     string         `json:"subcommand,omitempty"`
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments"`
	Interval       string         `json:"interval"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	NextRunAt      *time.Time     `json:"next_run_at"`
}

func (s *Server) handlePushRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(db.SQLiteTimeFormat)

	// Upsert watcher using SQLite's INSERT OR REPLACE pattern
	// First try to get existing watcher
	var watcherID int
	err := s.db.DB().QueryRowContext(ctx, `SELECT id FROM watchers WHERE name = ?`, req.Name).Scan(&watcherID)
	if err != nil {
		// Insert new watcher
		result, err := s.db.DB().ExecContext(ctx, `
			INSERT INTO watchers (name, version, callback_url, last_seen_at, registered_at)
			VALUES (?, ?, ?, ?, ?)
		`, req.Name, req.Version, req.CallbackURL, now, now)
		if err != nil {
			slog.Error("failed to register watcher", "name", req.Name, "error", err)
			http.Error(w, "failed to register watcher", http.StatusInternalServerError)
			return
		}
		id, _ := result.LastInsertId()
		watcherID = int(id)
	} else {
		// Update existing watcher
		_, err = s.db.DB().ExecContext(ctx, `
			UPDATE watchers SET version = ?, callback_url = ?, last_seen_at = ?
			WHERE id = ?
		`, req.Version, req.CallbackURL, now, watcherID)
		if err != nil {
			slog.Error("failed to update watcher", "name", req.Name, "error", err)
			http.Error(w, "failed to update watcher", http.StatusInternalServerError)
			return
		}
	}

	// Register probe types
	for _, pt := range req.ProbeTypes {
		if pt.Version == "" {
			pt.Version = "0.0.0"
		}

		argumentsJSON, _ := json.Marshal(pt.Arguments)

		// Upsert probe type - check if it exists first
		var probeTypeID int
		err := s.db.DB().QueryRowContext(ctx, `
			SELECT id FROM probe_types WHERE name = ? AND version = ?
		`, pt.Name, pt.Version).Scan(&probeTypeID)
		if err != nil {
			// Insert new probe type
			result, err := s.db.DB().ExecContext(ctx, `
				INSERT INTO probe_types (name, version, description, arguments, registered_at)
				VALUES (?, ?, ?, ?, ?)
			`, pt.Name, pt.Version, pt.Description, string(argumentsJSON), now)
			if err != nil {
				slog.Error("failed to register probe type", "name", pt.Name, "error", err)
				continue
			}
			id, _ := result.LastInsertId()
			probeTypeID = int(id)
		} else {
			// Update existing probe type
			_, err = s.db.DB().ExecContext(ctx, `
				UPDATE probe_types SET description = ?, arguments = ?, updated_at = ?
				WHERE id = ?
			`, pt.Description, string(argumentsJSON), now, probeTypeID)
			if err != nil {
				slog.Error("failed to update probe type", "name", pt.Name, "error", err)
				continue
			}
		}

		// Link probe type to watcher with executable path and subcommand
		// Use INSERT OR REPLACE for SQLite
		_, err = s.db.DB().ExecContext(ctx, `
			INSERT INTO watcher_probe_types (watcher_id, probe_type_id, executable_path, subcommand)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (watcher_id, probe_type_id) DO UPDATE SET
				executable_path = excluded.executable_path,
				subcommand = excluded.subcommand
		`, watcherID, probeTypeID, pt.ExecutablePath, pt.Subcommand)
		if err != nil {
			slog.Error("failed to link probe type to watcher", "watcher", req.Name, "probe", pt.Name, "error", err)
		}
	}

	slog.Info("watcher registered", "name", req.Name, "version", req.Version, "probe_types", len(req.ProbeTypes))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"watcher_id":        watcherID,
		"registered_probes": len(req.ProbeTypes),
	})
}

func (s *Server) handlePushHeartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(db.SQLiteTimeFormat)

	result, err := s.db.DB().ExecContext(ctx, `
		UPDATE watchers SET last_seen_at = ?, version = ? WHERE name = ?
	`, now, req.Version, req.Name)
	if err != nil {
		http.Error(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "watcher not registered", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handlePushResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get watcher ID
	var watcherID int
	err := s.db.DB().QueryRowContext(ctx, `SELECT id FROM watchers WHERE name = ?`, req.Watcher).Scan(&watcherID)
	if err != nil {
		http.Error(w, "watcher not found", http.StatusNotFound)
		return
	}

	// Parse next_run if provided by probe, otherwise calculate from interval
	var nextRunAt *time.Time
	if req.NextRun != "" {
		t, err := time.Parse(time.RFC3339, req.NextRun)
		if err == nil {
			nextRunAt = &t
		}
	} else {
		// Calculate next_run from interval
		var intervalStr string
		err := s.db.DB().QueryRowContext(ctx, `SELECT interval FROM probe_configs WHERE id = ?`, req.ProbeConfigID).Scan(&intervalStr)
		if err == nil {
			if interval, err := parseInterval(intervalStr); err == nil && interval > 0 {
				t := req.ExecutedAt.Add(interval)
				nextRunAt = &t
			}
		}
	}

	// Insert result
	metricsJSON, _ := json.Marshal(req.Metrics)
	dataJSON, _ := json.Marshal(req.Data)
	var nextRunAtStr *string
	if nextRunAt != nil {
		s := nextRunAt.UTC().Format(db.SQLiteTimeFormat)
		nextRunAtStr = &s
	}

	_, err = s.db.DB().ExecContext(ctx, `
		INSERT INTO probe_results (probe_config_id, watcher_id, status, message, metrics, data, duration_ms, next_run_at, scheduled_at, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ProbeConfigID, watcherID, req.Status, req.Message, string(metricsJSON), string(dataJSON), req.DurationMs, nextRunAtStr, req.ScheduledAt.UTC().Format(db.SQLiteTimeFormat), req.ExecutedAt.UTC().Format(db.SQLiteTimeFormat))
	if err != nil {
		slog.Error("failed to insert result", "probe_config_id", req.ProbeConfigID, "error", err)
		http.Error(w, "failed to record result", http.StatusInternalServerError)
		return
	}

	// Update next_run_at on probe_config
	if nextRunAtStr != nil {
		_, err = s.db.DB().ExecContext(ctx, `
			UPDATE probe_configs SET next_run_at = ? WHERE id = ?
		`, nextRunAtStr, req.ProbeConfigID)
		if err != nil {
			slog.Error("failed to update next_run_at", "probe_config_id", req.ProbeConfigID, "error", err)
		}
	}

	// Check for status change and send notifications
	s.checkStatusChangeAndNotify(ctx, req.ProbeConfigID, probe.Status(req.Status), req.Message)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) checkStatusChangeAndNotify(ctx context.Context, configID int, newStatus probe.Status, message string) {
	// Get probe config details, watcher paused status, and previous status
	var probeName string
	var notificationChannels db.JSONIntArray
	var prevStatus *string
	var watcherPaused int

	err := s.db.DB().QueryRowContext(ctx, `
		SELECT pc.name, pc.notification_channels,
		       (SELECT status FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1 OFFSET 1),
		       COALESCE(w.paused, 0)
		FROM probe_configs pc
		LEFT JOIN watchers w ON w.id = pc.watcher_id
		WHERE pc.id = ?
	`, configID).Scan(&probeName, &notificationChannels, &prevStatus, &watcherPaused)
	if err != nil {
		slog.Error("failed to get probe config for notification", "config_id", configID, "error", err)
		return
	}

	// Skip notifications if watcher is paused
	if watcherPaused != 0 {
		return
	}

	// Only notify on status change
	if prevStatus != nil && probe.Status(*prevStatus) == newStatus {
		return
	}

	if len(notificationChannels) == 0 {
		return
	}

	change := &notify.StatusChange{
		ProbeName: probeName,
		NewStatus: newStatus,
		Message:   message,
	}
	if prevStatus != nil {
		change.OldStatus = probe.Status(*prevStatus)
	}

	s.dispatcher.NotifyStatusChange(ctx, notificationChannels, change)
}

func (s *Server) handlePushAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req AlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(db.SQLiteTimeFormat)

	// Find or create a probe config for this external source
	var configID int
	var notificationChannels db.JSONIntArray

	err := s.db.DB().QueryRowContext(ctx, `
		SELECT id, notification_channels FROM probe_configs WHERE name = ? AND watcher_id IS NULL
	`, req.Source).Scan(&configID, &notificationChannels)
	if err != nil {
		// Create probe type and config for external alerts
		var probeTypeID int
		err = s.db.DB().QueryRowContext(ctx, `
			SELECT id FROM probe_types WHERE name = ? AND version = ?
		`, "external-alert", "1.0.0").Scan(&probeTypeID)
		if err != nil {
			// Insert the probe type
			result, err := s.db.DB().ExecContext(ctx, `
				INSERT INTO probe_types (name, version, description, arguments, registered_at)
				VALUES (?, ?, ?, ?, ?)
			`, "external-alert", "1.0.0", "External alert source", "{}", now)
			if err != nil {
				http.Error(w, "failed to create probe type", http.StatusInternalServerError)
				return
			}
			id, _ := result.LastInsertId()
			probeTypeID = int(id)
		}

		result, err := s.db.DB().ExecContext(ctx, `
			INSERT INTO probe_configs (probe_type_id, name, enabled, arguments, interval, timeout_seconds)
			VALUES (?, ?, 1, '{}', '0', 0)
		`, probeTypeID, req.Source)
		if err != nil {
			http.Error(w, "failed to create probe config", http.StatusInternalServerError)
			return
		}
		id, _ := result.LastInsertId()
		configID = int(id)
	}

	// Insert result (no watcher_id for external alerts)
	dataJSON, _ := json.Marshal(req.Data)
	_, err = s.db.DB().ExecContext(ctx, `
		INSERT INTO probe_results (probe_config_id, status, message, data, duration_ms, scheduled_at, executed_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
	`, configID, req.Status, req.Message, string(dataJSON), now, now)
	if err != nil {
		http.Error(w, "failed to record alert", http.StatusInternalServerError)
		return
	}

	// Notify on critical alerts
	if probe.Status(req.Status) == probe.StatusCritical && len(notificationChannels) > 0 {
		change := &notify.StatusChange{
			ProbeName: req.Source,
			NewStatus: probe.Status(req.Status),
			Message:   req.Message,
		}
		s.dispatcher.NotifyStatusChange(ctx, notificationChannels, change)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"config_id": configID,
		"status":    "recorded",
	})
}

func (s *Server) handlePushGetConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	watcherName := r.PathValue("watcher")

	// Get watcher ID
	var watcherID int
	err := s.db.DB().QueryRowContext(ctx, `SELECT id FROM watchers WHERE name = ?`, watcherName).Scan(&watcherID)
	if err != nil {
		http.Error(w, "watcher not found", http.StatusNotFound)
		return
	}

	// Get configs assigned to this watcher with probe type info
	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT pc.id, pt.name, pt.version, wpt.executable_path, wpt.subcommand, pc.name, pc.arguments,
		       pc.interval, pc.timeout_seconds, pc.next_run_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id AND wpt.watcher_id = ?
		WHERE pc.watcher_id = ? AND pc.enabled = 1
	`, watcherID, watcherID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var configs []ProbeConfigResponse
	for rows.Next() {
		var cfg ProbeConfigResponse
		var subcommand *string
		var arguments db.JSONMap
		var nextRunAt db.NullTime
		if err := rows.Scan(
			&cfg.ID, &cfg.ProbeTypeName, &cfg.ProbeVersion, &cfg.ExecutablePath, &subcommand,
			&cfg.Name, &arguments, &cfg.Interval, &cfg.TimeoutSeconds, &nextRunAt,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg.Arguments = arguments
		if subcommand != nil {
			cfg.Subcommand = *subcommand
		}
		if nextRunAt.Valid {
			cfg.NextRunAt = &nextRunAt.Time
		}
		configs = append(configs, cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
}

// parseInterval parses interval strings like "5m", "1h", "1d".
func parseInterval(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, nil
	}

	var value int
	var unit byte
	_, err := fmt.Sscanf(s, "%d%c", &value, &unit)
	if err != nil {
		return time.ParseDuration(s)
	}

	switch unit {
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return time.ParseDuration(s)
	}
}
