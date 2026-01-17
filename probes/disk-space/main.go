package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"syscall"
)

type Description struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Arguments   Arguments `json:"arguments"`
}

type Arguments struct {
	Required map[string]ArgSpec `json:"required"`
	Optional map[string]ArgSpec `json:"optional"`
}

type ArgSpec struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

type Result struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func main() {
	describe := flag.Bool("describe", false, "Print probe description")
	path := flag.String("path", "", "Path to check")
	minFreeGB := flag.Float64("min_free_gb", 10, "Minimum free gigabytes")
	minFreePercent := flag.Float64("min_free_percent", 0, "Minimum free percentage")
	flag.Parse()

	if *describe {
		printDescription()
		return
	}

	if *path == "" {
		outputError("path argument is required")
		return
	}

	checkDiskSpace(*path, *minFreeGB, *minFreePercent)
}

func printDescription() {
	desc := Description{
		Name:        "disk-space",
		Description: "Check available disk space on a path",
		Version:     "1.0.0",
		Arguments: Arguments{
			Required: map[string]ArgSpec{
				"path": {
					Type:        "string",
					Description: "Path to check",
				},
			},
			Optional: map[string]ArgSpec{
				"min_free_gb": {
					Type:        "number",
					Description: "Minimum free gigabytes",
					Default:     10,
				},
				"min_free_percent": {
					Type:        "number",
					Description: "Minimum free percentage (0-100)",
					Default:     0,
				},
			},
		},
	}
	json.NewEncoder(os.Stdout).Encode(desc)
}

func checkDiskSpace(path string, minFreeGB, minFreePercent float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		outputError(fmt.Sprintf("failed to stat %s: %v", path, err))
		return
	}

	freeBytes := stat.Bavail * uint64(stat.Bsize)
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	freePercent := float64(freeBytes) / float64(totalBytes) * 100

	status := "ok"
	var reasons []string

	if minFreeGB > 0 && freeGB < minFreeGB {
		status = "critical"
		reasons = append(reasons, fmt.Sprintf("%.1f GB free < %.1f GB minimum", freeGB, minFreeGB))
	}

	if minFreePercent > 0 && freePercent < minFreePercent {
		if status != "critical" {
			status = "critical"
		}
		reasons = append(reasons, fmt.Sprintf("%.1f%% free < %.1f%% minimum", freePercent, minFreePercent))
	}

	message := fmt.Sprintf("%.1f GB free on %s (%.1f%%)", freeGB, path, freePercent)
	if len(reasons) > 0 {
		message = reasons[0]
		if len(reasons) > 1 {
			message += "; " + reasons[1]
		}
	}

	result := Result{
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

	json.NewEncoder(os.Stdout).Encode(result)
}

func outputError(msg string) {
	result := Result{
		Status:  "unknown",
		Message: msg,
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
