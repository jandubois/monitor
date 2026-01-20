package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
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

// GitHub API response types
type Commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Stats struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Total     int `json:"total"`
	} `json:"stats"`
	Files []struct {
		Filename  string `json:"filename"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
	} `json:"files"`
}

type BranchResponse struct {
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

func main() {
	describe := flag.Bool("describe", false, "Print probe description")
	repo := flag.String("repo", "", "Repository (owner/name)")
	branch := flag.String("branch", "main", "Branch name")
	maxAgeHours := flag.Int("max_age_hours", 24, "Maximum commit age in hours")
	minFiles := flag.Int("min_files", 0, "Minimum changed files")
	minAdditions := flag.Int("min_additions", 0, "Minimum added lines")
	flag.Parse()

	if *describe {
		printDescription()
		return
	}

	if *repo == "" {
		output("critical", "repo argument is required")
		return
	}

	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	commit, err := getLastCommit(*repo, *branch, token)
	if err != nil {
		output("critical", fmt.Sprintf("Failed to get commit: %v", err))
		return
	}

	// Check conditions
	var failures []string
	commitAge := time.Since(commit.Commit.Author.Date)
	maxAge := time.Duration(*maxAgeHours) * time.Hour

	if commitAge > maxAge {
		failures = append(failures, fmt.Sprintf("commit is %.1f hours old (max %d)", commitAge.Hours(), *maxAgeHours))
	}

	filesChanged := len(commit.Files)
	if *minFiles > 0 && filesChanged < *minFiles {
		failures = append(failures, fmt.Sprintf("only %d files changed (min %d)", filesChanged, *minFiles))
	}

	if *minAdditions > 0 && commit.Stats.Additions < *minAdditions {
		failures = append(failures, fmt.Sprintf("only %d lines added (min %d)", commit.Stats.Additions, *minAdditions))
	}

	// Build result
	metrics := map[string]any{
		"age_hours":     commitAge.Hours(),
		"files_changed": filesChanged,
		"additions":     commit.Stats.Additions,
		"deletions":     commit.Stats.Deletions,
	}

	// Parse commit message into title and body
	commitTitle, commitBody := parseCommitMessage(commit.Commit.Message)

	// Build commit URL
	commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", *repo, commit.SHA)

	data := map[string]any{
		"sha":           commit.SHA[:7],
		"full_sha":      commit.SHA,
		"title":         commitTitle,
		"body":          commitBody,
		"url":           commitURL,
		"author_date":   commit.Commit.Author.Date.Format(time.RFC3339),
		"files_changed": filesChanged,
		"additions":     commit.Stats.Additions,
		"deletions":     commit.Stats.Deletions,
	}

	// Build Markdown message
	message := formatCommitMessage(*repo, commit, commitURL)

	if len(failures) > 0 {
		result := Result{
			Status:  "critical",
			Message: fmt.Sprintf("**Commit check failed:** %s\n\n%s", failures[0], message),
			Metrics: metrics,
			Data:    data,
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	result := Result{
		Status:  "ok",
		Message: message,
		Metrics: metrics,
		Data:    data,
	}
	json.NewEncoder(os.Stdout).Encode(result)
}

func getLastCommit(repo, branch, token string) (*Commit, error) {
	// First get the branch to find the latest commit SHA
	branchURL := fmt.Sprintf("https://api.github.com/repos/%s/branches/%s", repo, branch)
	branchResp, err := githubRequest(branchURL, token)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}
	defer branchResp.Body.Close()

	if branchResp.StatusCode != 200 {
		return nil, fmt.Errorf("branch request failed: %s", branchResp.Status)
	}

	var branchData BranchResponse
	if err := json.NewDecoder(branchResp.Body).Decode(&branchData); err != nil {
		return nil, fmt.Errorf("decode branch: %w", err)
	}

	// Now get the full commit details including stats
	commitURL := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, branchData.Commit.SHA)
	commitResp, err := githubRequest(commitURL, token)
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}
	defer commitResp.Body.Close()

	if commitResp.StatusCode != 200 {
		return nil, fmt.Errorf("commit request failed: %s", commitResp.Status)
	}

	var commit Commit
	if err := json.NewDecoder(commitResp.Body).Decode(&commit); err != nil {
		return nil, fmt.Errorf("decode commit: %w", err)
	}

	return &commit, nil
}

func githubRequest(url, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "monitor-probe")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return http.DefaultClient.Do(req)
}

func printDescription() {
	desc := Description{
		Name:        "github",
		Description: "Check GitHub repository commit activity",
		Version:     "1.0.0",
		Arguments: Arguments{
			Required: map[string]ArgSpec{
				"repo": {
					Type:        "string",
					Description: "Repository (owner/name)",
				},
			},
			Optional: map[string]ArgSpec{
				"branch": {
					Type:        "string",
					Description: "Branch name",
					Default:     "main",
				},
				"max_age_hours": {
					Type:        "number",
					Description: "Maximum commit age in hours (0 to disable)",
					Default:     24,
				},
				"min_files": {
					Type:        "number",
					Description: "Minimum changed files (0 to disable)",
					Default:     0,
				},
				"min_additions": {
					Type:        "number",
					Description: "Minimum added lines (0 to disable)",
					Default:     0,
				},
			},
		},
	}
	json.NewEncoder(os.Stdout).Encode(desc)
}

func parseCommitMessage(msg string) (title, body string) {
	parts := strings.SplitN(msg, "\n", 2)
	title = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	return
}

func formatCommitMessage(repo string, commit *Commit, commitURL string) string {
	var sb strings.Builder

	title, body := parseCommitMessage(commit.Commit.Message)

	// Commit link and title
	sb.WriteString(fmt.Sprintf("[%s](%s) **%s**\n\n", commit.SHA[:7], commitURL, title))

	// Commit body if present
	if body != "" {
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}

	// Stats line
	sb.WriteString(fmt.Sprintf("**+%d** / **-%d** in %d files",
		commit.Stats.Additions, commit.Stats.Deletions, len(commit.Files)))

	return sb.String()
}

func output(status, message string) {
	result := Result{
		Status:  status,
		Message: message,
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
