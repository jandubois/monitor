package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var metaWatcherCmd = &cobra.Command{
	Use:   "meta-watcher",
	Short: "Monitor the monitoring system",
	Long: `The meta-watcher monitors all registered watchers and reports their
health status as probe results. It detects connection issues, version
changes, and other watcher events.

Requires AUTH_TOKEN for privileged API access.`,
	RunE: runMetaWatcher,
}

func init() {
	rootCmd.AddCommand(metaWatcherCmd)

	metaWatcherCmd.Flags().String("web-url", "http://localhost:8080", "Web service URL")
	metaWatcherCmd.Flags().String("token", "", "AUTH_TOKEN for privileged API access (or AUTH_TOKEN env)")
	metaWatcherCmd.Flags().Duration("threshold", time.Minute, "Duration without heartbeat before unhealthy")
	metaWatcherCmd.Flags().Duration("interval", 30*time.Second, "How often to check watchers")
}

type metaWatcher struct {
	webURL    string
	token     string
	threshold time.Duration
	interval  time.Duration
	client    *http.Client

	// Registration state
	watcherID   int
	probeTypeID int

	// Track probe configs for each monitored watcher
	probeConfigs map[int]int // watcher_id -> probe_config_id
}

type watcherInfo struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Healthy        bool      `json:"healthy"`
	Version        string    `json:"version"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	ProbeTypeCount int       `json:"probe_type_count"`
	Approved       bool      `json:"approved"`
	Paused         bool      `json:"paused"`
}

type watcherEvent struct {
	ID           int            `json:"id"`
	WatcherID    int            `json:"watcher_id"`
	WatcherName  string         `json:"watcher_name"`
	Timestamp    string         `json:"timestamp"`
	EventType    string         `json:"event_type"`
	Severity     string         `json:"severity"`
	Summary      string         `json:"summary"`
	Details      map[string]any `json:"details"`
	Acknowledged bool           `json:"acknowledged"`
}

type probeConfig struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	ProbeTypeName string `json:"probe_type_name"`
	WatcherID     *int   `json:"watcher_id"`
	WatcherName   string `json:"watcher_name"`
}

type registerRequest struct {
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	Token      string           `json:"token"`
	ProbeTypes []registerProbe  `json:"probe_types"`
}

type registerProbe struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Description     string         `json:"description"`
	Arguments       map[string]any `json:"arguments"`
	Output          map[string]any `json:"output,omitempty"`
	DefaultInterval string         `json:"default_interval,omitempty"`
	ExecutablePath  string         `json:"executable_path"`
	Subcommand      string         `json:"subcommand,omitempty"`
}

type registerResponse struct {
	WatcherID        int  `json:"watcher_id"`
	RegisteredProbes int  `json:"registered_probes"`
	Approved         bool `json:"approved"`
}

type probeType struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	WatcherID int    `json:"watcher_id"`
}

type probeConfigCreate struct {
	ProbeTypeID    int            `json:"probe_type_id"`
	WatcherID      int            `json:"watcher_id"`
	Name           string         `json:"name"`
	Enabled        bool           `json:"enabled"`
	Arguments      map[string]any `json:"arguments"`
	Interval       string         `json:"interval"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}

type probeConfigResponse struct {
	ID int `json:"id"`
}

func runMetaWatcher(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutdown signal received")
		cancel()
	}()

	webURL, _ := cmd.Flags().GetString("web-url")
	token, _ := cmd.Flags().GetString("token")
	threshold, _ := cmd.Flags().GetDuration("threshold")
	interval, _ := cmd.Flags().GetDuration("interval")

	if token == "" {
		token = os.Getenv("AUTH_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("auth token required (--token or AUTH_TOKEN)")
	}

	mw := &metaWatcher{
		webURL:       webURL,
		token:        token,
		threshold:    threshold,
		interval:     interval,
		client:       &http.Client{Timeout: 30 * time.Second},
		probeConfigs: make(map[int]int),
	}

	slog.Info("meta-watcher starting", "web_url", webURL, "threshold", threshold, "interval", interval)

	// Register as a watcher
	if err := mw.register(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	slog.Info("registered as watcher", "watcher_id", mw.watcherID, "probe_type_id", mw.probeTypeID)

	// Initial check
	if err := mw.check(ctx); err != nil {
		slog.Error("initial check failed", "error", err)
	}

	// Run periodic checks
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("meta-watcher shutting down")
			return nil
		case <-ticker.C:
			if err := mw.check(ctx); err != nil {
				slog.Error("check failed", "error", err)
			}
		}
	}
}

func (mw *metaWatcher) register(ctx context.Context) error {
	req := registerRequest{
		Name:    "meta-watcher",
		Version: "1.0.0",
		Token:   mw.token,
		ProbeTypes: []registerProbe{
			{
				Name:            "watcher-health",
				Version:         "1.0.0",
				Description:     "Monitor watcher health and events",
				DefaultInterval: mw.interval.String(),
				ExecutablePath:  "internal",
				Arguments: map[string]any{
					"required": map[string]any{
						"watcher_id": map[string]any{
							"type":        "number",
							"description": "ID of the watcher to monitor",
						},
					},
					"optional": map[string]any{},
				},
				Output: map[string]any{
					"metrics": map[string]any{
						"last_seen_seconds_ago":   map[string]any{"type": "number", "unit": "seconds", "description": "Seconds since last heartbeat"},
						"unacknowledged_events":   map[string]any{"type": "integer", "unit": "count", "description": "Unacknowledged event count"},
					},
					"data": map[string]any{
						"watcher_name":    map[string]any{"type": "string", "description": "Watcher name"},
						"watcher_version": map[string]any{"type": "string", "description": "Watcher version"},
						"probe_type_count": map[string]any{"type": "integer", "description": "Number of probe types"},
					},
				},
			},
		},
	}

	var resp registerResponse
	if err := mw.postJSON(ctx, "/api/push/register", req, &resp, false); err != nil {
		return err
	}

	mw.watcherID = resp.WatcherID

	// Get our probe type ID
	probeTypes, err := mw.fetchProbeTypes(ctx, mw.watcherID)
	if err != nil {
		return fmt.Errorf("fetch probe types: %w", err)
	}
	for _, pt := range probeTypes {
		if pt.Name == "watcher-health" {
			mw.probeTypeID = pt.ID
			break
		}
	}
	if mw.probeTypeID == 0 {
		return fmt.Errorf("watcher-health probe type not found after registration")
	}

	return nil
}

func (mw *metaWatcher) check(ctx context.Context) error {
	// Fetch all watchers
	watchers, err := mw.fetchWatchers(ctx)
	if err != nil {
		return fmt.Errorf("fetch watchers: %w", err)
	}

	// Fetch unacknowledged events
	events, err := mw.fetchEvents(ctx)
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}

	// Group events by watcher
	eventsByWatcher := make(map[int][]watcherEvent)
	for _, e := range events {
		eventsByWatcher[e.WatcherID] = append(eventsByWatcher[e.WatcherID], e)
	}

	// Track current watcher IDs
	currentIDs := make(map[int]bool)

	// Check each watcher (excluding ourselves)
	for _, w := range watchers {
		if w.ID == mw.watcherID {
			continue
		}
		currentIDs[w.ID] = true

		// Ensure we have a probe config for this watcher
		configID, err := mw.ensureProbeConfig(ctx, w)
		if err != nil {
			slog.Error("failed to ensure probe config", "watcher", w.Name, "error", err)
			continue
		}

		watcherEvents := eventsByWatcher[w.ID]
		status, summary, message := mw.evaluateWatcher(w, watcherEvents)

		// Log the check result
		logLevel := slog.LevelInfo
		if status == "critical" {
			logLevel = slog.LevelError
		} else if status == "warning" {
			logLevel = slog.LevelWarn
		}
		slog.Log(ctx, logLevel, "watcher status",
			"watcher", w.Name,
			"status", status,
			"summary", summary,
			"events", len(watcherEvents))

		// Push probe result
		metrics := map[string]any{
			"last_seen_seconds_ago": time.Since(w.LastSeenAt).Seconds(),
			"unacknowledged_events": len(watcherEvents),
		}
		data := map[string]any{
			"watcher_name":     w.Name,
			"watcher_version":  w.Version,
			"probe_type_count": w.ProbeTypeCount,
		}

		if err := mw.pushResult(ctx, configID, status, summary, message, metrics, data); err != nil {
			slog.Error("failed to push result", "watcher", w.Name, "error", err)
		}
	}

	// Remove probe configs for watchers that no longer exist
	for watcherID := range mw.probeConfigs {
		if !currentIDs[watcherID] {
			slog.Info("watcher removed, cleaning up", "watcher_id", watcherID)
			delete(mw.probeConfigs, watcherID)
		}
	}

	return nil
}

func (mw *metaWatcher) evaluateWatcher(w watcherInfo, events []watcherEvent) (status, summary, message string) {
	// Check for critical conditions
	if !w.Approved {
		return "warning", "Watcher pending approval", fmt.Sprintf("Watcher '%s' is registered but not yet approved.", w.Name)
	}

	if w.Paused {
		return "ok", "Watcher paused", fmt.Sprintf("Watcher '%s' is paused.", w.Name)
	}

	// Check last seen time
	lastSeenAgo := time.Since(w.LastSeenAt)
	if lastSeenAgo > mw.threshold {
		return "critical", fmt.Sprintf("No heartbeat for %s", lastSeenAgo.Round(time.Second)),
			fmt.Sprintf("Watcher '%s' last seen %s ago (threshold: %s)", w.Name, lastSeenAgo.Round(time.Second), mw.threshold)
	}

	// Check for connection_lost events
	for _, e := range events {
		if e.EventType == "connection_lost" && !e.Acknowledged {
			return "critical", "Connection lost",
				fmt.Sprintf("Watcher '%s' has unacknowledged connection_lost event: %s", w.Name, e.Summary)
		}
	}

	// Check for warning events
	var warnings []string
	for _, e := range events {
		if e.Severity == "warning" && !e.Acknowledged {
			warnings = append(warnings, e.Summary)
		}
	}
	if len(warnings) > 0 {
		return "warning", fmt.Sprintf("%d warning event(s)", len(warnings)),
			fmt.Sprintf("Watcher '%s' has warning events:\n- %s", w.Name, warnings[0])
	}

	// All good
	return "ok", fmt.Sprintf("Healthy (last seen %s ago)", lastSeenAgo.Round(time.Second)),
		fmt.Sprintf("Watcher '%s' is healthy. Version: %s, Probes: %d", w.Name, w.Version, w.ProbeTypeCount)
}

func (mw *metaWatcher) ensureProbeConfig(ctx context.Context, w watcherInfo) (int, error) {
	// Check if we already have a probe config for this watcher
	if configID, exists := mw.probeConfigs[w.ID]; exists {
		return configID, nil
	}

	// Create a new probe config
	cfg := probeConfigCreate{
		ProbeTypeID:    mw.probeTypeID,
		WatcherID:      mw.watcherID,
		Name:           fmt.Sprintf("Monitor: %s", w.Name),
		Enabled:        true,
		Arguments:      map[string]any{"watcher_id": w.ID},
		Interval:       mw.interval.String(),
		TimeoutSeconds: 30,
	}

	var resp probeConfigResponse
	if err := mw.postJSON(ctx, "/api/probe-configs", cfg, &resp, true); err != nil {
		return 0, err
	}

	mw.probeConfigs[w.ID] = resp.ID
	slog.Info("created probe config for watcher", "watcher", w.Name, "config_id", resp.ID)

	return resp.ID, nil
}

func (mw *metaWatcher) fetchWatchers(ctx context.Context) ([]watcherInfo, error) {
	var watchers []watcherInfo
	if err := mw.getJSON(ctx, "/api/watchers", &watchers); err != nil {
		return nil, err
	}
	return watchers, nil
}

func (mw *metaWatcher) fetchEvents(ctx context.Context) ([]watcherEvent, error) {
	var events []watcherEvent
	if err := mw.getJSON(ctx, "/api/watcher-events?unacknowledged=true", &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (mw *metaWatcher) fetchProbeTypes(ctx context.Context, watcherID int) ([]probeType, error) {
	var probeTypes []probeType
	if err := mw.getJSON(ctx, fmt.Sprintf("/api/probe-types?watcher=%d", watcherID), &probeTypes); err != nil {
		return nil, err
	}
	return probeTypes, nil
}

func (mw *metaWatcher) pushResult(ctx context.Context, configID int, status, summary, message string, metrics, data map[string]any) error {
	body := map[string]any{
		"watcher":         "meta-watcher",
		"probe_config_id": configID,
		"status":          status,
		"summary":         summary,
		"message":         message,
		"metrics":         metrics,
		"data":            data,
		"duration_ms":     0,
		"scheduled_at":    time.Now().UTC(),
		"executed_at":     time.Now().UTC(),
	}

	return mw.postJSON(ctx, "/api/push/result", body, nil, true)
}

func (mw *metaWatcher) getJSON(ctx context.Context, path string, response any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", mw.webURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+mw.token)

	resp, err := mw.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return err
		}
	}

	return nil
}

func (mw *metaWatcher) postJSON(ctx context.Context, path string, body any, response any, useAuth bool) error {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mw.webURL+path, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if useAuth {
		req.Header.Set("Authorization", "Bearer "+mw.token)
	}

	resp, err := mw.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return err
		}
	}

	return nil
}
