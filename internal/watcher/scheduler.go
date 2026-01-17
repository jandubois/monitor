package watcher

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jankremlacek/monitor/internal/db"
)

// ProbeConfig represents a configured probe instance from the database.
type ProbeConfig struct {
	ID                   int
	ProbeTypeID          int
	Name                 string
	Enabled              bool
	Arguments            map[string]any
	Interval             time.Duration
	TimeoutSeconds       int
	NotificationChannels []int
	ExecutablePath       string
	LastExecutedAt       *time.Time
}

// Scheduler manages probe execution timing.
type Scheduler struct {
	db       *db.DB
	executor *Executor

	mu      sync.RWMutex
	configs map[int]*ProbeConfig
	timers  map[int]*time.Timer
}

// NewScheduler creates a new Scheduler.
func NewScheduler(database *db.DB, executor *Executor) *Scheduler {
	return &Scheduler{
		db:       database,
		executor: executor,
		configs:  make(map[int]*ProbeConfig),
		timers:   make(map[int]*time.Timer),
	}
}

// Run starts the scheduler loop.
func (s *Scheduler) Run(ctx context.Context) {
	// Initial load
	if err := s.Reload(ctx); err != nil {
		slog.Error("initial config load failed", "error", err)
	}

	// Check for missed runs on startup
	s.checkMissedRuns(ctx)

	<-ctx.Done()
	s.stopAllTimers()
}

// Reload reloads probe configurations from the database.
func (s *Scheduler) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing timers
	for _, timer := range s.timers {
		timer.Stop()
	}
	s.timers = make(map[int]*time.Timer)

	// Load configs from database
	rows, err := s.db.Pool().Query(ctx, `
		SELECT
			pc.id, pc.probe_type_id, pc.name, pc.enabled, pc.arguments,
			pc.interval, pc.timeout_seconds, pc.notification_channels,
			pt.executable_path,
			(SELECT executed_at FROM probe_results WHERE probe_config_id = pc.id ORDER BY executed_at DESC LIMIT 1)
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		WHERE pc.enabled = true
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.configs = make(map[int]*ProbeConfig)
	for rows.Next() {
		var cfg ProbeConfig
		var intervalStr string
		err := rows.Scan(
			&cfg.ID, &cfg.ProbeTypeID, &cfg.Name, &cfg.Enabled, &cfg.Arguments,
			&intervalStr, &cfg.TimeoutSeconds, &cfg.NotificationChannels,
			&cfg.ExecutablePath, &cfg.LastExecutedAt,
		)
		if err != nil {
			slog.Error("scan probe config failed", "error", err)
			continue
		}

		cfg.Interval, err = parseInterval(intervalStr)
		if err != nil {
			slog.Error("parse interval failed", "config", cfg.Name, "interval", intervalStr, "error", err)
			continue
		}

		s.configs[cfg.ID] = &cfg
		s.scheduleProbe(ctx, &cfg)
	}

	slog.Info("loaded probe configs", "count", len(s.configs))
	return nil
}

// TriggerImmediate runs a probe immediately.
func (s *Scheduler) TriggerImmediate(ctx context.Context, configIDStr string) error {
	configID, err := strconv.Atoi(configIDStr)
	if err != nil {
		return err
	}

	s.mu.RLock()
	cfg, ok := s.configs[configID]
	s.mu.RUnlock()

	if !ok {
		// Load from database if not in cache
		return s.runProbeByID(ctx, configID)
	}

	return s.executor.Execute(ctx, cfg)
}

func (s *Scheduler) scheduleProbe(ctx context.Context, cfg *ProbeConfig) {
	delay := s.calculateNextRun(cfg)
	slog.Debug("scheduling probe", "name", cfg.Name, "delay", delay)

	timer := time.AfterFunc(delay, func() {
		if err := s.executor.Execute(ctx, cfg); err != nil {
			slog.Error("probe execution failed", "name", cfg.Name, "error", err)
		}
		// Reschedule
		s.mu.Lock()
		s.scheduleProbe(ctx, cfg)
		s.mu.Unlock()
	})

	s.timers[cfg.ID] = timer
}

func (s *Scheduler) calculateNextRun(cfg *ProbeConfig) time.Duration {
	if cfg.LastExecutedAt == nil {
		// Never run, execute soon (with small jitter to avoid thundering herd)
		return time.Duration(cfg.ID%10) * time.Second
	}

	nextRun := cfg.LastExecutedAt.Add(cfg.Interval)
	delay := time.Until(nextRun)
	if delay < 0 {
		// Overdue, run immediately
		return 0
	}
	return delay
}

func (s *Scheduler) checkMissedRuns(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cfg := range s.configs {
		if cfg.LastExecutedAt == nil {
			continue
		}

		// Check if we missed any runs
		expectedRuns := time.Since(*cfg.LastExecutedAt) / cfg.Interval
		if expectedRuns > 1 {
			slog.Warn("detected missed runs",
				"probe", cfg.Name,
				"last_run", cfg.LastExecutedAt,
				"missed_count", int(expectedRuns)-1,
			)

			// Record missed run
			_, err := s.db.Pool().Exec(ctx, `
				INSERT INTO missed_runs (probe_config_id, scheduled_at, reason)
				VALUES ($1, $2, $3)
			`, cfg.ID, cfg.LastExecutedAt.Add(cfg.Interval), "watcher_down")
			if err != nil {
				slog.Error("failed to record missed run", "error", err)
			}
		}
	}
}

func (s *Scheduler) stopAllTimers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.timers {
		timer.Stop()
	}
}

func (s *Scheduler) runProbeByID(ctx context.Context, configID int) error {
	var cfg ProbeConfig
	var intervalStr string
	err := s.db.Pool().QueryRow(ctx, `
		SELECT
			pc.id, pc.probe_type_id, pc.name, pc.enabled, pc.arguments,
			pc.interval, pc.timeout_seconds, pc.notification_channels,
			pt.executable_path
		FROM probe_configs pc
		JOIN probe_types pt ON pt.id = pc.probe_type_id
		WHERE pc.id = $1
	`, configID).Scan(
		&cfg.ID, &cfg.ProbeTypeID, &cfg.Name, &cfg.Enabled, &cfg.Arguments,
		&intervalStr, &cfg.TimeoutSeconds, &cfg.NotificationChannels,
		&cfg.ExecutablePath,
	)
	if err != nil {
		return err
	}

	cfg.Interval, _ = parseInterval(intervalStr)
	return s.executor.Execute(ctx, &cfg)
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
