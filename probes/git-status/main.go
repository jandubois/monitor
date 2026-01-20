package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

type Result struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type RepoIssue struct {
	Path    string   `json:"path"`
	Issues  []string `json:"issues"`
	Warning bool     `json:"warning,omitempty"`
}

// AI agent file patterns to exclude when exclude_ai_files is enabled
var aiFilePatterns = []string{
	"CLAUDE.md",
	"AGENTS.md",
	"GEMINI.md",
	"COPILOT.md",
	".claude/",
	".cursor/",
	".github/copilot",
}

func main() {
	describe := flag.Bool("describe", false, "Print probe description")
	path := flag.String("path", "", "Directory to scan for git repositories")
	uncommittedHours := flag.Float64("uncommitted_hours", 1, "Hours after which uncommitted changes are a failure")
	unpushedHours := flag.Float64("unpushed_hours", 4, "Hours after which unpushed commits are a failure")
	excludeAIFiles := flag.Bool("exclude_ai_files", false, "Exclude AI agent files from uncommitted changes check")
	flag.Parse()

	if *describe {
		printDescription()
		return
	}

	if *path == "" {
		output("critical", "path argument is required")
		return
	}

	checkGitRepos(*path, *uncommittedHours, *unpushedHours, *excludeAIFiles)
}

func printDescription() {
	desc := Description{
		Name:        "git-status",
		Description: "Check git repositories for uncommitted changes and unpushed commits",
		Version:     "1.0.0",
		Arguments: Arguments{
			Required: map[string]ArgSpec{
				"path": {
					Type:        "string",
					Description: "Directory to scan for git repositories",
				},
			},
			Optional: map[string]ArgSpec{
				"uncommitted_hours": {
					Type:        "number",
					Description: "Hours after which uncommitted changes are a failure",
					Default:     1,
				},
				"unpushed_hours": {
					Type:        "number",
					Description: "Hours after which unpushed commits are a failure",
					Default:     4,
				},
				"exclude_ai_files": {
					Type:        "boolean",
					Description: "Exclude AI agent files (CLAUDE.md, .claude/, etc.) from uncommitted changes check",
					Default:     false,
				},
			},
		},
	}
	json.NewEncoder(os.Stdout).Encode(desc)
}

func formatDuration(hours float64) string {
	if hours < 24 {
		return fmt.Sprintf("%dh", int(hours))
	}
	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%d days", int(days))
	}
	weeks := days / 7
	if weeks < 5 {
		return fmt.Sprintf("%d weeks", int(weeks))
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%d months", int(months))
	}
	years := days / 365
	remainingMonths := int((days - years*365) / 30)
	if remainingMonths > 0 {
		return fmt.Sprintf("%dy %dmo", int(years), remainingMonths)
	}
	return fmt.Sprintf("%d years", int(years))
}

func checkGitRepos(root string, uncommittedHours, unpushedHours float64, excludeAIFiles bool) {
	repos := findGitRepos(root)
	if len(repos) == 0 {
		output("ok", fmt.Sprintf("No git repositories found in %s", root))
		return
	}

	var failures []RepoIssue
	var warnings []RepoIssue
	checkedCount := 0

	for _, repoPath := range repos {
		issues, isWarning := checkRepo(repoPath, uncommittedHours, unpushedHours, excludeAIFiles)
		if len(issues) > 0 {
			issue := RepoIssue{
				Path:    repoPath,
				Issues:  issues,
				Warning: isWarning,
			}
			if isWarning {
				warnings = append(warnings, issue)
			} else {
				failures = append(failures, issue)
			}
		}
		checkedCount++
	}

	metrics := map[string]any{
		"repos_checked": checkedCount,
		"repos_failed":  len(failures),
		"repos_warned":  len(warnings),
	}

	data := map[string]any{
		"failures": failures,
		"warnings": warnings,
	}

	if len(failures) > 0 {
		// Build Markdown message from failures
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%d repositories with issues:**\n\n", len(failures)))
		for _, f := range failures {
			relPath, _ := filepath.Rel(root, f.Path)
			if relPath == "" {
				relPath = filepath.Base(f.Path)
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", relPath, strings.Join(f.Issues, ", ")))
		}
		result := Result{
			Status:  "critical",
			Message: sb.String(),
			Metrics: metrics,
			Data:    data,
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	var msg string
	if len(warnings) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%d repositories clean** (%d warnings)\n\n", checkedCount, len(warnings)))
		for _, w := range warnings {
			relPath, _ := filepath.Rel(root, w.Path)
			if relPath == "" {
				relPath = filepath.Base(w.Path)
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", relPath, strings.Join(w.Issues, ", ")))
		}
		msg = sb.String()
	} else {
		msg = fmt.Sprintf("%d repositories clean", checkedCount)
	}

	result := Result{
		Status:  "ok",
		Message: msg,
		Metrics: metrics,
		Data:    data,
	}
	json.NewEncoder(os.Stdout).Encode(result)
}

func findGitRepos(root string) []string {
	var repos []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		if name == ".git" {
			repos = append(repos, filepath.Dir(path))
			return filepath.SkipDir
		}

		// Skip directories gitignored by a containing repo
		if isGitIgnored(path) {
			return filepath.SkipDir
		}

		return nil
	})
	return repos
}

// isGitIgnored checks if a path is ignored by a containing git repo.
func isGitIgnored(path string) bool {
	// Find the containing repo root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = filepath.Dir(path)
	out, err := cmd.Output()
	if err != nil {
		return false // Not in a repo
	}
	repoRoot := strings.TrimSpace(string(out))

	// Get path relative to repo root
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}

	// Check if ignored using relative path
	cmd = exec.Command("git", "check-ignore", "-q", relPath)
	cmd.Dir = repoRoot
	err = cmd.Run()
	return err == nil
}

func checkRepo(repoPath string, uncommittedHours, unpushedHours float64, excludeAIFiles bool) (issues []string, isWarningOnly bool) {
	lastCommitTime, err := getLastCommitTime(repoPath)
	if err != nil {
		return []string{fmt.Sprintf("failed to get last commit: %v", err)}, false
	}

	hoursSinceCommit := time.Since(lastCommitTime).Hours()
	hasFailure := false

	// Check for uncommitted changes
	hasUncommitted, err := hasUncommittedChanges(repoPath, excludeAIFiles)
	if err != nil {
		return []string{fmt.Sprintf("failed to check status: %v", err)}, false
	}

	if hasUncommitted && hoursSinceCommit > uncommittedHours {
		issues = append(issues, fmt.Sprintf("uncommitted changes (%s)", formatDuration(hoursSinceCommit)))
		hasFailure = true
	}

	// Check for unpushed commits
	unpushed, noRemote, err := hasUnpushedCommits(repoPath)
	if err != nil {
		return []string{fmt.Sprintf("failed to check push status: %v", err)}, false
	}

	if noRemote {
		issues = append(issues, "no remote configured")
	} else if unpushed && hoursSinceCommit > unpushedHours {
		issues = append(issues, fmt.Sprintf("unpushed commits (%s)", formatDuration(hoursSinceCommit)))
		hasFailure = true
	}

	if len(issues) == 0 {
		return nil, false
	}

	// Only a warning if all issues are "no remote" (no actual failures)
	return issues, !hasFailure
}

func getLastCommitTime(repoPath string) (time.Time, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--format=%cI")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
}

func hasUncommittedChanges(repoPath string, excludeAIFiles bool) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return false, nil
	}

	if !excludeAIFiles {
		return true, nil
	}

	// Filter out AI agent files
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Git status --porcelain format: XY filename
		// The filename starts at position 3
		if len(line) < 4 {
			continue
		}
		filename := strings.TrimSpace(line[3:])
		if !isAIFile(filename) {
			return true, nil
		}
	}
	return false, nil
}

func isAIFile(filename string) bool {
	for _, pattern := range aiFilePatterns {
		if strings.HasSuffix(pattern, "/") {
			// Directory pattern
			if strings.HasPrefix(filename, pattern) || strings.Contains(filename, "/"+pattern) {
				return true
			}
		} else {
			// File pattern
			if filename == pattern || strings.HasSuffix(filename, "/"+pattern) {
				return true
			}
		}
	}
	return false
}

func hasUnpushedCommits(repoPath string) (unpushed bool, noRemote bool, err error) {
	// Get current branch
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := cmd.Output()
	if err != nil {
		return false, false, err
	}
	branch := strings.TrimSpace(string(branchOut))

	// Get upstream tracking branch
	cmd = exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	_, err = cmd.Output()
	if err != nil {
		// No upstream configured
		return false, true, nil
	}

	// Check for unpushed commits
	cmd = exec.Command("git", "-C", repoPath, "log", branch+"@{upstream}..HEAD", "--oneline")
	out, err := cmd.Output()
	if err != nil {
		return false, false, err
	}

	return len(strings.TrimSpace(string(out))) > 0, false, nil
}

func output(status, message string) {
	result := Result{
		Status:  status,
		Message: message,
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
