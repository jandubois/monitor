package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jandubois/monitor/internal/probe"
)

// Dispatcher manages notification channels and sends notifications.
type Dispatcher struct {
	pool *pgxpool.Pool

	mu       sync.RWMutex
	channels map[int]Channel
}

// NewDispatcher creates a new notification dispatcher.
func NewDispatcher(pool *pgxpool.Pool) *Dispatcher {
	return &Dispatcher{
		pool:     pool,
		channels: make(map[int]Channel),
	}
}

// LoadChannels loads notification channels from the database.
func (d *Dispatcher) LoadChannels(ctx context.Context) error {
	rows, err := d.pool.Query(ctx, `
		SELECT id, type, config FROM notification_channels WHERE enabled = true
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	d.mu.Lock()
	defer d.mu.Unlock()

	d.channels = make(map[int]Channel)

	for rows.Next() {
		var id int
		var channelType string
		var configJSON []byte

		if err := rows.Scan(&id, &channelType, &configJSON); err != nil {
			slog.Error("scan notification channel failed", "error", err)
			continue
		}

		channel, err := d.createChannel(channelType, configJSON)
		if err != nil {
			slog.Error("create notification channel failed", "type", channelType, "error", err)
			continue
		}

		d.channels[id] = channel
	}

	slog.Info("loaded notification channels", "count", len(d.channels))
	return nil
}

func (d *Dispatcher) createChannel(channelType string, configJSON []byte) (Channel, error) {
	switch channelType {
	case "ntfy":
		var cfg NtfyConfig
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, err
		}
		return NewNtfyChannel(cfg), nil
	case "pushover":
		var cfg PushoverConfig
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, err
		}
		return NewPushoverChannel(cfg), nil
	default:
		return nil, nil
	}
}

// NotifyStatusChange sends notifications for a status change.
func (d *Dispatcher) NotifyStatusChange(ctx context.Context, channelIDs []int, change *StatusChange) {
	msg := FormatStatusChange(change)

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, id := range channelIDs {
		channel, ok := d.channels[id]
		if !ok {
			continue
		}

		go func(ch Channel, chID int) {
			if err := ch.Send(ctx, msg); err != nil {
				slog.Error("notification send failed",
					"channel_id", chID,
					"channel_type", ch.Type(),
					"error", err,
				)
			} else {
				slog.Debug("notification sent",
					"channel_id", chID,
					"probe", change.ProbeName,
					"status", change.NewStatus,
				)
			}
		}(channel, id)
	}
}

// GetPreviousStatus retrieves the previous status for a probe config.
func (d *Dispatcher) GetPreviousStatus(ctx context.Context, configID int) (probe.Status, error) {
	var status string
	err := d.pool.QueryRow(ctx, `
		SELECT status FROM probe_results
		WHERE probe_config_id = $1
		ORDER BY executed_at DESC
		LIMIT 1 OFFSET 1
	`, configID).Scan(&status)
	if err != nil {
		return "", err
	}
	return probe.Status(status), nil
}
