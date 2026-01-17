package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PushoverChannel sends notifications via Pushover.
type PushoverChannel struct {
	APIToken string
	UserKey  string
	client   *http.Client
}

// PushoverConfig is the JSON configuration for a Pushover channel.
type PushoverConfig struct {
	APIToken string `json:"api_token"`
	UserKey  string `json:"user_key"`
}

// NewPushoverChannel creates a new Pushover notification channel.
func NewPushoverChannel(cfg PushoverConfig) *PushoverChannel {
	return &PushoverChannel{
		APIToken: cfg.APIToken,
		UserKey:  cfg.UserKey,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Type returns the channel type.
func (p *PushoverChannel) Type() string {
	return "pushover"
}

// Send sends a notification via Pushover.
func (p *PushoverChannel) Send(ctx context.Context, msg *Message) error {
	data := url.Values{
		"token":   {p.APIToken},
		"user":    {p.UserKey},
		"title":   {msg.Title},
		"message": {msg.Body},
	}

	switch msg.Priority {
	case PriorityLow:
		data.Set("priority", "-1")
	case PriorityNormal:
		data.Set("priority", "0")
	case PriorityHigh:
		data.Set("priority", "1")
	case PriorityUrgent:
		data.Set("priority", "2")
		data.Set("retry", "60")
		data.Set("expire", "3600")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.pushover.net/1/messages.json",
		strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("pushover returned status %d", resp.StatusCode)
	}

	return nil
}
