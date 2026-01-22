package diskspace

import (
	"strings"
	"testing"

	"github.com/jandubois/monitor/internal/probe"
)

func TestRunEmptyPath(t *testing.T) {
	result := Run("", 10, 0)
	if result.Status != probe.StatusUnknown {
		t.Errorf("expected status %q, got %q", probe.StatusUnknown, result.Status)
	}
	if result.Message != "path argument is required" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRunInvalidPath(t *testing.T) {
	result := Run("/nonexistent/path/that/does/not/exist", 10, 0)
	if result.Status != probe.StatusUnknown {
		t.Errorf("expected status %q, got %q", probe.StatusUnknown, result.Status)
	}
	if !strings.Contains(result.Message, "failed to stat") {
		t.Errorf("expected 'failed to stat' in message, got: %s", result.Message)
	}
}

func TestRunRootPath(t *testing.T) {
	result := Run("/", 0, 0) // No thresholds, should always be OK
	if result.Status != probe.StatusOK {
		t.Errorf("expected status %q, got %q", probe.StatusOK, result.Status)
	}

	// Check metrics exist
	if result.Metrics == nil {
		t.Fatal("expected metrics to be set")
	}
	if _, ok := result.Metrics["free_bytes"]; !ok {
		t.Error("expected 'free_bytes' in metrics")
	}
	if _, ok := result.Metrics["total_bytes"]; !ok {
		t.Error("expected 'total_bytes' in metrics")
	}
	if _, ok := result.Metrics["free_gb"]; !ok {
		t.Error("expected 'free_gb' in metrics")
	}
	if _, ok := result.Metrics["free_percent"]; !ok {
		t.Error("expected 'free_percent' in metrics")
	}

	// Check data
	if result.Data == nil {
		t.Fatal("expected data to be set")
	}
	if result.Data["path"] != "/" {
		t.Errorf("expected path '/', got %v", result.Data["path"])
	}
}

func TestRunWithMinFreeGB(t *testing.T) {
	// Test with impossibly high threshold - should fail
	result := Run("/", 999999999, 0) // 999 million GB
	if result.Status != probe.StatusCritical {
		t.Errorf("expected status %q with high min_free_gb, got %q", probe.StatusCritical, result.Status)
	}
	if !strings.Contains(result.Message, "minimum") {
		t.Errorf("expected 'minimum' in message, got: %s", result.Message)
	}
}

func TestRunWithMinFreePercent(t *testing.T) {
	// Test with impossibly high threshold - should fail
	result := Run("/", 0, 100.1) // More than 100%
	if result.Status != probe.StatusCritical {
		t.Errorf("expected status %q with high min_free_percent, got %q", probe.StatusCritical, result.Status)
	}
	if !strings.Contains(result.Message, "minimum") {
		t.Errorf("expected 'minimum' in message, got: %s", result.Message)
	}
}

func TestRunMessageFormat(t *testing.T) {
	result := Run("/", 0, 0)
	if result.Status != probe.StatusOK {
		t.Fatalf("expected OK status, got %q", result.Status)
	}

	// Message should contain human-readable size and percentage
	if !strings.Contains(result.Message, "free on /") {
		t.Errorf("expected 'free on /' in message, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "%") {
		t.Errorf("expected percentage in message, got: %s", result.Message)
	}
}

func TestGetDescription(t *testing.T) {
	desc := GetDescription()
	if desc.Name != "disk-space" {
		t.Errorf("expected name 'disk-space', got %q", desc.Name)
	}
	if desc.Subcommand != Name {
		t.Errorf("expected subcommand %q, got %q", Name, desc.Subcommand)
	}

	// Check required arguments
	if _, ok := desc.Arguments.Required["path"]; !ok {
		t.Error("expected 'path' in required arguments")
	}

	// Check optional arguments
	if _, ok := desc.Arguments.Optional["min_free_gb"]; !ok {
		t.Error("expected 'min_free_gb' in optional arguments")
	}
	if _, ok := desc.Arguments.Optional["min_free_percent"]; !ok {
		t.Error("expected 'min_free_percent' in optional arguments")
	}
}
