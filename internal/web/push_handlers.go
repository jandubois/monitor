package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jankremlacek/monitor/internal/notify"
	"github.com/jankremlacek/monitor/internal/probe"
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

	// Upsert watcher
	var watcherID int
	err := s.db.Pool().QueryRow(ctx, `
		INSERT INTO watchers (name, version, callback_url, last_seen_at, registered_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE SET
			version = EXCLUDED.version,
			callback_url = EXCLUDED.callback_url,
			last_seen_at = NOW()
		RETURNING id
	`, req.Name, req.Version, req.CallbackURL).Scan(&watcherID)
	if err != nil {
		slog.Error("failed to register watcher", "name", req.Name, "error", err)
		http.Error(w, "failed to register watcher", http.StatusInternalServerError)
		return
	}

	// Register probe types
	for _, pt := range req.ProbeTypes {
		if pt.Version == "" {
			pt.Version = "0.0.0"
		}

		// Upsert probe type
		var probeTypeID int
		err := s.db.Pool().QueryRow(ctx, `
			INSERT INTO probe_types (name, version, description, arguments, registered_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (name, version) DO UPDATE SET
				description = EXCLUDED.description,
				arguments = EXCLUDED.arguments,
				updated_at = NOW()
			RETURNING id
		`, pt.Name, pt.Version, pt.Description, pt.Arguments).Scan(&probeTypeID)
		if err != nil {
			slog.Error("failed to register probe type", "name", pt.Name, "error", err)
			continue
		}

		// Link probe type to watcher with executable path and subcommand
		_, err = s.db.Pool().Exec(ctx, `
			INSERT INTO watcher_probe_types (watcher_id, probe_type_id, executable_path, subcommand)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (watcher_id, probe_type_id) DO UPDATE SET
				executable_path = EXCLUDED.executable_path,
				subcommand = EXCLUDED.subcommand
		`, watcherID, probeTypeID, pt.ExecutablePath, pt.Subcommand)
		if err != nil {
			slog.Error("failed to link probe type to watcher", "watcher", req.Name, "probe", pt.Name, "error", err)
		}
	}

	slog.Info("watcher registered", "name", req.Name, "version", req.Version, "probe_types", len(req.ProbeTypes))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"watcher_id":         watcherID,
		"registered_probes":  len(req.ProbeTypes),
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

	result, err := s.db.Pool().Exec(ctx, `
		UPDATE watchers SET last_seen_at = NOW(), version = $2 WHERE name = $1
	`, req.Name, req.Version)
	if err != nil {
		http.Error(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
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
	err := s.db.Pool().QueryRow(ctx, `SELECT id FROM watchers WHERE name = $1`, req.Watcher).Scan(&watcherID)
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
		err := s.db.Pool().QueryRow(ctx, `SELECT interval FROM probe_configs WHERE id = $1`, req.ProbeConfigID).Scan(&intervalStr)
		if err == nil {
			if interval, err := parseInterval(intervalStr); err == nil && interval > 0 {
				t := req.ExecutedAt.Add(interval)
				nextRunAt = &t
			}
		}
	}

	// Insert result
	_, err = s.db.Pool().Exec(ctx, `
		INSERT INTO probe_results (probe_config_id, watcher_id, status, message, metrics, data, duration_ms, next_run_at, scheduled_at, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, req.ProbeConfigID, watcherID, req.Status, req.Message, req.Metrics, req.Data, req.DurationMs, nextRunAt, req.ScheduledAt, req.ExecutedAt)
	if err != nil {
		slog.Error("failed to insert result", "probe_config_id", req.ProbeConfigID, "error", err)
		http.Error(w, "failed to record result", http.StatusInternalServerError)
		return
	}

	// Update next_run_at on probe_config
	if nextRunAt != nil {
		_, err = s.db.Pool().Exec(ctx, `
			UPDATE probe_configs SET next_run_at = $1 WHERE id = $2
		`, nextRunAt, req.ProbeConfigID)
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
	// Get probe config details and previous status
	var probeName string
	var notificationChannels []int
	var prevStatus *string

	err := s.db.Pool().QueryRow(ctx, `
		SELECT pc.name, pc.notification_channels,
		       (SELECT status FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1 OFFSET 1)
		FROM probe_configs pc WHERE pc.id = $1
	`, configID).Scan(&probeName, &notificationChannels, &prevStatus)
	if err != nil {
		slog.Error("failed to get probe config for notification", "config_id", configID, "error", err)
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

	// Find or create a probe config for this external source
	var configID int
	var notificationChannels []int

	err := s.db.Pool().QueryRow(ctx, `
		SELECT id, notification_channels FROM probe_configs WHERE name = $1 AND watcher_id IS NULL
	`, req.Source).Scan(&configID, &notificationChannels)
	if err != nil {
		// Create probe type and config for external alerts
		var probeTypeID int
		err = s.db.Pool().QueryRow(ctx, `
			INSERT INTO probe_types (name, version, description, arguments, registered_at)
			VALUES ($1, '1.0.0', 'External alert source', '{}', NOW())
			ON CONFLICT (name, version) DO UPDATE SET updated_at = NOW()
			RETURNING id
		`, "external-alert").Scan(&probeTypeID)
		if err != nil {
			http.Error(w, "failed to create probe type", http.StatusInternalServerError)
			return
		}

		err = s.db.Pool().QueryRow(ctx, `
			INSERT INTO probe_configs (probe_type_id, name, enabled, arguments, interval, timeout_seconds)
			VALUES ($1, $2, true, '{}', '0', 0)
			RETURNING id
		`, probeTypeID, req.Source).Scan(&configID)
		if err != nil {
			http.Error(w, "failed to create probe config", http.StatusInternalServerError)
			return
		}
	}

	// Insert result (no watcher_id for external alerts)
	now := time.Now()
	_, err = s.db.Pool().Exec(ctx, `
		INSERT INTO probe_results (probe_config_id, status, message, data, duration_ms, scheduled_at, executed_at)
		VALUES ($1, $2, $3, $4, 0, $5, $5)
	`, configID, req.Status, req.Message, req.Data, now)
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
	err := s.db.Pool().QueryRow(ctx, `SELECT id FROM watchers WHERE name = $1`, watcherName).Scan(&watcherID)
	if err != nil {
		http.Error(w, "watcher not found", http.StatusNotFound)
		return
	}

	// Get configs assigned to this watcher with probe type info
	rows, err := s.db.Pool().Query(ctx, `
		SELECT pc.id, pt.name, pt.version, wpt.executable_path, wpt.subcommand, pc.name, pc.arguments,
		       pc.interval, pc.timeout_seconds, pc.next_run_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id AND wpt.watcher_id = $1
		WHERE pc.watcher_id = $1 AND pc.enabled = true
	`, watcherID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var configs []ProbeConfigResponse
	for rows.Next() {
		var cfg ProbeConfigResponse
		var subcommand *string
		if err := rows.Scan(
			&cfg.ID, &cfg.ProbeTypeName, &cfg.ProbeVersion, &cfg.ExecutablePath, &subcommand,
			&cfg.Name, &cfg.Arguments, &cfg.Interval, &cfg.TimeoutSeconds, &cfg.NextRunAt,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if subcommand != nil {
			cfg.Subcommand = *subcommand
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
