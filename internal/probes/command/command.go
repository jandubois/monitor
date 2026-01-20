// Package command provides the command probe implementation.
package command

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/jankremlacek/monitor/internal/probe"
)

// Name is the probe subcommand name.
const Name = "command"

// GetDescription returns the probe description.
func GetDescription() probe.Description {
	return probe.Description{
		Name:        "command",
		Description: "Run a command and check its exit code",
		Version:     "1.0.0",
		Subcommand:  Name,
		Arguments: probe.Arguments{
			Required: map[string]probe.ArgumentSpec{
				"command": {
					Type:        "string",
					Description: "Command to run",
				},
			},
			Optional: map[string]probe.ArgumentSpec{
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
	}
}

// Run executes the probe with the given arguments.
func Run(command, shell, okCodes, warningCodes string, captureOutput bool) *probe.Result {
	if command == "" {
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Message: "command argument is required",
		}
	}

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
			return &probe.Result{
				Status:  probe.StatusUnknown,
				Message: fmt.Sprintf("failed to run command: %v", err),
			}
		}
	}

	status := probe.StatusCritical
	if okCodeSet[exitCode] {
		status = probe.StatusOK
	} else if warningCodeSet[exitCode] {
		status = probe.StatusWarning
	}

	message := fmt.Sprintf("Command exited with code %d", exitCode)
	if status == probe.StatusOK {
		message = "Command completed successfully"
	}

	result := &probe.Result{
		Status:  status,
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

	return result
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
