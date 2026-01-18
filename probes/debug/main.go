package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"
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
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Result struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func main() {
	describe := flag.Bool("describe", false, "Print probe description")
	mode := flag.String("mode", "ok", "Behavior mode: ok, warning, critical, timeout, crash, error")
	message := flag.String("message", "", "Custom message (optional)")
	delayMs := flag.Int("delay_ms", 0, "Delay before responding in milliseconds")
	flag.Parse()

	if *describe {
		printDescription()
		return
	}

	// Apply delay if specified
	if *delayMs > 0 {
		time.Sleep(time.Duration(*delayMs) * time.Millisecond)
	}

	switch *mode {
	case "ok":
		msg := *message
		if msg == "" {
			msg = "Debug probe completed successfully"
		}
		output("ok", msg)

	case "warning":
		msg := *message
		if msg == "" {
			msg = "Debug probe simulated warning"
		}
		output("warning", msg)

	case "critical":
		msg := *message
		if msg == "" {
			msg = "Debug probe simulated critical failure"
		}
		output("critical", msg)

	case "timeout":
		// Sleep forever - watcher will kill us
		select {}

	case "crash":
		panic("debug probe intentional crash")

	case "error":
		// Exit with non-zero code without outputting valid JSON
		os.Exit(1)

	default:
		output("unknown", "Invalid mode: "+*mode)
	}
}

func printDescription() {
	desc := Description{
		Name:        "debug",
		Description: "Debug probe for testing failure modes",
		Version:     "1.0.0",
		Arguments: Arguments{
			Required: map[string]ArgSpec{},
			Optional: map[string]ArgSpec{
				"mode": {
					Type:        "string",
					Description: "Probe behavior mode",
					Default:     "ok",
					Enum:        []string{"ok", "warning", "critical", "timeout", "crash", "error"},
				},
				"message": {
					Type:        "string",
					Description: "Custom message to return",
				},
				"delay_ms": {
					Type:        "number",
					Description: "Delay before responding (milliseconds)",
					Default:     0,
				},
			},
		},
	}
	json.NewEncoder(os.Stdout).Encode(desc)
}

func output(status, message string) {
	result := Result{
		Status:  status,
		Message: message,
		Data: map[string]any{
			"mode": status,
		},
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
