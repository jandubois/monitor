package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jandubois/monitor/internal/config"
	"github.com/jandubois/monitor/internal/watcher"
	"github.com/spf13/cobra"
)

var watcherCmd = &cobra.Command{
	Use:   "watcher",
	Short: "Run the background scheduler service",
	Long: `The watcher service schedules and executes probes, pushing results
to the central web service via HTTP.

Each watcher must have a unique name and requires the web service URL
and authentication token to communicate.`,
	RunE: runWatcher,
}

func init() {
	rootCmd.AddCommand(watcherCmd)

	watcherCmd.Flags().String("name", "", "Unique watcher name (required)")
	watcherCmd.Flags().String("push-url", "http://localhost:8080", "URL of the web service")
	watcherCmd.Flags().String("callback-url", "", "URL where web service can reach this watcher (for triggers)")
	watcherCmd.Flags().String("auth-token", "", "Authentication token (or AUTH_TOKEN env var)")
	watcherCmd.Flags().String("probes-dir", "./probes", "Directory containing probe executables")
	watcherCmd.Flags().Int("max-concurrent", 10, "Maximum concurrent probe executions")
	watcherCmd.Flags().Int("api-port", 8081, "Port for local watcher API (health check, reload)")

	watcherCmd.MarkFlagRequired("name")
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

	name, _ := cmd.Flags().GetString("name")
	pushURL, _ := cmd.Flags().GetString("push-url")
	callbackURL, _ := cmd.Flags().GetString("callback-url")
	authToken, _ := cmd.Flags().GetString("auth-token")
	probesDir, _ := cmd.Flags().GetString("probes-dir")
	maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent")
	apiPort, _ := cmd.Flags().GetInt("api-port")

	// Auth token from env if not provided via flag
	if authToken == "" {
		authToken = os.Getenv("AUTH_TOKEN")
	}
	if authToken == "" {
		return fmt.Errorf("auth token required (use --auth-token or AUTH_TOKEN env var)")
	}

	// Load configuration
	cfg := &config.WatcherConfig{
		Name:          name,
		ProbesDir:     probesDir,
		MaxConcurrent: maxConcurrent,
		APIPort:       apiPort,
		PushURL:       pushURL,
		CallbackURL:   callbackURL,
		AuthToken:     authToken,
	}

	// Create and run watcher
	w, err := watcher.New(cfg)
	if err != nil {
		return fmt.Errorf("watcher initialization failed: %w", err)
	}

	slog.Info("starting watcher",
		"name", name,
		"push_url", pushURL,
		"probes_dir", probesDir,
		"max_concurrent", maxConcurrent,
	)
	return w.Run(ctx)
}
