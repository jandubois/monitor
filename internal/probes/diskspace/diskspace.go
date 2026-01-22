// Package diskspace provides the disk-space probe implementation.
package diskspace

import (
	"fmt"
	"syscall"

	units "github.com/docker/go-units"
	"github.com/jandubois/monitor/internal/probe"
)

// Name is the probe subcommand name.
const Name = "disk-space"

// GetDescription returns the probe description.
func GetDescription() probe.Description {
	return probe.Description{
		Name:        "disk-space",
		Description: "Check available disk space on a path",
		Version:     "1.0.0",
		Subcommand:  Name,
		Arguments: probe.Arguments{
			Required: map[string]probe.ArgumentSpec{
				"path": {
					Type:        "string",
					Description: "Path to check",
				},
			},
			Optional: map[string]probe.ArgumentSpec{
				"min_free_gb": {
					Type:        "number",
					Description: "Minimum free gigabytes",
					Default:     float64(10),
				},
				"min_free_percent": {
					Type:        "number",
					Description: "Minimum free percentage (0-100)",
					Default:     float64(0),
				},
			},
		},
	}
}

// Run executes the probe with the given arguments.
func Run(path string, minFreeGB, minFreePercent float64) *probe.Result {
	if path == "" {
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Message: "path argument is required",
		}
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Message: fmt.Sprintf("failed to stat %s: %v", path, err),
		}
	}

	freeBytes := stat.Bavail * uint64(stat.Bsize)
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	freePercent := float64(freeBytes) / float64(totalBytes) * 100

	status := probe.StatusOK
	var reasons []string

	if minFreeGB > 0 && freeGB < minFreeGB {
		status = probe.StatusCritical
		reasons = append(reasons, fmt.Sprintf("%s free < %.0f GB minimum", units.HumanSize(float64(freeBytes)), minFreeGB))
	}

	if minFreePercent > 0 && freePercent < minFreePercent {
		status = probe.StatusCritical
		reasons = append(reasons, fmt.Sprintf("%.1f%% free < %.1f%% minimum", freePercent, minFreePercent))
	}

	message := fmt.Sprintf("%s free on %s (%.1f%%)", units.HumanSize(float64(freeBytes)), path, freePercent)
	if len(reasons) > 0 {
		message = reasons[0]
		if len(reasons) > 1 {
			message += "; " + reasons[1]
		}
	}

	return &probe.Result{
		Status:  status,
		Message: message,
		Metrics: map[string]any{
			"free_bytes":   freeBytes,
			"total_bytes":  totalBytes,
			"free_gb":      freeGB,
			"free_percent": freePercent,
		},
		Data: map[string]any{
			"path": path,
		},
	}
}
