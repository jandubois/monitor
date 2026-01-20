package cmd

import (
	"log/slog"

	"github.com/jandubois/monitor/internal/db"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE:  runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().Bool("down", false, "Roll back all migrations")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	databaseURL := getDatabaseURL(cmd)
	down, _ := cmd.Flags().GetBool("down")

	if down {
		slog.Info("rolling back all migrations")
		if err := db.RollbackMigrations(databaseURL); err != nil {
			return err
		}
		slog.Info("migrations rolled back")
	} else {
		slog.Info("running migrations")
		if err := db.RunMigrations(databaseURL); err != nil {
			return err
		}
		slog.Info("migrations complete")
	}

	return nil
}
