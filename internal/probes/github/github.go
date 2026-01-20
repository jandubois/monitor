// Package github provides the GitHub repository monitoring probe.
package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jankremlacek/monitor/internal/probe"
)

// Name is the probe subcommand name.
const Name = "github"

// Commit represents a GitHub commit.
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

type branchResponse struct {
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// GetDescription returns the probe description.
func GetDescription() probe.Description {
	return probe.Description{
		Name:        "github",
		Description: "Check GitHub repository commit activity",
		Version:     "1.0.0",
		Subcommand:  Name,
		Arguments: probe.Arguments{
			Required: map[string]probe.ArgumentSpec{
				"repo": {
					Type:        "string",
					Description: "Repository (owner/name)",
				},
			},
			Optional: map[string]probe.ArgumentSpec{
				"branch": {
					Type:        "string",
					Description: "Branch name",
					Default:     "main",
				},
				"max_age_hours": {
					Type:        "number",
					Description: "Maximum commit age in hours (0 to disable)",
					Default:     float64(24),
				},
				"min_files": {
					Type:        "number",
					Description: "Minimum changed files (0 to disable)",
					Default:     float64(0),
				},
				"min_additions": {
					Type:        "number",
					Description: "Minimum added lines (0 to disable)",
					Default:     float64(0),
				},
			},
		},
	}
}

// Run executes the probe with the given arguments.
func Run(repo, branch, token string, maxAgeHours, minFiles, minAdditions int) *probe.Result {
	if repo == "" {
		return &probe.Result{
			Status:  probe.StatusCritical,
			Message: "repo argument is required",
		}
	}

	commit, err := getLastCommit(repo, branch, token)
	if err != nil {
		return &probe.Result{
			Status:  probe.StatusCritical,
			Message: fmt.Sprintf("Failed to get commit: %v", err),
		}
	}

	// Check conditions
	var failures []string
	commitAge := time.Since(commit.Commit.Author.Date)
	maxAge := time.Duration(maxAgeHours) * time.Hour

	if maxAgeHours > 0 && commitAge > maxAge {
		failures = append(failures, fmt.Sprintf("commit is %.1f hours old (max %d)", commitAge.Hours(), maxAgeHours))
	}

	filesChanged := len(commit.Files)
	if minFiles > 0 && filesChanged < minFiles {
		failures = append(failures, fmt.Sprintf("only %d files changed (min %d)", filesChanged, minFiles))
	}

	if minAdditions > 0 && commit.Stats.Additions < minAdditions {
		failures = append(failures, fmt.Sprintf("only %d lines added (min %d)", commit.Stats.Additions, minAdditions))
	}

	// Build result
	metrics := map[string]any{
		"age_hours":     commitAge.Hours(),
		"files_changed": filesChanged,
		"additions":     commit.Stats.Additions,
		"deletions":     commit.Stats.Deletions,
	}

	commitTitle, commitBody := parseCommitMessage(commit.Commit.Message)
	commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit.SHA)

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

	message := formatCommitMessage(repo, commit, commitURL)

	if len(failures) > 0 {
		return &probe.Result{
			Status:  probe.StatusCritical,
			Message: fmt.Sprintf("**Commit check failed:** %s\n\n%s", failures[0], message),
			Metrics: metrics,
			Data:    data,
		}
	}

	return &probe.Result{
		Status:  probe.StatusOK,
		Message: message,
		Metrics: metrics,
		Data:    data,
	}
}

func getLastCommit(repo, branch, token string) (*Commit, error) {
	branchURL := fmt.Sprintf("https://api.github.com/repos/%s/branches/%s", repo, branch)
	branchResp, err := githubRequest(branchURL, token)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}
	defer branchResp.Body.Close()

	if branchResp.StatusCode != 200 {
		return nil, fmt.Errorf("branch request failed: %s", branchResp.Status)
	}

	var branchData branchResponse
	if err := json.NewDecoder(branchResp.Body).Decode(&branchData); err != nil {
		return nil, fmt.Errorf("decode branch: %w", err)
	}

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

	sb.WriteString(fmt.Sprintf("[%s](%s) **%s**\n\n", commit.SHA[:7], commitURL, title))

	if body != "" {
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("**+%d** / **-%d** in %d files",
		commit.Stats.Additions, commit.Stats.Deletions, len(commit.Files)))

	return sb.String()
}
