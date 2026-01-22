package gitstatus

import (
	"testing"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		hours    float64
		expected string
	}{
		{"1 hour", 1, "1h"},
		{"12 hours", 12, "12h"},
		{"23 hours", 23, "23h"},
		{"24 hours", 24, "1 days"},
		{"48 hours", 48, "2 days"},
		{"1 week", 168, "1 weeks"},
		{"2 weeks", 336, "2 weeks"},
		{"5 weeks", 840, "1 months"},
		{"3 months", 2160, "3 months"},
		{"1 year", 8760, "1 years"},
		{"1.5 years", 13140, "1 years"}, // years branch doesn't show months for ~1.5 years
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.hours)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.hours, result, tt.expected)
			}
		})
	}
}

func TestIsAIFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		// Should match
		{"CLAUDE.md", true},
		{"AGENTS.md", true},
		{"GEMINI.md", true},
		{"COPILOT.md", true},
		{".claude/settings.json", true},
		{".cursor/config", true},
		{"foo/.claude/bar", true},
		{"path/to/CLAUDE.md", true},
		{".github/copilot", true}, // exact match

		// Should not match
		{"README.md", false},
		{"main.go", false},
		{"claude.md", false}, // case sensitive
		{".claudeignore", false},
		{"CLAUDIUS.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isAIFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isAIFile(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestRunEmptyPath(t *testing.T) {
	result := Run("", 1, 4, false)
	if result.Status != "critical" {
		t.Errorf("expected status critical, got %s", result.Status)
	}
	if result.Message != "path argument is required" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRunNonexistentPath(t *testing.T) {
	result := Run("/nonexistent/path/that/does/not/exist", 1, 4, false)
	if result.Status != "ok" {
		t.Errorf("expected status ok (no repos found), got %s", result.Status)
	}
}

func TestGetDescription(t *testing.T) {
	desc := GetDescription()
	if desc.Name != "git-status" {
		t.Errorf("expected name 'git-status', got %q", desc.Name)
	}
	if desc.Subcommand != Name {
		t.Errorf("expected subcommand %q, got %q", Name, desc.Subcommand)
	}

	// Check required arguments
	if _, ok := desc.Arguments.Required["path"]; !ok {
		t.Error("expected 'path' in required arguments")
	}

	// Check optional arguments
	if _, ok := desc.Arguments.Optional["uncommitted_hours"]; !ok {
		t.Error("expected 'uncommitted_hours' in optional arguments")
	}
	if _, ok := desc.Arguments.Optional["unpushed_hours"]; !ok {
		t.Error("expected 'unpushed_hours' in optional arguments")
	}
	if _, ok := desc.Arguments.Optional["exclude_ai_files"]; !ok {
		t.Error("expected 'exclude_ai_files' in optional arguments")
	}
}
