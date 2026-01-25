package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending migrations.
func RunMigrations(dbPath string) error {
	return runMigrate(dbPath, false)
}

// RollbackMigrations rolls back all migrations.
func RollbackMigrations(dbPath string) error {
	return runMigrate(dbPath, true)
}

func runMigrate(dbPath string, down bool) error {
	// Create parent directory if needed
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Create migrations table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			dirty INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	var dirty int
	err = db.QueryRow(`SELECT COALESCE(MAX(version), 0), COALESCE(MAX(dirty), 0) FROM schema_migrations`).Scan(&currentVersion, &dirty)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if dirty != 0 {
		return fmt.Errorf("database is in dirty state at version %d, manual intervention required", currentVersion)
	}

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	type migration struct {
		version int
		name    string
		up      string
		down    string
	}

	migrations := make(map[int]*migration)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		var version int
		var suffix string
		_, err := fmt.Sscanf(name, "%d_%s", &version, &suffix)
		if err != nil {
			continue
		}

		if migrations[version] == nil {
			migrations[version] = &migration{version: version}
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if strings.HasSuffix(name, ".up.sql") {
			migrations[version].up = string(content)
			migrations[version].name = strings.TrimSuffix(name, ".up.sql")
		} else if strings.HasSuffix(name, ".down.sql") {
			migrations[version].down = string(content)
		}
	}

	// Sort versions
	var versions []int
	for v := range migrations {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	if down {
		// Roll back all migrations in reverse order
		sort.Sort(sort.Reverse(sort.IntSlice(versions)))
		for _, v := range versions {
			if v > currentVersion {
				continue
			}
			m := migrations[v]
			if m.down == "" {
				return fmt.Errorf("no down migration for version %d", v)
			}

			// Mark as dirty
			_, err = db.Exec(`INSERT OR REPLACE INTO schema_migrations (version, dirty) VALUES (?, 1)`, v)
			if err != nil {
				return fmt.Errorf("mark version %d as dirty: %w", v, err)
			}

			// Run down migration
			_, err = db.Exec(m.down)
			if err != nil {
				return fmt.Errorf("run down migration %d: %w", v, err)
			}

			// Remove version record
			_, err = db.Exec(`DELETE FROM schema_migrations WHERE version = ?`, v)
			if err != nil {
				return fmt.Errorf("remove version %d: %w", v, err)
			}
		}
	} else {
		// Apply pending migrations
		for _, v := range versions {
			if v <= currentVersion {
				continue
			}
			m := migrations[v]
			if m.up == "" {
				return fmt.Errorf("no up migration for version %d", v)
			}

			// Mark as dirty
			_, err = db.Exec(`INSERT OR REPLACE INTO schema_migrations (version, dirty) VALUES (?, 1)`, v)
			if err != nil {
				return fmt.Errorf("mark version %d as dirty: %w", v, err)
			}

			// Run up migration
			_, err = db.Exec(m.up)
			if err != nil {
				return fmt.Errorf("run up migration %d: %w", v, err)
			}

			// Mark as clean
			_, err = db.Exec(`UPDATE schema_migrations SET dirty = 0 WHERE version = ?`, v)
			if err != nil {
				return fmt.Errorf("mark version %d as clean: %w", v, err)
			}
		}
	}

	return nil
}
