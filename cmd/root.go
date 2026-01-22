package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/jandubois/monitor/cmd.Version=..."
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Personal infrastructure monitoring system",
	Long:  `Monitor tracks diverse digital systems with flexible, self-describing probes.`,
}

const probeGroupID = "probes"

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddGroup(&cobra.Group{ID: probeGroupID, Title: "Built-in Probes:"})
	rootCmd.PersistentFlags().StringP("database", "d", "", "SQLite database path")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
}

func getDatabasePath(cmd *cobra.Command) string {
	path, _ := cmd.Flags().GetString("database")
	if path == "" {
		path = os.Getenv("DATABASE_PATH")
	}
	if path == "" {
		// Default to a reasonable location
		path = "/var/lib/monitor/monitor.db"
	}
	return path
}
