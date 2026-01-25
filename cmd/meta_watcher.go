package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	// Track known watchers to detect additions/removals
	knownWatchers map[int]string // id -> name
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
		webURL:        webURL,
		token:         token,
		threshold:     threshold,
		interval:      interval,
		client:        &http.Client{Timeout: 30 * time.Second},
		knownWatchers: make(map[int]string),
	}

	slog.Info("meta-watcher starting", "web_url", webURL, "threshold", threshold, "interval", interval)

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

	// Check each watcher and log status
	for _, w := range watchers {
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

		// Track new watchers
		if _, known := mw.knownWatchers[w.ID]; !known {
			slog.Info("discovered watcher", "name", w.Name, "id", w.ID)
			mw.knownWatchers[w.ID] = w.Name
		}

		// In a full implementation, we would push probe results here
		// For now, we just log the status
		_ = message
	}

	// Detect removed watchers
	currentIDs := make(map[int]bool)
	for _, w := range watchers {
		currentIDs[w.ID] = true
	}
	for id, name := range mw.knownWatchers {
		if !currentIDs[id] {
			slog.Info("watcher removed", "name", name, "id", id)
			delete(mw.knownWatchers, id)
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

func (mw *metaWatcher) fetchWatchers(ctx context.Context) ([]watcherInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", mw.webURL+"/api/watchers", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mw.token)

	resp, err := mw.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var watchers []watcherInfo
	if err := json.NewDecoder(resp.Body).Decode(&watchers); err != nil {
		return nil, err
	}

	return watchers, nil
}

func (mw *metaWatcher) fetchEvents(ctx context.Context) ([]watcherEvent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", mw.webURL+"/api/watcher-events?unacknowledged=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mw.token)

	resp, err := mw.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var events []watcherEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	return events, nil
}

// pushResult sends a probe result to the web service (for future use).
func (mw *metaWatcher) pushResult(ctx context.Context, configID int, status, summary, message string, metrics map[string]any) error {
	body := map[string]any{
		"watcher":         "meta-watcher",
		"probe_config_id": configID,
		"status":          status,
		"summary":         summary,
		"message":         message,
		"metrics":         metrics,
		"duration_ms":     0,
		"scheduled_at":    time.Now().UTC(),
		"executed_at":     time.Now().UTC(),
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", mw.webURL+"/api/push/result", bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+mw.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := mw.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}
