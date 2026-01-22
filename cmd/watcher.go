package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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

The watcher name defaults to the hostname. Use --name to override.`,
	RunE: runWatcher,
}

func init() {
	rootCmd.AddCommand(watcherCmd)

	watcherCmd.Flags().String("name", "", "Unique watcher name (defaults to hostname)")
	watcherCmd.Flags().String("push-url", "http://localhost:8080", "URL of the web service")
	watcherCmd.Flags().String("callback-url", "", "URL where web service can reach this watcher (for triggers)")
	watcherCmd.Flags().String("probes-dir", "./probes", "Directory containing probe executables")
	watcherCmd.Flags().Int("max-concurrent", 10, "Maximum concurrent probe executions")
	watcherCmd.Flags().Int("api-port", 8081, "Port for local watcher API (health check, reload)")
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
	probesDir, _ := cmd.Flags().GetString("probes-dir")
	maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent")
	apiPort, _ := cmd.Flags().GetInt("api-port")

	// Default name to hostname (without domain)
	if name == "" {
		name = getShortHostname()
	}

	// Load or create watcher token
	authToken, err := watcher.LoadOrCreateToken(name)
	if err != nil {
		return fmt.Errorf("load or create token: %w", err)
	}
	slog.Debug("loaded watcher token", "name", name)

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

func getShortHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	// Strip domain suffix
	if idx := strings.Index(hostname, "."); idx != -1 {
		hostname = hostname[:idx]
	}
	return hostname
}
