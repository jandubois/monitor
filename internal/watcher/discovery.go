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

	"github.com/jandubois/monitor/internal/probe"
	"github.com/jandubois/monitor/internal/probes"
)

// Discovery scans for probes and describes them.
type Discovery struct {
	probesDir   string
	extraProbes []string
}

// NewDiscovery creates a new probe discovery instance.
func NewDiscovery(probesDir string, extraProbes []string) *Discovery {
	return &Discovery{
		probesDir:   probesDir,
		extraProbes: extraProbes,
	}
}

// DiscoverAll scans the probes directory and returns descriptions of all found probes,
// including built-in probes from this binary.
func (d *Discovery) DiscoverAll(ctx context.Context) ([]RegisterProbeType, error) {
	var probeTypes []RegisterProbeType

	// First, discover built-in probes
	builtInTypes, err := d.discoverBuiltIn()
	if err != nil {
		slog.Warn("failed to discover built-in probes", "error", err)
	} else {
		probeTypes = append(probeTypes, builtInTypes...)
	}

	// Then discover external probes from the probes directory
	entries, err := os.ReadDir(d.probesDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read probes directory", "path", d.probesDir, "error", err)
		}
		entries = nil
	}
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

		descs, err := d.describeProbe(ctx, probePath)
		if err != nil {
			slog.Warn("failed to describe probe", "path", probePath, "error", err)
			continue
		}

		// Convert absolute path if relative
		absPath, err := filepath.Abs(probePath)
		if err != nil {
			absPath = probePath
		}

		for _, desc := range descs {
			argsMap := descriptionArgsToMap(desc.Arguments)
			outputMap := outputSchemaToMap(desc.Output)
			version := desc.Version
			if version == "" {
				version = "0.0.0"
			}

			probeTypes = append(probeTypes, RegisterProbeType{
				Name:            desc.Name,
				Version:         version,
				Description:     desc.Description,
				Arguments:       argsMap,
				Output:          outputMap,
				DefaultName:     desc.DefaultName,
				DefaultInterval: desc.DefaultInterval,
				ExecutablePath:  absPath,
				Subcommand:      desc.Subcommand,
			})

			slog.Info("discovered probe", "name", desc.Name, "version", version, "subcommand", desc.Subcommand)
		}
	}

	// Discover extra probes from explicit paths
	for _, probePath := range d.extraProbes {
		absPath, err := filepath.Abs(probePath)
		if err != nil {
			slog.Warn("failed to resolve extra probe path", "path", probePath, "error", err)
			continue
		}

		descs, err := d.describeProbe(ctx, absPath)
		if err != nil {
			slog.Warn("failed to describe extra probe", "path", absPath, "error", err)
			continue
		}

		for _, desc := range descs {
			argsMap := descriptionArgsToMap(desc.Arguments)
			outputMap := outputSchemaToMap(desc.Output)
			version := desc.Version
			if version == "" {
				version = "0.0.0"
			}

			probeTypes = append(probeTypes, RegisterProbeType{
				Name:            desc.Name,
				Version:         version,
				Description:     desc.Description,
				Arguments:       argsMap,
				Output:          outputMap,
				DefaultName:     desc.DefaultName,
				DefaultInterval: desc.DefaultInterval,
				ExecutablePath:  absPath,
				Subcommand:      desc.Subcommand,
			})

			slog.Info("discovered extra probe", "name", desc.Name, "version", version, "path", absPath)
		}
	}

	return probeTypes, nil
}

// discoverBuiltIn returns descriptions of built-in probes using this binary's path.
func (d *Discovery) discoverBuiltIn() ([]RegisterProbeType, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get executable path: %w", err)
	}

	absPath, err := filepath.Abs(exe)
	if err != nil {
		absPath = exe
	}

	var probeTypes []RegisterProbeType
	for _, desc := range probes.GetAllDescriptions() {
		argsMap := descriptionArgsToMap(desc.Arguments)
		outputMap := outputSchemaToMap(desc.Output)
		version := desc.Version
		if version == "" {
			version = "0.0.0"
		}

		probeTypes = append(probeTypes, RegisterProbeType{
			Name:            desc.Name,
			Version:         version,
			Description:     desc.Description,
			Arguments:       argsMap,
			Output:          outputMap,
			DefaultName:     desc.DefaultName,
			DefaultInterval: desc.DefaultInterval,
			ExecutablePath:  absPath,
			Subcommand:      desc.Subcommand,
		})

		slog.Info("discovered built-in probe", "name", desc.Name, "version", version, "subcommand", desc.Subcommand)
	}

	return probeTypes, nil
}

// descriptionArgsToMap converts probe.Arguments to map[string]any format.
func descriptionArgsToMap(args probe.Arguments) map[string]any {
	argsMap := make(map[string]any)
	if args.Required != nil {
		reqMap := make(map[string]any)
		for k, v := range args.Required {
			spec := map[string]any{
				"type":        v.Type,
				"description": v.Description,
			}
			if v.Default != nil {
				spec["default"] = v.Default
			}
			if len(v.Enum) > 0 {
				spec["enum"] = v.Enum
			}
			reqMap[k] = spec
		}
		argsMap["required"] = reqMap
	}
	if args.Optional != nil {
		optMap := make(map[string]any)
		for k, v := range args.Optional {
			spec := map[string]any{
				"type":        v.Type,
				"description": v.Description,
			}
			if v.Default != nil {
				spec["default"] = v.Default
			}
			if len(v.Enum) > 0 {
				spec["enum"] = v.Enum
			}
			optMap[k] = spec
		}
		argsMap["optional"] = optMap
	}
	return argsMap
}

// outputSchemaToMap converts probe.OutputSchema to map[string]any format.
func outputSchemaToMap(output probe.OutputSchema) map[string]any {
	if len(output.Metrics) == 0 && len(output.Data) == 0 {
		return nil
	}

	outputMap := make(map[string]any)
	if len(output.Metrics) > 0 {
		metricsMap := make(map[string]any)
		for k, v := range output.Metrics {
			spec := map[string]any{
				"type": v.Type,
			}
			if v.Unit != "" {
				spec["unit"] = v.Unit
			}
			if v.Description != "" {
				spec["description"] = v.Description
			}
			metricsMap[k] = spec
		}
		outputMap["metrics"] = metricsMap
	}
	if len(output.Data) > 0 {
		dataMap := make(map[string]any)
		for k, v := range output.Data {
			spec := map[string]any{
				"type": v.Type,
			}
			if v.Description != "" {
				spec["description"] = v.Description
			}
			dataMap[k] = spec
		}
		outputMap["data"] = dataMap
	}
	return outputMap
}

func (d *Discovery) describeProbe(ctx context.Context, path string) ([]probe.Description, error) {
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

	// Try parsing as array first (new format)
	var descs []probe.Description
	if err := json.Unmarshal(stdout.Bytes(), &descs); err == nil {
		return descs, nil
	}

	// Fall back to single object (backward compatibility)
	var desc probe.Description
	if err := json.Unmarshal(stdout.Bytes(), &desc); err != nil {
		return nil, fmt.Errorf("parse description: %w", err)
	}

	return []probe.Description{desc}, nil
}
