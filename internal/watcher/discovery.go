package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jankremlacek/monitor/internal/db"
	"github.com/jankremlacek/monitor/internal/probe"
)

// Discovery scans for probes and registers them in the database.
type Discovery struct {
	db        *db.DB
	probesDir string
}

// NewDiscovery creates a new probe discovery instance.
func NewDiscovery(database *db.DB, probesDir string) *Discovery {
	return &Discovery{
		db:        database,
		probesDir: probesDir,
	}
}

// DiscoverAll scans the probes directory and registers all found probes.
func (d *Discovery) DiscoverAll(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(d.probesDir)
	if err != nil {
		return 0, fmt.Errorf("read probes directory: %w", err)
	}

	registered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		probePath := filepath.Join(d.probesDir, entry.Name(), entry.Name())
		if _, err := os.Stat(probePath); os.IsNotExist(err) {
			// Try without subdirectory name duplication
			probePath = filepath.Join(d.probesDir, entry.Name())
			if info, err := os.Stat(probePath); err != nil || info.IsDir() {
				continue
			}
		}

		desc, err := d.describeProbe(ctx, probePath)
		if err != nil {
			slog.Warn("failed to describe probe", "path", probePath, "error", err)
			continue
		}

		if err := d.registerProbe(ctx, desc, probePath); err != nil {
			slog.Error("failed to register probe", "name", desc.Name, "error", err)
			continue
		}

		slog.Info("registered probe", "name", desc.Name, "version", desc.Version)
		registered++
	}

	return registered, nil
}

func (d *Discovery) describeProbe(ctx context.Context, path string) (*probe.Description, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use absolute path to avoid any path resolution issues
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	cmd := exec.CommandContext(ctx, absPath, "--describe")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run --describe: %w (stderr: %s)", err, stderr.String())
	}

	var desc probe.Description
	if err := json.Unmarshal(stdout.Bytes(), &desc); err != nil {
		return nil, fmt.Errorf("parse description: %w", err)
	}

	return &desc, nil
}

func (d *Discovery) registerProbe(ctx context.Context, desc *probe.Description, execPath string) error {
	// Convert absolute path if relative
	absPath, err := filepath.Abs(execPath)
	if err != nil {
		absPath = execPath
	}

	argsJSON, err := json.Marshal(desc.Arguments)
	if err != nil {
		return fmt.Errorf("marshal arguments: %w", err)
	}

	_, err = d.db.Pool().Exec(ctx, `
		INSERT INTO probe_types (name, description, version, arguments, executable_path, registered_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE SET
			description = EXCLUDED.description,
			version = EXCLUDED.version,
			arguments = EXCLUDED.arguments,
			executable_path = EXCLUDED.executable_path,
			updated_at = NOW()
	`, desc.Name, desc.Description, desc.Version, argsJSON, absPath)

	return err
}
