package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NtfyChannel sends notifications via ntfy.sh or self-hosted ntfy.
type NtfyChannel struct {
	ServerURL string
	Topic     string
	Token     string // Optional auth token
	client    *http.Client
}

// NtfyConfig is the JSON configuration for an ntfy channel.
type NtfyConfig struct {
	ServerURL string `json:"server_url"`
	Topic     string `json:"topic"`
	Token     string `json:"token,omitempty"`
}

// NewNtfyChannel creates a new ntfy notification channel.
func NewNtfyChannel(cfg NtfyConfig) *NtfyChannel {
	serverURL := cfg.ServerURL
	if serverURL == "" {
		serverURL = "https://ntfy.sh"
	}
	return &NtfyChannel{
		ServerURL: strings.TrimSuffix(serverURL, "/"),
		Topic:     cfg.Topic,
		Token:     cfg.Token,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Type returns the channel type.
func (n *NtfyChannel) Type() string {
	return "ntfy"
}

// Send sends a notification via ntfy.
func (n *NtfyChannel) Send(ctx context.Context, msg *Message) error {
	url := fmt.Sprintf("%s/%s", n.ServerURL, n.Topic)

	payload := map[string]any{
		"title":   msg.Title,
		"message": msg.Body,
	}

	if len(msg.Tags) > 0 {
		payload["tags"] = msg.Tags
	}

	switch msg.Priority {
	case PriorityLow:
		payload["priority"] = 2
	case PriorityNormal:
		payload["priority"] = 3
	case PriorityHigh:
		payload["priority"] = 4
	case PriorityUrgent:
		payload["priority"] = 5
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}
