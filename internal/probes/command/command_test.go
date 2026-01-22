package command

import (
	"runtime"
	"testing"

	"github.com/jandubois/monitor/internal/probe"
)

func TestParseCodeSet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[int]bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[int]bool{},
		},
		{
			name:     "single code",
			input:    "0",
			expected: map[int]bool{0: true},
		},
		{
			name:     "multiple codes",
			input:    "0,1,2",
			expected: map[int]bool{0: true, 1: true, 2: true},
		},
		{
			name:     "codes with spaces",
			input:    "0, 1, 2",
			expected: map[int]bool{0: true, 1: true, 2: true},
		},
		{
			name:     "non-zero codes",
			input:    "1,2,3",
			expected: map[int]bool{1: true, 2: true, 3: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCodeSet(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d codes, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected code %d to be %v", k, v)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello... (truncated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRunEmptyCommand(t *testing.T) {
	result := Run("", "/bin/sh", "0", "", true)
	if result.Status != probe.StatusUnknown {
		t.Errorf("expected status %q, got %q", probe.StatusUnknown, result.Status)
	}
	if result.Message != "command argument is required" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRunSuccessfulCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	result := Run("echo hello", "/bin/sh", "0", "", true)
	if result.Status != probe.StatusOK {
		t.Errorf("expected status %q, got %q", probe.StatusOK, result.Status)
	}
	if result.Message != "Command completed successfully" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if stdout, ok := result.Data["stdout"].(string); !ok || stdout != "hello\n" {
		t.Errorf("unexpected stdout: %v", result.Data["stdout"])
	}
}

func TestRunFailingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	result := Run("exit 1", "/bin/sh", "0", "", true)
	if result.Status != probe.StatusCritical {
		t.Errorf("expected status %q, got %q", probe.StatusCritical, result.Status)
	}
	if result.Metrics["exit_code"] != 1 {
		t.Errorf("expected exit code 1, got %v", result.Metrics["exit_code"])
	}
}

func TestRunWarningCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	result := Run("exit 2", "/bin/sh", "0", "2", true)
	if result.Status != probe.StatusWarning {
		t.Errorf("expected status %q, got %q", probe.StatusWarning, result.Status)
	}
}

func TestRunCustomOkCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	result := Run("exit 42", "/bin/sh", "0,42", "", true)
	if result.Status != probe.StatusOK {
		t.Errorf("expected status %q, got %q", probe.StatusOK, result.Status)
	}
}

func TestRunCaptureOutputDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	result := Run("echo secret", "/bin/sh", "0", "", false)
	if result.Status != probe.StatusOK {
		t.Errorf("expected status %q, got %q", probe.StatusOK, result.Status)
	}
	if _, ok := result.Data["stdout"]; ok {
		t.Error("stdout should not be captured when captureOutput is false")
	}
}

func TestGetDescription(t *testing.T) {
	desc := GetDescription()
	if desc.Name != "command" {
		t.Errorf("expected name 'command', got %q", desc.Name)
	}
}
