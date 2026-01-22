package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jandubois/monitor/internal/db"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all watchers and their health status
	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT name, last_seen_at, version FROM watchers ORDER BY name
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var watchers []map[string]any
	allHealthy := true
	for rows.Next() {
		var name string
		var lastSeen db.NullTime
		var version *string
		if err := rows.Scan(&name, &lastSeen, &version); err != nil {
			continue
		}
		healthy := lastSeen.Valid && time.Since(lastSeen.Time) < 30*time.Second
		if !healthy {
			allHealthy = false
		}
		watcher := map[string]any{
			"name":    name,
			"healthy": healthy,
		}
		if lastSeen.Valid {
			watcher["last_seen"] = lastSeen.Time
		}
		if version != nil {
			watcher["version"] = *version
		}
		watchers = append(watchers, watcher)
	}

	// Get recent failure count
	var recentFailures int
	s.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM probe_results
		WHERE status IN ('critical', 'unknown')
		AND executed_at > datetime('now', '-1 hour')
	`).Scan(&recentFailures)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"server_name":     s.config.Name,
		"watchers":        watchers,
		"all_healthy":     allHealthy,
		"recent_failures": recentFailures,
	})
}

func (s *Server) handleListProbeTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for watcher filter
	watcherIDStr := r.URL.Query().Get("watcher")

	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close() error
	}
	var err error

	if watcherIDStr != "" {
		// Filter by watcher - include executable_path from watcher_probe_types
		watcherID, _ := strconv.Atoi(watcherIDStr)
		rows, err = s.db.DB().QueryContext(ctx, `
			SELECT pt.id, pt.name, pt.description, pt.version, pt.arguments,
			       wpt.executable_path, pt.registered_at, pt.updated_at
			FROM probe_types pt
			JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id
			WHERE wpt.watcher_id = ?
			ORDER BY pt.name, pt.version
		`, watcherID)
	} else {
		// No filter - return all probe types without executable_path
		rows, err = s.db.DB().QueryContext(ctx, `
			SELECT id, name, description, version, arguments, registered_at, updated_at
			FROM probe_types
			ORDER BY name, version
		`)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var probeTypes []map[string]any
	for rows.Next() {
		var id int
		var name, version string
		var description *string
		var arguments db.JSONMap
		var registeredAt db.NullTime
		var updatedAt db.NullTime

		pt := map[string]any{}

		if watcherIDStr != "" {
			var executablePath string
			if err := rows.Scan(&id, &name, &description, &version, &arguments, &executablePath, &registeredAt, &updatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			pt["executable_path"] = executablePath
		} else {
			if err := rows.Scan(&id, &name, &description, &version, &arguments, &registeredAt, &updatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		pt["id"] = id
		pt["name"] = name
		if description != nil {
			pt["description"] = *description
		} else {
			pt["description"] = ""
		}
		pt["version"] = version
		pt["arguments"] = arguments
		if registeredAt.Valid {
			pt["registered_at"] = registeredAt.Time
		}
		if updatedAt.Valid {
			pt["updated_at"] = updatedAt.Time
		}

		probeTypes = append(probeTypes, pt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(probeTypes)
}

func (s *Server) handleDiscoverProbeTypes(w http.ResponseWriter, r *http.Request) {
	// In the new architecture, probe discovery happens on watchers
	// and is pushed via POST /api/push/register
	// This endpoint now just returns the current state
	ctx := r.Context()

	var count int
	s.db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM probe_types`).Scan(&count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message":     "probe types are discovered by watchers on startup",
		"probe_types": count,
	})
}

func (s *Server) handleListWatchers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT w.id, w.name, w.last_seen_at, w.version, w.registered_at, w.paused,
		       (SELECT COUNT(*) FROM watcher_probe_types WHERE watcher_id = w.id) as probe_type_count,
		       (SELECT COUNT(*) FROM probe_configs WHERE watcher_id = w.id) as config_count
		FROM watchers w
		ORDER BY w.name
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var watchers []map[string]any
	for rows.Next() {
		var id int
		var name string
		var lastSeen db.NullTime
		var version *string
		var registeredAt db.NullTime
		var paused int
		var probeTypeCount, configCount int

		if err := rows.Scan(&id, &name, &lastSeen, &version, &registeredAt, &paused, &probeTypeCount, &configCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		healthy := lastSeen.Valid && time.Since(lastSeen.Time) < 30*time.Second

		watcher := map[string]any{
			"id":               id,
			"name":             name,
			"healthy":          healthy,
			"paused":           paused != 0,
			"probe_type_count": probeTypeCount,
			"config_count":     configCount,
		}
		if registeredAt.Valid {
			watcher["registered_at"] = registeredAt.Time
		}
		if lastSeen.Valid {
			watcher["last_seen_at"] = lastSeen.Time
		}
		if version != nil {
			watcher["version"] = *version
		}

		watchers = append(watchers, watcher)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(watchers)
}

func (s *Server) handleGetWatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var name string
	var lastSeen db.NullTime
	var version *string
	var registeredAt db.NullTime
	var paused int

	err := s.db.DB().QueryRowContext(ctx, `
		SELECT id, name, last_seen_at, version, registered_at, paused
		FROM watchers WHERE id = ?
	`, id).Scan(&id, &name, &lastSeen, &version, &registeredAt, &paused)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Get probe types for this watcher
	ptRows, err := s.db.DB().QueryContext(ctx, `
		SELECT pt.id, pt.name, pt.version, pt.description, wpt.executable_path
		FROM probe_types pt
		JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id
		WHERE wpt.watcher_id = ?
		ORDER BY pt.name
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ptRows.Close()

	var probeTypes []map[string]any
	for ptRows.Next() {
		var ptID int
		var ptName, ptVersion, execPath string
		var ptDesc *string
		if err := ptRows.Scan(&ptID, &ptName, &ptVersion, &ptDesc, &execPath); err != nil {
			continue
		}
		pt := map[string]any{
			"id":              ptID,
			"name":            ptName,
			"version":         ptVersion,
			"executable_path": execPath,
		}
		if ptDesc != nil {
			pt["description"] = *ptDesc
		}
		probeTypes = append(probeTypes, pt)
	}

	healthy := lastSeen.Valid && time.Since(lastSeen.Time) < 30*time.Second

	watcher := map[string]any{
		"id":          id,
		"name":        name,
		"healthy":     healthy,
		"paused":      paused != 0,
		"probe_types": probeTypes,
	}
	if registeredAt.Valid {
		watcher["registered_at"] = registeredAt.Time
	}
	if lastSeen.Valid {
		watcher["last_seen_at"] = lastSeen.Time
	}
	if version != nil {
		watcher["version"] = *version
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(watcher)
}

func (s *Server) handleDeleteWatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	result, err := s.db.DB().ExecContext(ctx, `DELETE FROM watchers WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "watcher not found", http.StatusNotFound)
		return
	}

	slog.Info("watcher deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetWatcherPaused(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var req struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pausedInt := 0
	if req.Paused {
		pausedInt = 1
	}

	result, err := s.db.DB().ExecContext(ctx, `UPDATE watchers SET paused = ? WHERE id = ?`, pausedInt, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "watcher not found", http.StatusNotFound)
		return
	}

	slog.Info("watcher paused state changed", "id", id, "paused", req.Paused)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"paused": req.Paused})
}

func (s *Server) handleListProbeConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Build query with optional filters
	// Use a subquery instead of LATERAL JOIN for SQLite compatibility
	query := `
		SELECT pc.id, pc.probe_type_id, pt.name as probe_type_name, pc.name, pc.enabled,
		       pc.arguments, pc.interval, pc.timeout_seconds, pc.notification_channels,
		       pc.watcher_id, w.name as watcher_name, pc.next_run_at, pc.group_path, pc.keywords,
		       pc.created_at, pc.updated_at,
		       (SELECT status FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1) as last_status,
		       (SELECT message FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1) as last_message,
		       (SELECT executed_at FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1) as last_executed_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		LEFT JOIN watchers w ON w.id = pc.watcher_id
		WHERE 1=1
	`
	args := []any{}

	// Filter by watcher
	if watcherID := r.URL.Query().Get("watcher"); watcherID != "" {
		query += " AND pc.watcher_id = ?"
		args = append(args, watcherID)
	}

	// Filter by group
	if group := r.URL.Query().Get("group"); group != "" {
		query += " AND (pc.group_path = ? OR pc.group_path LIKE ?)"
		args = append(args, group, group+"/%")
	}

	// Filter by keywords - use JSON functions for SQLite
	if keywords := r.URL.Query().Get("keywords"); keywords != "" {
		// For SQLite, we search if the keywords JSON array contains the value
		query += " AND EXISTS (SELECT 1 FROM json_each(pc.keywords) WHERE json_each.value = ?)"
		args = append(args, keywords)
	}

	query += " ORDER BY pc.name"

	rows, err := s.db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var configs []map[string]any
	for rows.Next() {
		var id, probeTypeID, timeoutSeconds int
		var probeTypeName, name, interval string
		var enabled int
		var arguments db.JSONMap
		var notificationChannels db.JSONIntArray
		var watcherID *int
		var watcherName, groupPath *string
		var keywords db.JSONStringArray
		var nextRunAt db.NullTime
		var createdAt db.NullTime
		var updatedAt, lastExecutedAt db.NullTime
		var lastStatus, lastMessage *string

		if err := rows.Scan(
			&id, &probeTypeID, &probeTypeName, &name, &enabled,
			&arguments, &interval, &timeoutSeconds, &notificationChannels,
			&watcherID, &watcherName, &nextRunAt, &groupPath, &keywords,
			&createdAt, &updatedAt,
			&lastStatus, &lastMessage, &lastExecutedAt,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		config := map[string]any{
			"id":                    id,
			"probe_type_id":         probeTypeID,
			"probe_type_name":       probeTypeName,
			"name":                  name,
			"enabled":               enabled != 0,
			"arguments":             arguments,
			"interval":              interval,
			"timeout_seconds":       timeoutSeconds,
			"notification_channels": notificationChannels,
			"keywords":              keywords,
		}
		if createdAt.Valid {
			config["created_at"] = createdAt.Time
		}
		if updatedAt.Valid {
			config["updated_at"] = updatedAt.Time
		}
		if watcherID != nil {
			config["watcher_id"] = *watcherID
		}
		if watcherName != nil {
			config["watcher_name"] = *watcherName
		}
		if nextRunAt.Valid {
			config["next_run_at"] = nextRunAt.Time
		}
		if groupPath != nil {
			config["group_path"] = *groupPath
		}
		if lastStatus != nil {
			config["last_status"] = *lastStatus
		}
		if lastMessage != nil {
			config["last_message"] = *lastMessage
		}
		if lastExecutedAt.Valid {
			config["last_executed_at"] = lastExecutedAt.Time
		}

		configs = append(configs, config)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
}

func (s *Server) handleCreateProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		ProbeTypeID          int            `json:"probe_type_id"`
		WatcherID            *int           `json:"watcher_id"`
		Name                 string         `json:"name"`
		Enabled              bool           `json:"enabled"`
		Arguments            map[string]any `json:"arguments"`
		Interval             string         `json:"interval"`
		TimeoutSeconds       int            `json:"timeout_seconds"`
		NotificationChannels []int          `json:"notification_channels"`
		GroupPath            *string        `json:"group_path"`
		Keywords             []string       `json:"keywords"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 60
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	argumentsJSON, _ := json.Marshal(req.Arguments)
	notificationChannelsJSON, _ := json.Marshal(req.NotificationChannels)
	keywordsJSON, _ := json.Marshal(req.Keywords)

	result, err := s.db.DB().ExecContext(ctx, `
		INSERT INTO probe_configs (probe_type_id, watcher_id, name, enabled, arguments, interval, timeout_seconds, notification_channels, group_path, keywords)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ProbeTypeID, req.WatcherID, req.Name, enabledInt, string(argumentsJSON), req.Interval, req.TimeoutSeconds, string(notificationChannelsJSON), req.GroupPath, string(keywordsJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (s *Server) handleGetProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var probeTypeID, timeoutSeconds int
	var probeTypeName, name, interval string
	var enabled int
	var arguments db.JSONMap
	var notificationChannels db.JSONIntArray
	var watcherID *int
	var watcherName, groupPath *string
	var keywords db.JSONStringArray
	var nextRunAt db.NullTime
	var createdAt db.NullTime
	var updatedAt db.NullTime

	err := s.db.DB().QueryRowContext(ctx, `
		SELECT pc.id, pc.probe_type_id, pt.name, pc.name, pc.enabled, pc.arguments,
		       pc.interval, pc.timeout_seconds, pc.notification_channels,
		       pc.watcher_id, w.name, pc.next_run_at, pc.group_path, pc.keywords,
		       pc.created_at, pc.updated_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		LEFT JOIN watchers w ON w.id = pc.watcher_id
		WHERE pc.id = ?
	`, id).Scan(&id, &probeTypeID, &probeTypeName, &name, &enabled, &arguments,
		&interval, &timeoutSeconds, &notificationChannels,
		&watcherID, &watcherName, &nextRunAt, &groupPath, &keywords,
		&createdAt, &updatedAt)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	config := map[string]any{
		"id":                    id,
		"probe_type_id":         probeTypeID,
		"probe_type_name":       probeTypeName,
		"name":                  name,
		"enabled":               enabled != 0,
		"arguments":             arguments,
		"interval":              interval,
		"timeout_seconds":       timeoutSeconds,
		"notification_channels": notificationChannels,
		"keywords":              keywords,
	}
	if createdAt.Valid {
		config["created_at"] = createdAt.Time
	}
	if updatedAt.Valid {
		config["updated_at"] = updatedAt.Time
	}
	if watcherID != nil {
		config["watcher_id"] = *watcherID
	}
	if watcherName != nil {
		config["watcher_name"] = *watcherName
	}
	if nextRunAt.Valid {
		config["next_run_at"] = nextRunAt.Time
	}
	if groupPath != nil {
		config["group_path"] = *groupPath
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (s *Server) handleUpdateProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var req struct {
		WatcherID            *int           `json:"watcher_id"`
		Name                 string         `json:"name"`
		Enabled              bool           `json:"enabled"`
		Arguments            map[string]any `json:"arguments"`
		Interval             string         `json:"interval"`
		TimeoutSeconds       int            `json:"timeout_seconds"`
		NotificationChannels []int          `json:"notification_channels"`
		GroupPath            *string        `json:"group_path"`
		Keywords             []string       `json:"keywords"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	argumentsJSON, _ := json.Marshal(req.Arguments)
	notificationChannelsJSON, _ := json.Marshal(req.NotificationChannels)
	keywordsJSON, _ := json.Marshal(req.Keywords)

	_, err := s.db.DB().ExecContext(ctx, `
		UPDATE probe_configs
		SET watcher_id = ?, name = ?, enabled = ?, arguments = ?, interval = ?,
		    timeout_seconds = ?, notification_channels = ?, group_path = ?, keywords = ?, updated_at = datetime('now')
		WHERE id = ?
	`, req.WatcherID, req.Name, enabledInt, string(argumentsJSON), req.Interval, req.TimeoutSeconds, string(notificationChannelsJSON), req.GroupPath, string(keywordsJSON), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	_, err := s.db.DB().ExecContext(ctx, `DELETE FROM probe_configs WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	// Get watcher callback URL for this probe config
	var callbackURL *string
	err := s.db.DB().QueryRowContext(ctx, `
		SELECT w.callback_url
		FROM probe_configs pc
		JOIN watchers w ON w.id = pc.watcher_id
		WHERE pc.id = ? AND pc.enabled = 1
	`, id).Scan(&callbackURL)
	if err != nil {
		http.Error(w, "probe config not found or disabled", http.StatusNotFound)
		return
	}

	// If watcher has callback URL, trigger directly
	if callbackURL != nil && *callbackURL != "" {
		triggerURL := fmt.Sprintf("%s/trigger/%d", *callbackURL, id)
		req, err := http.NewRequestWithContext(ctx, "POST", triggerURL, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+s.config.AuthToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Warn("failed to trigger watcher directly, falling back to poll", "error", err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
				return
			}
			slog.Warn("watcher trigger returned non-OK status", "status", resp.StatusCode)
		}
	}

	// Fall back to setting next_run_at for poll-based trigger
	_, err = s.db.DB().ExecContext(ctx, `
		UPDATE probe_configs SET next_run_at = datetime('now') WHERE id = ?
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scheduled"})
}

func (s *Server) handleSetProbeEnabled(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	// Update enabled state
	_, err := s.db.DB().ExecContext(ctx, `
		UPDATE probe_configs SET enabled = ?, updated_at = datetime('now') WHERE id = ?
	`, enabledInt, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If enabling (resuming), trigger immediate run
	if req.Enabled {
		// Get watcher callback URL
		var callbackURL *string
		s.db.DB().QueryRowContext(ctx, `
			SELECT w.callback_url
			FROM probe_configs pc
			JOIN watchers w ON w.id = pc.watcher_id
			WHERE pc.id = ?
		`, id).Scan(&callbackURL)

		if callbackURL != nil && *callbackURL != "" {
			triggerURL := fmt.Sprintf("%s/trigger/%d", *callbackURL, id)
			triggerReq, _ := http.NewRequestWithContext(ctx, "POST", triggerURL, nil)
			triggerReq.Header.Set("Authorization", "Bearer "+s.config.AuthToken)
			if resp, err := http.DefaultClient.Do(triggerReq); err == nil {
				resp.Body.Close()
			}
		} else {
			// Fall back to poll-based trigger
			s.db.DB().ExecContext(ctx, `UPDATE probe_configs SET next_run_at = datetime('now') WHERE id = ?`, id)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": req.Enabled})
}

func (s *Server) handleQueryResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configID := r.URL.Query().Get("config_id")
	status := r.URL.Query().Get("status")
	since := r.URL.Query().Get("since")
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")

	query := `
		SELECT pr.id, pr.probe_config_id, pc.name as config_name, pr.status, pr.message,
		       pr.metrics, pr.data, pr.duration_ms, pr.scheduled_at, pr.executed_at, pr.recorded_at
		FROM probe_results pr
		JOIN probe_configs pc ON pc.id = pr.probe_config_id
		WHERE 1=1
	`
	args := []any{}

	if configID != "" {
		query += " AND pr.probe_config_id = ?"
		args = append(args, configID)
	}
	if status != "" {
		// Support multiple statuses with comma separator using IN clause
		query += " AND pr.status IN (SELECT value FROM json_each(?))"
		statusArray, _ := json.Marshal([]string{status})
		args = append(args, string(statusArray))
	}
	if since != "" {
		query += " AND pr.executed_at > ?"
		args = append(args, since)
	}

	query += " ORDER BY pr.executed_at DESC"

	if limit != "" {
		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		query += " LIMIT 100"
	}

	if offset != "" {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := s.db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, probeConfigID, durationMs int
		var configName, statusVal string
		var message *string
		var metrics, data db.JSONMap
		var scheduledAt, executedAt, recordedAt db.NullTime

		if err := rows.Scan(&id, &probeConfigID, &configName, &statusVal, &message,
			&metrics, &data, &durationMs, &scheduledAt, &executedAt, &recordedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result := map[string]any{
			"id":              id,
			"probe_config_id": probeConfigID,
			"config_name":     configName,
			"status":          statusVal,
			"metrics":         metrics,
			"data":            data,
			"duration_ms":     durationMs,
		}
		if message != nil {
			result["message"] = *message
		} else {
			result["message"] = ""
		}
		if scheduledAt.Valid {
			result["scheduled_at"] = scheduledAt.Time
		}
		if executedAt.Valid {
			result["executed_at"] = executedAt.Time
		}
		if recordedAt.Valid {
			result["recorded_at"] = recordedAt.Time
		}

		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleGetResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configID := r.PathValue("config_id")

	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT id, probe_config_id, status, message, metrics, data,
		       duration_ms, scheduled_at, executed_at, recorded_at
		FROM probe_results
		WHERE probe_config_id = ?
		ORDER BY executed_at DESC
		LIMIT 100
	`, configID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, probeConfigID, durationMs int
		var statusVal string
		var message *string
		var metrics, data db.JSONMap
		var scheduledAt, executedAt, recordedAt db.NullTime

		if err := rows.Scan(&id, &probeConfigID, &statusVal, &message, &metrics, &data,
			&durationMs, &scheduledAt, &executedAt, &recordedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result := map[string]any{
			"id":              id,
			"probe_config_id": probeConfigID,
			"status":          statusVal,
			"metrics":         metrics,
			"data":            data,
			"duration_ms":     durationMs,
		}
		if message != nil {
			result["message"] = *message
		} else {
			result["message"] = ""
		}
		if scheduledAt.Valid {
			result["scheduled_at"] = scheduledAt.Time
		}
		if executedAt.Valid {
			result["executed_at"] = executedAt.Time
		}
		if recordedAt.Valid {
			result["recorded_at"] = recordedAt.Time
		}

		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleResultStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalConfigs, enabledConfigs int
	s.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END) FROM probe_configs
	`).Scan(&totalConfigs, &enabledConfigs)

	// Use a subquery to get the latest status for each probe config, then count
	var okCount, warningCount, criticalCount, unknownCount int
	s.db.DB().QueryRowContext(ctx, `
		SELECT
			SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'warning' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'critical' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'unknown' THEN 1 ELSE 0 END)
		FROM (
			SELECT probe_config_id, status,
			       ROW_NUMBER() OVER (PARTITION BY probe_config_id ORDER BY executed_at DESC) as rn
			FROM probe_results
		) WHERE rn = 1
	`).Scan(&okCount, &warningCount, &criticalCount, &unknownCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total_configs":   totalConfigs,
		"enabled_configs": enabledConfigs,
		"status_counts": map[string]int{
			"ok":       okCount,
			"warning":  warningCount,
			"critical": criticalCount,
			"unknown":  unknownCount,
		},
	})
}

func (s *Server) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT id, name, type, config, enabled FROM notification_channels ORDER BY name
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var channels []map[string]any
	for rows.Next() {
		var id int
		var name, channelType string
		var config db.JSONMap
		var enabled int

		if err := rows.Scan(&id, &name, &channelType, &config, &enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		channels = append(channels, map[string]any{
			"id":      id,
			"name":    name,
			"type":    channelType,
			"config":  config,
			"enabled": enabled != 0,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

func (s *Server) handleCreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name    string         `json:"name"`
		Type    string         `json:"type"`
		Config  map[string]any `json:"config"`
		Enabled bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	configJSON, _ := json.Marshal(req.Config)

	result, err := s.db.DB().ExecContext(ctx, `
		INSERT INTO notification_channels (name, type, config, enabled)
		VALUES (?, ?, ?, ?)
	`, req.Name, req.Type, string(configJSON), enabledInt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (s *Server) handleUpdateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var req struct {
		Name    string         `json:"name"`
		Type    string         `json:"type"`
		Config  map[string]any `json:"config"`
		Enabled bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	configJSON, _ := json.Marshal(req.Config)

	_, err := s.db.DB().ExecContext(ctx, `
		UPDATE notification_channels
		SET name = ?, type = ?, config = ?, enabled = ?
		WHERE id = ?
	`, req.Name, req.Type, string(configJSON), enabledInt, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	_, err := s.db.DB().ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	// This would send a test notification
	// For now, return not implemented
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
