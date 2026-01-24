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
		"max-age-hours": 24,
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
