// Package debug provides the debug probe implementation.
package debug

import (
	"time"

	"github.com/jandubois/monitor/internal/probe"
)

// Name is the probe subcommand name.
const Name = "debug"

// GetDescription returns the probe description.
func GetDescription() probe.Description {
	return probe.Description{
		Name:            "debug",
		Description:     "Debug probe for testing failure modes",
		Version:         "1.0.0",
		Subcommand:      Name,
		DefaultName:     "Debug ({{mode}})",
		DefaultInterval: "1m",
		Arguments: probe.Arguments{
			Required: map[string]probe.ArgumentSpec{},
			Optional: map[string]probe.ArgumentSpec{
				"Mode": {
					Type:        "string",
					Description: "Probe behavior mode",
					Default:     "ok",
					Enum:        []string{"ok", "warning", "critical", "timeout", "crash", "error"},
				},
				"Message": {
					Type:        "string",
					Description: "Custom message to return",
				},
				"Delay MS": {
					Type:        "number",
					Description: "Delay before responding (milliseconds)",
					Default:     float64(0),
				},
			},
		},
		Output: probe.OutputSchema{
			Data: map[string]probe.DataSpec{
				"mode": {Type: "string", Description: "Active mode"},
			},
		},
	}
}

// Run executes the probe with the given arguments.
// Note: "timeout", "crash", and "error" modes behave differently when run directly.
func Run(mode, message string, delayMs int) *probe.Result {
	// Apply delay if specified
	if delayMs > 0 {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	switch mode {
	case "ok":
		msg := message
		if msg == "" {
			msg = "Debug probe completed successfully"
		}
		return &probe.Result{
			Status:  probe.StatusOK,
			Summary: "mode=ok",
			Message: msg,
			Data:    map[string]any{"mode": "ok"},
		}

	case "warning":
		msg := message
		if msg == "" {
			msg = "Debug probe simulated warning"
		}
		return &probe.Result{
			Status:  probe.StatusWarning,
			Summary: "mode=warning",
			Message: msg,
			Data:    map[string]any{"mode": "warning"},
		}

	case "critical":
		msg := message
		if msg == "" {
			msg = "Debug probe simulated critical failure"
		}
		return &probe.Result{
			Status:  probe.StatusCritical,
			Summary: "mode=critical",
			Message: msg,
			Data:    map[string]any{"mode": "critical"},
		}

	case "timeout":
		// Sleep forever - caller will need to handle timeout
		select {}

	case "crash":
		panic("debug probe intentional crash")

	case "error":
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Summary: "mode=error",
			Message: "Debug probe simulated error",
			Data:    map[string]any{"mode": "error"},
		}

	default:
		return &probe.Result{
			Status:  probe.StatusUnknown,
			Summary: "invalid mode",
			Message: "Invalid mode: " + mode,
		}
	}
}
