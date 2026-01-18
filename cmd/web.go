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
	"github.com/jankremlacek/monitor/internal/web"
	"github.com/spf13/cobra"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the web backend server",
	Long: `The web server provides a REST API for the React SPA and serves
the frontend static files.`,
	RunE: runWeb,
}

func init() {
	rootCmd.AddCommand(webCmd)

	webCmd.Flags().Int("port", 8080, "Port to listen on")
	webCmd.Flags().String("auth-token", "", "Authentication token (or AUTH_TOKEN env)")
	webCmd.Flags().String("watcher-url", "http://localhost:8081", "Watcher API URL for trigger/reload")
}

func runWeb(cmd *cobra.Command, args []string) error {
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
	port, _ := cmd.Flags().GetInt("port")
	authToken, _ := cmd.Flags().GetString("auth-token")
	watcherURL, _ := cmd.Flags().GetString("watcher-url")

	if authToken == "" {
		authToken = os.Getenv("AUTH_TOKEN")
	}
	if authToken == "" {
		return fmt.Errorf("auth token required (--auth-token or AUTH_TOKEN)")
	}
	if watcherURL == "http://localhost:8081" {
		if url := os.Getenv("WATCHER_URL"); url != "" {
			watcherURL = url
		}
	}

	// Connect to database
	database, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer database.Close()

	cfg := &config.WebConfig{
		Port:       port,
		AuthToken:  authToken,
		WatcherURL: watcherURL,
	}

	server, err := web.NewServer(database, cfg)
	if err != nil {
		return fmt.Errorf("web server initialization failed: %w", err)
	}

	slog.Info("starting web server", "port", port)
	return server.Run(ctx)
}
