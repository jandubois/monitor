package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jandubois/monitor/internal/probe"
)

// Client communicates with the web service via HTTP.
type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewClient creates a new HTTP client for the web service.
func NewClient(baseURL, authToken string) *Client {
	return &Client{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RegisterRequest is sent on watcher startup.
type RegisterRequest struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Token       string              `json:"token"`
	CallbackURL string              `json:"callback_url,omitempty"`
	ProbeTypes  []RegisterProbeType `json:"probe_types"`
}

// RegisterProbeType describes a probe type available on this watcher.
type RegisterProbeType struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description"`
	Arguments      map[string]any `json:"arguments"`
	ExecutablePath string         `json:"executable_path"`
	Subcommand     string         `json:"subcommand,omitempty"`
}

// RegisterResponse is returned from registration.
type RegisterResponse struct {
	WatcherID        int  `json:"watcher_id"`
	RegisteredProbes int  `json:"registered_probes"`
	Approved         bool `json:"approved"`
}

// HeartbeatRequest is sent periodically.
type HeartbeatRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ResultRequest is sent when a probe completes.
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

// ProbeConfigResponse is returned when fetching configs.
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

// Register registers the watcher and its probe types with the web service.
// Registration uses the token in the request body rather than Authorization header.
func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.postNoAuth(ctx, "/api/push/register", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Heartbeat sends a heartbeat to the web service.
func (c *Client) Heartbeat(ctx context.Context, req *HeartbeatRequest) error {
	return c.post(ctx, "/api/push/heartbeat", req, nil)
}

// SendResult sends a probe result to the web service with retry on failure.
func (c *Client) SendResult(ctx context.Context, req *ResultRequest) error {
	return c.postWithRetry(ctx, "/api/push/result", req, nil)
}

// GetConfigs fetches probe configs assigned to this watcher.
func (c *Client) GetConfigs(ctx context.Context, watcherName string) ([]ProbeConfigResponse, error) {
	var configs []ProbeConfigResponse
	if err := c.get(ctx, "/api/push/configs/"+watcherName, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

// postWithRetry sends a POST request with exponential backoff retry.
// Retries up to 5 times over ~30 seconds for transient network failures.
func (c *Client) postWithRetry(ctx context.Context, path string, body any, response any) error {
	var lastErr error
	delays := []time.Duration{0, 1 * time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second}

	for attempt, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = c.post(ctx, path, body, response)
		if lastErr == nil {
			return nil
		}

		// Don't retry on context cancellation or client errors (4xx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		slog.Warn("request failed, retrying", "path", path, "attempt", attempt+1, "error", lastErr)
	}

	return fmt.Errorf("request failed after 5 attempts: %w", lastErr)
}

func (c *Client) post(ctx context.Context, path string, body any, response any) error {
	return c.doPost(ctx, path, body, response, true)
}

func (c *Client) postNoAuth(ctx context.Context, path string, body any, response any) error {
	return c.doPost(ctx, path, body, response, false)
}

func (c *Client) doPost(ctx context.Context, path string, body any, response any, includeAuth bool) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if includeAuth {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func (c *Client) get(ctx context.Context, path string, response any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// HTTPResultWriter sends probe results via HTTP to the web service.
type HTTPResultWriter struct {
	client      *Client
	watcherName string
}

// NewHTTPResultWriter creates a new HTTP-based result writer.
func NewHTTPResultWriter(client *Client, watcherName string) *HTTPResultWriter {
	return &HTTPResultWriter{
		client:      client,
		watcherName: watcherName,
	}
}

// WriteResult sends a probe result to the web service.
func (w *HTTPResultWriter) WriteResult(ctx context.Context, cfg *ProbeConfig, result *probe.Result, scheduledAt, executedAt time.Time, durationMs int) error {
	req := &ResultRequest{
		Watcher:       w.watcherName,
		ProbeConfigID: cfg.ID,
		Status:        string(result.Status),
		Message:       result.Message,
		Metrics:       result.Metrics,
		Data:          result.Data,
		DurationMs:    durationMs,
		NextRun:       result.NextRun,
		ScheduledAt:   scheduledAt,
		ExecutedAt:    executedAt,
	}

	return w.client.SendResult(ctx, req)
}
