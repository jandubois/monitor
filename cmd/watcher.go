package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jankremlacek/monitor/internal/config"
	"github.com/jankremlacek/monitor/internal/db"
	"github.com/jankremlacek/monitor/internal/watcher"
	"github.com/spf13/cobra"
)

var watcherCmd = &cobra.Command{
	Use:   "watcher",
	Short: "Run the background scheduler service",
	Long: `The watcher service schedules and executes probes, stores results,
and dispatches notifications on status changes.`,
	RunE: runWatcher,
}

func init() {
	rootCmd.AddCommand(watcherCmd)

	watcherCmd.Flags().String("probes-dir", "./probes", "Directory containing probe executables")
	watcherCmd.Flags().Int("max-concurrent", 10, "Maximum concurrent probe executions")
	watcherCmd.Flags().Int("api-port", 8081, "Port for watcher API (trigger, reload)")
}

func runWatcher(cmd *cobra.Command, args []string) error {
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

	databaseURL := getDatabaseURL(cmd)
	probesDir, _ := cmd.Flags().GetString("probes-dir")
	maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent")
	apiPort, _ := cmd.Flags().GetInt("api-port")

	// Connect to database
	database, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer database.Close()

	// Load configuration
	cfg := &config.WatcherConfig{
		ProbesDir:     probesDir,
		MaxConcurrent: maxConcurrent,
		APIPort:       apiPort,
	}

	// Create and run watcher
	w, err := watcher.New(database, cfg)
	if err != nil {
		return fmt.Errorf("watcher initialization failed: %w", err)
	}

	slog.Info("starting watcher", "probes_dir", probesDir, "max_concurrent", maxConcurrent)
	return w.Run(ctx)
}
