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

	"github.com/jankremlacek/monitor/internal/probe"
)

// Discovery scans for probes and describes them.
type Discovery struct {
	probesDir string
}

// NewDiscovery creates a new probe discovery instance.
func NewDiscovery(probesDir string) *Discovery {
	return &Discovery{
		probesDir: probesDir,
	}
}

// DiscoverAll scans the probes directory and returns descriptions of all found probes.
func (d *Discovery) DiscoverAll(ctx context.Context) ([]RegisterProbeType, error) {
	entries, err := os.ReadDir(d.probesDir)
	if err != nil {
		return nil, fmt.Errorf("read probes directory: %w", err)
	}

	var probeTypes []RegisterProbeType
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

		// Convert absolute path if relative
		absPath, err := filepath.Abs(probePath)
		if err != nil {
			absPath = probePath
		}

		// Convert Arguments to map[string]any
		argsMap := make(map[string]any)
		if desc.Arguments.Required != nil {
			reqMap := make(map[string]any)
			for k, v := range desc.Arguments.Required {
				reqMap[k] = map[string]any{
					"type":        v.Type,
					"description": v.Description,
				}
				if v.Default != nil {
					reqMap[k].(map[string]any)["default"] = v.Default
				}
			}
			argsMap["required"] = reqMap
		}
		if desc.Arguments.Optional != nil {
			optMap := make(map[string]any)
			for k, v := range desc.Arguments.Optional {
				optMap[k] = map[string]any{
					"type":        v.Type,
					"description": v.Description,
				}
				if v.Default != nil {
					optMap[k].(map[string]any)["default"] = v.Default
				}
			}
			argsMap["optional"] = optMap
		}

		version := desc.Version
		if version == "" {
			version = "0.0.0"
		}

		probeTypes = append(probeTypes, RegisterProbeType{
			Name:           desc.Name,
			Version:        version,
			Description:    desc.Description,
			Arguments:      argsMap,
			ExecutablePath: absPath,
		})

		slog.Info("discovered probe", "name", desc.Name, "version", version)
	}

	return probeTypes, nil
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
