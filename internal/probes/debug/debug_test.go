package debug

import (
	"testing"

	"github.com/jandubois/monitor/internal/probe"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		message        string
		delayMs        int
		expectedStatus probe.Status
		expectedMsg    string
	}{
		{
			name:           "ok mode default message",
			mode:           "ok",
			message:        "",
			delayMs:        0,
			expectedStatus: probe.StatusOK,
			expectedMsg:    "Debug probe completed successfully",
		},
		{
			name:           "ok mode custom message",
			mode:           "ok",
			message:        "custom ok",
			delayMs:        0,
			expectedStatus: probe.StatusOK,
			expectedMsg:    "custom ok",
		},
		{
			name:           "warning mode",
			mode:           "warning",
			message:        "",
			delayMs:        0,
			expectedStatus: probe.StatusWarning,
			expectedMsg:    "Debug probe simulated warning",
		},
		{
			name:           "critical mode",
			mode:           "critical",
			message:        "",
			delayMs:        0,
			expectedStatus: probe.StatusCritical,
			expectedMsg:    "Debug probe simulated critical failure",
		},
		{
			name:           "error mode",
			mode:           "error",
			message:        "",
			delayMs:        0,
			expectedStatus: probe.StatusUnknown,
			expectedMsg:    "Debug probe simulated error",
		},
		{
			name:           "invalid mode",
			mode:           "invalid",
			message:        "",
			delayMs:        0,
			expectedStatus: probe.StatusUnknown,
			expectedMsg:    "Invalid mode: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Run(tt.mode, tt.message, tt.delayMs)
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %q, got %q", tt.expectedStatus, result.Status)
			}
			if result.Message != tt.expectedMsg {
				t.Errorf("expected message %q, got %q", tt.expectedMsg, result.Message)
			}
		})
	}
}

func TestRunCrashMode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for crash mode")
		}
	}()
	Run("crash", "", 0)
}

func TestGetDescription(t *testing.T) {
	desc := GetDescription()
	if desc.Name != "debug" {
		t.Errorf("expected name 'debug', got %q", desc.Name)
	}
	if desc.Subcommand != Name {
		t.Errorf("expected subcommand %q, got %q", Name, desc.Subcommand)
	}
}
