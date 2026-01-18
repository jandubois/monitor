package web

import (
	"encoding/json"
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

	// Check watcher health from heartbeat
	var lastSeen *time.Time
	var watcherVersion *string
	err := s.db.Pool().QueryRow(ctx, `
		SELECT last_seen_at, watcher_version FROM watcher_heartbeat WHERE id = 1
	`).Scan(&lastSeen, &watcherVersion)

	watcherHealthy := err == nil && lastSeen != nil && time.Since(*lastSeen) < 30*time.Second

	// Get recent failure count
	var recentFailures int
	s.db.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM probe_results
		WHERE status IN ('critical', 'unknown')
		AND executed_at > NOW() - INTERVAL '1 hour'
	`).Scan(&recentFailures)

	response := map[string]any{
		"watcher_healthy":  watcherHealthy,
		"watcher_last_seen": lastSeen,
		"recent_failures":  recentFailures,
	}
	if watcherVersion != nil {
		response["watcher_version"] = *watcherVersion
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleListProbeTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool().Query(ctx, `
		SELECT id, name, description, version, arguments, executable_path, registered_at, updated_at
		FROM probe_types
		ORDER BY name
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var probeTypes []map[string]any
	for rows.Next() {
		var id int
		var name, description, version, executablePath string
		var arguments map[string]any
		var registeredAt time.Time
		var updatedAt *time.Time

		if err := rows.Scan(&id, &name, &description, &version, &arguments, &executablePath, &registeredAt, &updatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		probeTypes = append(probeTypes, map[string]any{
			"id":              id,
			"name":            name,
			"description":     description,
			"version":         version,
			"arguments":       arguments,
			"executable_path": executablePath,
			"registered_at":   registeredAt,
			"updated_at":      updatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(probeTypes)
}

func (s *Server) handleDiscoverProbeTypes(w http.ResponseWriter, r *http.Request) {
	resp, err := s.callWatcher(r.Context(), http.MethodPost, "/discover")
	if err != nil {
		http.Error(w, "watcher unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleListProbeConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool().Query(ctx, `
		SELECT pc.id, pc.probe_type_id, pt.name as probe_type_name, pc.name, pc.enabled,
		       pc.arguments, pc.interval, pc.timeout_seconds, pc.notification_channels,
		       pc.created_at, pc.updated_at,
		       pr.status as last_status, pr.message as last_message, pr.executed_at as last_executed_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		LEFT JOIN LATERAL (
			SELECT status, message, executed_at
			FROM probe_results
			WHERE probe_config_id = pc.id
			ORDER BY executed_at DESC
			LIMIT 1
		) pr ON true
		ORDER BY pc.name
	`)
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
		var createdAt time.Time
		var updatedAt, lastExecutedAt *time.Time
		var lastStatus, lastMessage *string

		if err := rows.Scan(
			&id, &probeTypeID, &probeTypeName, &name, &enabled,
			&arguments, &interval, &timeoutSeconds, &notificationChannels,
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
			"created_at":            createdAt,
			"updated_at":            updatedAt,
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
		Name                 string         `json:"name"`
		Enabled              bool           `json:"enabled"`
		Arguments            map[string]any `json:"arguments"`
		Interval             string         `json:"interval"`
		TimeoutSeconds       int            `json:"timeout_seconds"`
		NotificationChannels []int          `json:"notification_channels"`
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
		INSERT INTO probe_configs (probe_type_id, name, enabled, arguments, interval, timeout_seconds, notification_channels)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, req.ProbeTypeID, req.Name, req.Enabled, req.Arguments, req.Interval, req.TimeoutSeconds, req.NotificationChannels).Scan(&id)
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
	var createdAt time.Time
	var updatedAt *time.Time

	err := s.db.Pool().QueryRow(ctx, `
		SELECT pc.id, pc.probe_type_id, pt.name, pc.name, pc.enabled, pc.arguments,
		       pc.interval, pc.timeout_seconds, pc.notification_channels, pc.created_at, pc.updated_at
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		WHERE pc.id = $1
	`, id).Scan(&id, &probeTypeID, &probeTypeName, &name, &enabled, &arguments,
		&interval, &timeoutSeconds, &notificationChannels, &createdAt, &updatedAt)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":                    id,
		"probe_type_id":         probeTypeID,
		"probe_type_name":       probeTypeName,
		"name":                  name,
		"enabled":               enabled,
		"arguments":             arguments,
		"interval":              interval,
		"timeout_seconds":       timeoutSeconds,
		"notification_channels": notificationChannels,
		"created_at":            createdAt,
		"updated_at":            updatedAt,
	})
}

func (s *Server) handleUpdateProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.Atoi(r.PathValue("id"))

	var req struct {
		Name                 string         `json:"name"`
		Enabled              bool           `json:"enabled"`
		Arguments            map[string]any `json:"arguments"`
		Interval             string         `json:"interval"`
		TimeoutSeconds       int            `json:"timeout_seconds"`
		NotificationChannels []int          `json:"notification_channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := s.db.Pool().Exec(ctx, `
		UPDATE probe_configs
		SET name = $1, enabled = $2, arguments = $3, interval = $4,
		    timeout_seconds = $5, notification_channels = $6, updated_at = NOW()
		WHERE id = $7
	`, req.Name, req.Enabled, req.Arguments, req.Interval, req.TimeoutSeconds, req.NotificationChannels, id)
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
	id := r.PathValue("id")
	resp, err := s.callWatcher(r.Context(), http.MethodPost, "/trigger/"+id)
	if err != nil {
		http.Error(w, "watcher unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleQueryResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configID := r.URL.Query().Get("config_id")
	status := r.URL.Query().Get("status")
	since := r.URL.Query().Get("since")
	limit := r.URL.Query().Get("limit")

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
		query += " AND pr.status = $" + strconv.Itoa(argNum)
		args = append(args, status)
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
	} else {
		query += " LIMIT 100"
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
