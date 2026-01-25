package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"
)

type Description struct {
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Version         string       `json:"version"`
	Arguments       Arguments    `json:"arguments"`
	Output          OutputSchema `json:"output,omitempty"`
	DefaultInterval string       `json:"default_interval,omitempty"`
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
	mode := flag.String("mode", "ok", "Behavior mode: ok, warning, critical, timeout, crash, error")
	message := flag.String("message", "", "Custom message (optional)")
	delayMs := flag.Int("delay-ms", 0, "Delay before responding in milliseconds")
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
		output("ok", "mode=ok", msg)

	case "warning":
		msg := *message
		if msg == "" {
			msg = "Debug probe simulated warning"
		}
		output("warning", "mode=warning", msg)

	case "critical":
		msg := *message
		if msg == "" {
			msg = "Debug probe simulated critical failure"
		}
		output("critical", "mode=critical", msg)

	case "timeout":
		// Sleep forever - watcher will kill us
		select {}

	case "crash":
		panic("debug probe intentional crash")

	case "error":
		// Exit with non-zero code without outputting valid JSON
		os.Exit(1)

	default:
		output("unknown", "invalid mode", "Invalid mode: "+*mode)
	}
}

func printDescription() {
	desc := Description{
		Name:            "debug",
		Description:     "Debug probe for testing failure modes",
		Version:         "1.0.0",
		DefaultInterval: "1m",
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
		Output: OutputSchema{
			Data: map[string]DataSpec{
				"mode": {Type: "string", Description: "Active mode"},
			},
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(desc)
}

func output(status, summary, message string) {
	result := Result{
		Status:  status,
		Summary: summary,
		Message: message,
		Data: map[string]any{
			"mode": status,
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
