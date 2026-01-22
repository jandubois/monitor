package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jandubois/monitor/internal/probe"
)

func TestParseCommitMessage(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTitle string
		expectedBody  string
	}{
		{
			name:          "title only",
			input:         "Fix bug in parser",
			expectedTitle: "Fix bug in parser",
			expectedBody:  "",
		},
		{
			name:          "title and body",
			input:         "Fix bug in parser\n\nThis fixes the issue where...",
			expectedTitle: "Fix bug in parser",
			expectedBody:  "This fixes the issue where...",
		},
		{
			name:          "title with trailing newline",
			input:         "Fix bug\n",
			expectedTitle: "Fix bug",
			expectedBody:  "",
		},
		{
			name:          "multiline body",
			input:         "Title\n\nLine 1\nLine 2\nLine 3",
			expectedTitle: "Title",
			expectedBody:  "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := parseCommitMessage(tt.input)
			if title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, title)
			}
			if body != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, body)
			}
		})
	}
}

func TestRunEmptyRepo(t *testing.T) {
	result := Run("", "main", "", 24, 0, 0)
	if result.Status != probe.StatusCritical {
		t.Errorf("expected status %q, got %q", probe.StatusCritical, result.Status)
	}
	if result.Message != "repo argument is required" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRunWithMockServer(t *testing.T) {
	// Create mock GitHub API server
	commitTime := time.Now().Add(-1 * time.Hour)
	branchHandler := func(w http.ResponseWriter, r *http.Request) {
		resp := branchResponse{}
		resp.Commit.SHA = "abc123def456789"
		json.NewEncoder(w).Encode(resp)
	}
	commitHandler := func(w http.ResponseWriter, r *http.Request) {
		commit := Commit{
			SHA: "abc123def456789",
		}
		commit.Commit.Message = "Test commit message\n\nWith body"
		commit.Commit.Author.Date = commitTime
		commit.Stats.Additions = 10
		commit.Stats.Deletions = 5
		commit.Stats.Total = 15
		commit.Files = []struct {
			Filename  string `json:"filename"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
			Changes   int    `json:"changes"`
		}{
			{Filename: "file1.go", Additions: 5, Deletions: 2, Changes: 7},
			{Filename: "file2.go", Additions: 5, Deletions: 3, Changes: 8},
		}
		json.NewEncoder(w).Encode(commit)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/branches/main", branchHandler)
	mux.HandleFunc("/repos/owner/repo/commits/abc123def456789", commitHandler)

	server := httptest.NewServer(mux)
	defer server.Close()

	// We can't easily inject the server URL into the probe since it uses hardcoded GitHub URLs
	// So we'll just test the helper functions and validation
	t.Skip("Full integration test requires URL injection - testing helpers instead")
}

func TestGetDescription(t *testing.T) {
	desc := GetDescription()
	if desc.Name != "github" {
		t.Errorf("expected name 'github', got %q", desc.Name)
	}
	if desc.Subcommand != Name {
		t.Errorf("expected subcommand %q, got %q", Name, desc.Subcommand)
	}

	// Check required arguments
	if _, ok := desc.Arguments.Required["repo"]; !ok {
		t.Error("expected 'repo' in required arguments")
	}

	// Check optional arguments
	expectedOptional := []string{"branch", "max_age_hours", "min_files", "min_additions"}
	for _, arg := range expectedOptional {
		if _, ok := desc.Arguments.Optional[arg]; !ok {
			t.Errorf("expected %q in optional arguments", arg)
		}
	}
}

func TestFormatCommitMessage(t *testing.T) {
	commit := &Commit{
		SHA: "abc123def456789",
	}
	commit.Commit.Message = "Test commit\n\nWith body text"
	commit.Stats.Additions = 100
	commit.Stats.Deletions = 50
	commit.Files = make([]struct {
		Filename  string `json:"filename"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
	}, 5)

	result := formatCommitMessage("owner/repo", commit, "https://github.com/owner/repo/commit/abc123def456789")

	// Check that the message contains expected parts
	if len(result) == 0 {
		t.Error("expected non-empty message")
	}
	// Should contain short SHA link
	if !contains(result, "abc123d") {
		t.Error("expected short SHA in message")
	}
	// Should contain stats
	if !contains(result, "+100") {
		t.Error("expected additions in message")
	}
	if !contains(result, "-50") {
		t.Error("expected deletions in message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
