package cmd

import (
	"fmt"
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
	rootCmd.PersistentFlags().StringP("database-url", "d", "", "PostgreSQL connection URL")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
}

func getDatabaseURL(cmd *cobra.Command) string {
	url, _ := cmd.Flags().GetString("database-url")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		fmt.Fprintln(os.Stderr, "error: database URL required (--database-url or DATABASE_URL)")
		os.Exit(1)
	}
	return url
}
