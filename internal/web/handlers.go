package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all watchers and their health status
	rows, err := s.db.Pool().Query(ctx, `
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
		var lastSeen *time.Time
		var version *string
		if err := rows.Scan(&name, &lastSeen, &version); err != nil {
			continue
		}
		healthy := lastSeen != nil && time.Since(*lastSeen) < 30*time.Second
		if !healthy {
			allHealthy = false
		}
		watcher := map[string]any{
			"name":      name,
			"healthy":   healthy,
			"last_seen": lastSeen,
		}
		if version != nil {
			watcher["version"] = *version
		}
		watchers = append(watchers, watcher)
	}

	// Get recent failure count
	var recentFailures int
	s.db.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM probe_results
		WHERE status IN ('critical', 'unknown')
		AND executed_at > NOW() - INTERVAL '1 hour'
	`).Scan(&recentFailures)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
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
		Close()
	}
	var err error

	if watcherIDStr != "" {
		// Filter by watcher - include executable_path from watcher_probe_types
		watcherID, _ := strconv.Atoi(watcherIDStr)
		rows, err = s.db.Pool().Query(ctx, `
			SELECT pt.id, pt.name, pt.description, pt.version, pt.arguments,
			       wpt.executable_path, pt.registered_at, pt.updated_at
			FROM probe_types pt
			JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id
			WHERE wpt.watcher_id = $1
			ORDER BY pt.name, pt.version
		`, watcherID)
	} else {
		// No filter - return all probe types without executable_path
		rows, err = s.db.Pool().Query(ctx, `
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
		var name, description, version string
		var arguments map[string]any
		var registeredAt time.Time
		var updatedAt *time.Time

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
		pt["description"] = description
		pt["version"] = version
		pt["arguments"] = arguments
		pt["registered_at"] = registeredAt
		pt["updated_at"] = updatedAt

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
	s.db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM probe_types`).Scan(&count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message":     "probe types are discovered by watchers on startup",
		"probe_types": count,
	})
}

func (s *Server) handleListWatchers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool().Query(ctx, `
		SELECT w.id, w.name, w.last_seen_at, w.version, w.registered_at,
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
		var lastSeen *time.Time
		var version *string
		var registeredAt time.Time
		var probeTypeCount, configCount int

		if err := rows.Scan(&id, &name, &lastSeen, &version, &registeredAt, &probeTypeCount, &configCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		healthy := lastSeen != nil && time.Since(*lastSeen) < 30*time.Second

		watcher := map[string]any{
			"id":               id,
			"name":             name,
			"healthy":          healthy,
			"registered_at":    registeredAt,
			"probe_type_count": probeTypeCount,
			"config_count":     configCount,
		}
		if lastSeen != nil {
			watcher["last_seen_at"] = *lastSeen
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
	var lastSeen *time.Time
	var version *string
	var registeredAt time.Time

	err := s.db.Pool().QueryRow(ctx, `
		SELECT id, name, last_seen_at, version, registered_at
		FROM watchers WHERE id = $1
	`, id).Scan(&id, &name, &lastSeen, &version, &registeredAt)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Get probe types for this watcher
	ptRows, err := s.db.Pool().Query(ctx, `
		SELECT pt.id, pt.name, pt.version, pt.description, wpt.executable_path
		FROM probe_types pt
		JOIN watcher_probe_types wpt ON wpt.probe_type_id = pt.id
		WHERE wpt.watcher_id = $1
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

	healthy := lastSeen != nil && time.Since(*lastSeen) < 30*time.Second

	watcher := map[string]any{
		"id":            id,
		"name":          name,
		"healthy":       healthy,
		"registered_at": registeredAt,
		"probe_types":   probeTypes,
	}
	if lastSeen != nil {
		watcher["last_seen_at"] = *lastSeen
	}
	if version != nil {
		watcher["version"] = *version
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(watcher)
}

func (s *Server) handleListProbeConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Build query with optional filters
	query := `
		SELECT pc.id, pc.probe_type_id, pt.name as probe_type_name, pc.name, pc.enabled,
		       pc.arguments, pc.interval, pc.timeout_seconds, pc.notification_channels,
		       pc.watcher_id, w.name as watcher_name, pc.next_run_at, pc.group_path, pc.keywords,
		       pc.created_at, pc.updated_at,
		       pr.status as last_status, pr.message as last_message, pr.executed_at as last_executed_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		LEFT JOIN watchers w ON w.id = pc.watcher_id
		LEFT JOIN LATERAL (
			SELECT status, message, executed_at
			FROM probe_results
			WHERE probe_config_id = pc.id
			ORDER BY executed_at DESC
			LIMIT 1
		) pr ON true
		WHERE 1=1
	`
	args := []any{}
	argNum := 1

	// Filter by watcher
	if watcherID := r.URL.Query().Get("watcher"); watcherID != "" {
		query += " AND pc.watcher_id = $" + strconv.Itoa(argNum)
		args = append(args, watcherID)
		argNum++
	}

	// Filter by group
	if group := r.URL.Query().Get("group"); group != "" {
		query += " AND (pc.group_path = $" + strconv.Itoa(argNum) + " OR pc.group_path LIKE $" + strconv.Itoa(argNum+1) + ")"
		args = append(args, group, group+"/%")
		argNum += 2
	}

	// Filter by keywords
	if keywords := r.URL.Query().Get("keywords"); keywords != "" {
		query += " AND pc.keywords && $" + strconv.Itoa(argNum)
		args = append(args, "{"+keywords+"}")
		argNum++
	}

	query += " ORDER BY pc.name"

	rows, err := s.db.Pool().Query(ctx, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var configs []map[string]any
	for rows.Next() {
		var id, probeTypeID, timeoutSeconds int
		var probeTypeName, name, interval string
		var enabled bool
		var arguments map[string]any
		var notificationChannels []int
		var watcherID *int
		var watcherName, groupPath *string
		var keywords []string
		var nextRunAt *time.Time
		var createdAt time.Time
		var updatedAt, lastExecutedAt *time.Time
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
			"enabled":               enabled,
			"arguments":             arguments,
			"interval":              interval,
			"timeout_seconds":       timeoutSeconds,
			"notification_channels": notificationChannels,
			"keywords":              keywords,
			"created_at":            createdAt,
			"updated_at":            updatedAt,
		}
		if watcherID != nil {
			config["watcher_id"] = *watcherID
		}
		if watcherName != nil {
			config["watcher_name"] = *watcherName
		}
		if nextRunAt != nil {
			config["next_run_at"] = *nextRunAt
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
		if lastExecutedAt != nil {
			config["last_executed_at"] = *lastExecutedAt
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

	var id int
	err := s.db.Pool().QueryRow(ctx, `
		INSERT INTO probe_configs (probe_type_id, watcher_id, name, enabled, arguments, interval, timeout_seconds, notification_channels, group_path, keywords)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, req.ProbeTypeID, req.WatcherID, req.Name, req.Enabled, req.Arguments, req.Interval, req.TimeoutSeconds, req.NotificationChannels, req.GroupPath, req.Keywords).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (s *Server) handleGetProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var probeTypeID, timeoutSeconds int
	var probeTypeName, name, interval string
	var enabled bool
	var arguments map[string]any
	var notificationChannels []int
	var watcherID *int
	var watcherName, groupPath *string
	var keywords []string
	var nextRunAt *time.Time
	var createdAt time.Time
	var updatedAt *time.Time

	err := s.db.Pool().QueryRow(ctx, `
		SELECT pc.id, pc.probe_type_id, pt.name, pc.name, pc.enabled, pc.arguments,
		       pc.interval, pc.timeout_seconds, pc.notification_channels,
		       pc.watcher_id, w.name, pc.next_run_at, pc.group_path, pc.keywords,
		       pc.created_at, pc.updated_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		LEFT JOIN watchers w ON w.id = pc.watcher_id
		WHERE pc.id = $1
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
		"enabled":               enabled,
		"arguments":             arguments,
		"interval":              interval,
		"timeout_seconds":       timeoutSeconds,
		"notification_channels": notificationChannels,
		"keywords":              keywords,
		"created_at":            createdAt,
		"updated_at":            updatedAt,
	}
	if watcherID != nil {
		config["watcher_id"] = *watcherID
	}
	if watcherName != nil {
		config["watcher_name"] = *watcherName
	}
	if nextRunAt != nil {
		config["next_run_at"] = *nextRunAt
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

	_, err := s.db.Pool().Exec(ctx, `
		UPDATE probe_configs
		SET watcher_id = $1, name = $2, enabled = $3, arguments = $4, interval = $5,
		    timeout_seconds = $6, notification_channels = $7, group_path = $8, keywords = $9, updated_at = NOW()
		WHERE id = $10
	`, req.WatcherID, req.Name, req.Enabled, req.Arguments, req.Interval, req.TimeoutSeconds, req.NotificationChannels, req.GroupPath, req.Keywords, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	_, err := s.db.Pool().Exec(ctx, `DELETE FROM probe_configs WHERE id = $1`, id)
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
	err := s.db.Pool().QueryRow(ctx, `
		SELECT w.callback_url
		FROM probe_configs pc
		JOIN watchers w ON w.id = pc.watcher_id
		WHERE pc.id = $1 AND pc.enabled = true
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
	_, err = s.db.Pool().Exec(ctx, `
		UPDATE probe_configs SET next_run_at = NOW() WHERE id = $1
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

	// Update enabled state
	_, err := s.db.Pool().Exec(ctx, `
		UPDATE probe_configs SET enabled = $1, updated_at = NOW() WHERE id = $2
	`, req.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If enabling (resuming), trigger immediate run
	if req.Enabled {
		// Get watcher callback URL
		var callbackURL *string
		s.db.Pool().QueryRow(ctx, `
			SELECT w.callback_url
			FROM probe_configs pc
			JOIN watchers w ON w.id = pc.watcher_id
			WHERE pc.id = $1
		`, id).Scan(&callbackURL)

		if callbackURL != nil && *callbackURL != "" {
			triggerURL := fmt.Sprintf("%s/trigger/%d", *callbackURL, id)
			triggerReq, _ := http.NewRequestWithContext(ctx, "POST", triggerURL, nil)
			if resp, err := http.DefaultClient.Do(triggerReq); err == nil {
				resp.Body.Close()
			}
		} else {
			// Fall back to poll-based trigger
			s.db.Pool().Exec(ctx, `UPDATE probe_configs SET next_run_at = NOW() WHERE id = $1`, id)
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
	argNum := 1

	if configID != "" {
		query += " AND pr.probe_config_id = $" + strconv.Itoa(argNum)
		args = append(args, configID)
		argNum++
	}
	if status != "" {
		// Support multiple statuses with comma separator
		query += " AND pr.status = ANY($" + strconv.Itoa(argNum) + "::text[])"
		args = append(args, "{"+status+"}")
		argNum++
	}
	if since != "" {
		query += " AND pr.executed_at > $" + strconv.Itoa(argNum)
		args = append(args, since)
		argNum++
	}

	query += " ORDER BY pr.executed_at DESC"

	if limit != "" {
		query += " LIMIT $" + strconv.Itoa(argNum)
		args = append(args, limit)
		argNum++
	} else {
		query += " LIMIT 100"
	}

	if offset != "" {
		query += " OFFSET $" + strconv.Itoa(argNum)
		args = append(args, offset)
	}

	rows, err := s.db.Pool().Query(ctx, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, probeConfigID, durationMs int
		var configName, statusVal, message string
		var metrics, data map[string]any
		var scheduledAt, executedAt, recordedAt time.Time

		if err := rows.Scan(&id, &probeConfigID, &configName, &statusVal, &message,
			&metrics, &data, &durationMs, &scheduledAt, &executedAt, &recordedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		results = append(results, map[string]any{
			"id":              id,
			"probe_config_id": probeConfigID,
			"config_name":     configName,
			"status":          statusVal,
			"message":         message,
			"metrics":         metrics,
			"data":            data,
			"duration_ms":     durationMs,
			"scheduled_at":    scheduledAt,
			"executed_at":     executedAt,
			"recorded_at":     recordedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleGetResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configID := r.PathValue("config_id")

	rows, err := s.db.Pool().Query(ctx, `
		SELECT id, probe_config_id, status, message, metrics, data,
		       duration_ms, scheduled_at, executed_at, recorded_at
		FROM probe_results
		WHERE probe_config_id = $1
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
		var statusVal, message string
		var metrics, data map[string]any
		var scheduledAt, executedAt, recordedAt time.Time

		if err := rows.Scan(&id, &probeConfigID, &statusVal, &message, &metrics, &data,
			&durationMs, &scheduledAt, &executedAt, &recordedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		results = append(results, map[string]any{
			"id":              id,
			"probe_config_id": probeConfigID,
			"status":          statusVal,
			"message":         message,
			"metrics":         metrics,
			"data":            data,
			"duration_ms":     durationMs,
			"scheduled_at":    scheduledAt,
			"executed_at":     executedAt,
			"recorded_at":     recordedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleResultStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalConfigs, enabledConfigs int
	s.db.Pool().QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE enabled) FROM probe_configs`).Scan(&totalConfigs, &enabledConfigs)

	var okCount, warningCount, criticalCount, unknownCount int
	s.db.Pool().QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'ok'),
			COUNT(*) FILTER (WHERE status = 'warning'),
			COUNT(*) FILTER (WHERE status = 'critical'),
			COUNT(*) FILTER (WHERE status = 'unknown')
		FROM (
			SELECT DISTINCT ON (probe_config_id) status
			FROM probe_results
			ORDER BY probe_config_id, executed_at DESC
		) latest
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

	rows, err := s.db.Pool().Query(ctx, `
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
		var config map[string]any
		var enabled bool

		if err := rows.Scan(&id, &name, &channelType, &config, &enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		channels = append(channels, map[string]any{
			"id":      id,
			"name":    name,
			"type":    channelType,
			"config":  config,
			"enabled": enabled,
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

	var id int
	err := s.db.Pool().QueryRow(ctx, `
		INSERT INTO notification_channels (name, type, config, enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Name, req.Type, req.Config, req.Enabled).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	_, err := s.db.Pool().Exec(ctx, `
		UPDATE notification_channels
		SET name = $1, type = $2, config = $3, enabled = $4
		WHERE id = $5
	`, req.Name, req.Type, req.Config, req.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	_, err := s.db.Pool().Exec(ctx, `DELETE FROM notification_channels WHERE id = $1`, id)
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
