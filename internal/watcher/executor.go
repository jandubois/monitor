package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/jandubois/monitor/internal/probe"
)

// Executor runs probes as subprocesses.
type Executor struct {
	probesDir     string
	maxConcurrent int
	semaphore     chan struct{}

	mu           sync.Mutex
	resultWriter ResultWriter
}

// ResultWriter persists probe results.
type ResultWriter interface {
	WriteResult(ctx context.Context, cfg *ProbeConfig, result *probe.Result, scheduledAt, executedAt time.Time, durationMs int) error
}

// NewExecutor creates a new Executor.
func NewExecutor(maxConcurrent int, probesDir string) *Executor {
	return &Executor{
		probesDir:     probesDir,
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

// SetResultWriter sets the result writer for persisting results.
func (e *Executor) SetResultWriter(w ResultWriter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resultWriter = w
}

// Execute runs a probe and stores the result.
func (e *Executor) Execute(ctx context.Context, cfg *ProbeConfig) error {
	// Acquire semaphore
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}

	scheduledAt := time.Now()
	result, duration := e.runProbe(ctx, cfg)
	executedAt := time.Now()

	slog.Info("probe executed",
		"name", cfg.Name,
		"status", result.Status,
		"duration_ms", duration.Milliseconds(),
		"message", result.Message,
	)

	e.mu.Lock()
	writer := e.resultWriter
	e.mu.Unlock()

	if writer != nil {
		if err := writer.WriteResult(ctx, cfg, result, scheduledAt, executedAt, int(duration.Milliseconds())); err != nil {
			slog.Error("failed to write result", "probe", cfg.Name, "error", err)
			return err
		}
	}

	return nil
}

func (e *Executor) runProbe(ctx context.Context, cfg *ProbeConfig) (*probe.Result, time.Duration) {
	start := time.Now()

	// Build command arguments
	args := buildArgs(cfg.Arguments)

	// If subcommand is set, prepend it to args
	if cfg.Subcommand != "" {
		args = append([]string{cfg.Subcommand}, args...)
	}

	// Create timeout context
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, cfg.ExecutablePath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		// Check if it was a timeout
		if timeoutCtx.Err() == context.DeadlineExceeded {
			// Try graceful shutdown with SIGTERM
			if cmd.Process != nil {
				cmd.Process.Signal(syscall.SIGTERM)
				time.Sleep(5 * time.Second)
				cmd.Process.Kill()
			}
			return &probe.Result{
				Status:  probe.StatusUnknown,
				Message: fmt.Sprintf("probe timed out after %s", timeout),
			}, duration
		}

		return &probe.Result{
			Status:  probe.StatusUnknown,
			Message: fmt.Sprintf("probe execution failed: %v, stderr: %s", err, stderr.String()),
		}, duration
	}

	// Parse JSON output
	var result probe.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Message: fmt.Sprintf("failed to parse probe output: %v, stdout: %s", err, stdout.String()),
		}, duration
	}

	return &result, duration
}

func buildArgs(arguments map[string]any) []string {
	var args []string
	for key, value := range arguments {
		// Use --key=value format for boolean flags (Go's flag package requires this)
		// Also use it for all other types for consistency
		args = append(args, fmt.Sprintf("--%s=%v", key, value))
	}
	return args
}
