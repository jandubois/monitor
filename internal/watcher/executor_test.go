package watcher

import (
	"strings"
	"testing"
)

func TestToEnvName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"command", "COMMAND"},
		{"max-age-hours", "MAX_AGE_HOURS"},
		{"ok_codes", "OK_CODES"},
		{"CamelCase", "CAMELCASE"},
		{"with spaces", "WITHSPACES"},
		{"special!@#chars", "SPECIALCHARS"},
		{"123numeric", "123NUMERIC"},
		{"mixed-case_and-stuff", "MIXED_CASE_AND_STUFF"},
		{"", ""},
		{"---", "___"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toEnvName(tt.input)
			if result != tt.expected {
				t.Errorf("toEnvName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildEnv(t *testing.T) {
	args := map[string]any{
		"command":       "ls -la",
		"max_age_hours": 24,
		"enabled":       true,
	}

	env := buildEnv(args)

	// Check that the new variables are present
	found := make(map[string]bool)
	for _, e := range env {
		if strings.HasPrefix(e, "PROBE_") {
			found[e] = true
		}
	}

	expected := []string{
		"PROBE_COMMAND=ls -la",
		"PROBE_MAX_AGE_HOURS=24",
		"PROBE_ENABLED=true",
	}

	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected %q in environment, got: %v", exp, found)
		}
	}

	// Verify existing environment is preserved (PATH should exist)
	hasPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("expected PATH to be preserved in environment")
	}
}

func TestBuildEnvEmptyName(t *testing.T) {
	// A parameter that becomes empty after sanitization should be skipped
	args := map[string]any{
		"!!!": "value",
	}

	env := buildEnv(args)

	for _, e := range env {
		if strings.HasPrefix(e, "PROBE_=") {
			t.Error("should not create PROBE_= for empty sanitized name")
		}
	}
}

func TestBuildArgs(t *testing.T) {
	// buildArgs should convert underscores to dashes for CLI flags
	args := map[string]any{
		"min_free_gb": 10,
		"path":        "/volume1",
	}

	cliArgs := buildArgs(args)

	// Check that underscores were converted to dashes
	found := make(map[string]bool)
	for _, arg := range cliArgs {
		found[arg] = true
	}

	// Note: map iteration order is not guaranteed, so we check both args exist
	if !found["--min-free-gb=10"] {
		t.Errorf("expected --min-free-gb=10 in args, got: %v", cliArgs)
	}
	if !found["--path=/volume1"] {
		t.Errorf("expected --path=/volume1 in args, got: %v", cliArgs)
	}
}
