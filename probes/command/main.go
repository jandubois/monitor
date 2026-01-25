package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Description struct {
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Version         string       `json:"version"`
	Arguments       Arguments    `json:"arguments"`
	Output          OutputSchema `json:"output,omitempty"`
	DefaultName     string       `json:"default_name,omitempty"`
	DefaultInterval string       `json:"default_interval,omitempty"`
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

type OutputSchema struct {
	Metrics map[string]MetricSpec `json:"metrics,omitempty"`
	Data    map[string]DataSpec   `json:"data,omitempty"`
}

type MetricSpec struct {
	Type        string `json:"type"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

type DataSpec struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type Result struct {
	Status  string         `json:"status"`
	Summary string         `json:"summary,omitempty"`
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func main() {
	describe := flag.Bool("describe", false, "Print probe description")
	command := flag.String("command", "", "Command to run")
	shell := flag.String("shell", "/bin/sh", "Shell to use")
	okCodes := flag.String("ok-codes", "0", "Comma-separated exit codes that indicate success")
	warningCodes := flag.String("warning-codes", "", "Comma-separated exit codes that indicate warning")
	captureOutput := flag.Bool("capture-output", true, "Capture command output in result")
	flag.Parse()

	if *describe {
		printDescription()
		return
	}

	if *command == "" {
		outputError("command argument is required")
		return
	}

	runCommand(*command, *shell, *okCodes, *warningCodes, *captureOutput)
}

func printDescription() {
	desc := Description{
		Name:            "command",
		Description:     "Run a command and check its exit code",
		Version:         "1.0.0",
		DefaultName:     "Command on {{Watcher}}",
		DefaultInterval: "5m",
		Arguments: Arguments{
			Required: map[string]ArgSpec{
				"command": {
					Type:        "string",
					Description: "Command to run",
				},
			},
			Optional: map[string]ArgSpec{
				"shell": {
					Type:        "string",
					Description: "Shell to use for execution",
					Default:     "/bin/sh",
				},
				"ok_codes": {
					Type:        "string",
					Description: "Comma-separated exit codes that indicate success",
					Default:     "0",
				},
				"warning_codes": {
					Type:        "string",
					Description: "Comma-separated exit codes that indicate warning",
					Default:     "",
				},
				"capture_output": {
					Type:        "boolean",
					Description: "Include command output in result data",
					Default:     true,
				},
			},
		},
		Output: OutputSchema{
			Metrics: map[string]MetricSpec{
				"exit_code":   {Type: "integer", Description: "Command exit code"},
				"duration_ms": {Type: "integer", Unit: "milliseconds", Description: "Execution time"},
			},
			Data: map[string]DataSpec{
				"command": {Type: "string", Description: "Executed command"},
				"stdout":  {Type: "string", Description: "Standard output"},
				"stderr":  {Type: "string", Description: "Standard error"},
			},
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(desc)
}

func runCommand(command, shell, okCodes, warningCodes string, captureOutput bool) {
	okCodeSet := parseCodeSet(okCodes)
	warningCodeSet := parseCodeSet(warningCodes)

	start := time.Now()
	cmd := exec.Command(shell, "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		} else {
			outputError(fmt.Sprintf("failed to run command: %v", err))
			return
		}
	}

	status := "critical"
	if okCodeSet[exitCode] {
		status = "ok"
	} else if warningCodeSet[exitCode] {
		status = "warning"
	}

	summary := fmt.Sprintf("Exit code %d (%dms)", exitCode, duration.Milliseconds())
	message := fmt.Sprintf("Command exited with code %d", exitCode)
	if status == "ok" {
		summary = fmt.Sprintf("OK (%dms)", duration.Milliseconds())
		message = "Command completed successfully"
	}

	result := Result{
		Status:  status,
		Summary: summary,
		Message: message,
		Metrics: map[string]any{
			"exit_code":   exitCode,
			"duration_ms": duration.Milliseconds(),
		},
		Data: map[string]any{
			"command": command,
		},
	}

	if captureOutput {
		result.Data["stdout"] = truncate(stdout.String(), 10000)
		result.Data["stderr"] = truncate(stderr.String(), 10000)
	}

	_ = json.NewEncoder(os.Stdout).Encode(result)
}

func parseCodeSet(codes string) map[int]bool {
	set := make(map[int]bool)
	if codes == "" {
		return set
	}
	for _, part := range strings.Split(codes, ",") {
		var code int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &code); err == nil {
			set[code] = true
		}
	}
	return set
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

func outputError(msg string) {
	result := Result{
		Status:  "unknown",
		Message: msg,
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
