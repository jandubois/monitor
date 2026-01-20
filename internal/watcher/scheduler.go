package watcher

import (
	"context"
	"log/slog"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// ProbeConfig represents a configured probe instance.
type ProbeConfig struct {
	ID                   int
	Name                 string
	ExecutablePath       string
	Arguments            map[string]any
	Interval             time.Duration
	TimeoutSeconds       int
	NextRunAt            *time.Time
	NotificationChannels []int // Kept for compatibility with ResultWriter interface
}

// Scheduler manages probe execution timing.
type Scheduler struct {
	client      *Client
	executor    *Executor
	watcherName string

	mu      sync.RWMutex
	configs map[int]*ProbeConfig
	timers  map[int]*time.Timer
}

// NewScheduler creates a new Scheduler.
func NewScheduler(client *Client, executor *Executor, watcherName string) *Scheduler {
	return &Scheduler{
		client:      client,
		executor:    executor,
		watcherName: watcherName,
		configs:     make(map[int]*ProbeConfig),
		timers:      make(map[int]*time.Timer),
	}
}

// Run starts the scheduler loop.
func (s *Scheduler) Run(ctx context.Context) {
	// Initial load
	if err := s.Reload(ctx); err != nil {
		slog.Error("initial config load failed", "error", err)
	}

	// Periodic config refresh to pick up new configs or trigger requests
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.stopAllTimers()
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil {
				slog.Error("config reload failed", "error", err)
			}
		}
	}
}

// Reload reloads probe configurations from the web service.
func (s *Scheduler) Reload(ctx context.Context) error {
	// Fetch configs from web service
	configs, err := s.client.GetConfigs(ctx, s.watcherName)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Track which configs we've seen
	seen := make(map[int]bool)

	for _, cfg := range configs {
		seen[cfg.ID] = true

		interval, err := parseInterval(cfg.Interval)
		if err != nil {
			slog.Error("parse interval failed", "config", cfg.Name, "interval", cfg.Interval, "error", err)
			continue
		}

		probeConfig := &ProbeConfig{
			ID:             cfg.ID,
			Name:           cfg.Name,
			ExecutablePath: cfg.ExecutablePath,
			Arguments:      cfg.Arguments,
			Interval:       interval,
			TimeoutSeconds: cfg.TimeoutSeconds,
			NextRunAt:      cfg.NextRunAt,
		}

		// Check if config changed or is new
		existing, exists := s.configs[cfg.ID]
		if exists && !s.configChanged(existing, probeConfig) {
			continue
		}

		// Stop existing timer if any
		if timer, ok := s.timers[cfg.ID]; ok {
			timer.Stop()
			delete(s.timers, cfg.ID)
		}

		s.configs[cfg.ID] = probeConfig
		s.scheduleProbe(ctx, probeConfig)
	}

	// Remove configs that are no longer assigned to us
	for id, timer := range s.timers {
		if !seen[id] {
			timer.Stop()
			delete(s.timers, id)
			delete(s.configs, id)
			slog.Debug("removed config", "id", id)
		}
	}

	slog.Debug("loaded probe configs", "count", len(configs))
	return nil
}

// TriggerImmediate runs a probe immediately with fresh config.
// Execution happens asynchronously; this returns immediately after scheduling.
func (s *Scheduler) TriggerImmediate(ctx context.Context, configIDStr string) error {
	configID, err := strconv.Atoi(configIDStr)
	if err != nil {
		return err
	}

	// Reload configs to get the latest changes before executing
	if err := s.Reload(ctx); err != nil {
		slog.Warn("failed to reload before trigger", "error", err)
	}

	s.mu.RLock()
	cfg, ok := s.configs[configID]
	s.mu.RUnlock()

	if !ok {
		return nil // Config not assigned to this watcher
	}

	// Run asynchronously so the HTTP trigger returns immediately
	go func() {
		if err := s.executor.Execute(context.Background(), cfg); err != nil {
			slog.Error("triggered probe execution failed", "name", cfg.Name, "error", err)
		}
	}()

	return nil
}

func (s *Scheduler) scheduleProbe(ctx context.Context, cfg *ProbeConfig) {
	delay := s.calculateNextRun(cfg)
	slog.Debug("scheduling probe", "name", cfg.Name, "delay", delay)

	timer := time.AfterFunc(delay, func() {
		if err := s.executor.Execute(ctx, cfg); err != nil {
			slog.Error("probe execution failed", "name", cfg.Name, "error", err)
		}
		// Reschedule with interval (next_run_at will be updated via web service)
		s.mu.Lock()
		cfg.NextRunAt = nil // Clear next_run_at, will be recalculated based on interval
		s.scheduleProbe(ctx, cfg)
		s.mu.Unlock()
	})

	s.timers[cfg.ID] = timer
}

func (s *Scheduler) calculateNextRun(cfg *ProbeConfig) time.Duration {
	// If next_run_at is set (either from web service or probe result), use it
	if cfg.NextRunAt != nil {
		delay := time.Until(*cfg.NextRunAt)
		if delay < 0 {
			// Overdue, run immediately
			return 0
		}
		return delay
	}

	// Use the configured interval
	return cfg.Interval
}

func (s *Scheduler) configChanged(old, new *ProbeConfig) bool {
	if old.ExecutablePath != new.ExecutablePath {
		return true
	}
	if old.Interval != new.Interval {
		return true
	}
	if old.TimeoutSeconds != new.TimeoutSeconds {
		return true
	}
	if !reflect.DeepEqual(old.Arguments, new.Arguments) {
		return true
	}
	// Check if next_run_at changed and is in the past (immediate run requested)
	if new.NextRunAt != nil && (old.NextRunAt == nil || !new.NextRunAt.Equal(*old.NextRunAt)) {
		if time.Until(*new.NextRunAt) <= 0 {
			return true
		}
	}
	return false
}

func (s *Scheduler) stopAllTimers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.timers {
		timer.Stop()
	}
}

// parseInterval parses interval strings like "5m", "1h", "1d".
func parseInterval(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, nil
	}

	value, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, err
	}

	switch s[len(s)-1] {
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
