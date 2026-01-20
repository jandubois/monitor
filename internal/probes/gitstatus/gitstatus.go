// Package gitstatus provides the git-status probe implementation.
package gitstatus

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jankremlacek/monitor/internal/probe"
)

// Name is the probe subcommand name.
const Name = "git-status"

// RepoIssue represents issues found in a repository.
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

// GetDescription returns the probe description.
func GetDescription() probe.Description {
	return probe.Description{
		Name:        "git-status",
		Description: "Check git repositories for uncommitted changes and unpushed commits",
		Version:     "1.0.0",
		Subcommand:  Name,
		Arguments: probe.Arguments{
			Required: map[string]probe.ArgumentSpec{
				"path": {
					Type:        "string",
					Description: "Directory to scan for git repositories",
				},
			},
			Optional: map[string]probe.ArgumentSpec{
				"uncommitted_hours": {
					Type:        "number",
					Description: "Hours after which uncommitted changes are a failure",
					Default:     float64(1),
				},
				"unpushed_hours": {
					Type:        "number",
					Description: "Hours after which unpushed commits are a failure",
					Default:     float64(4),
				},
				"exclude_ai_files": {
					Type:        "boolean",
					Description: "Exclude AI agent files (CLAUDE.md, .claude/, etc.) from uncommitted changes check",
					Default:     false,
				},
			},
		},
	}
}

// Run executes the probe with the given arguments.
func Run(root string, uncommittedHours, unpushedHours float64, excludeAIFiles bool) *probe.Result {
	if root == "" {
		return &probe.Result{
			Status:  probe.StatusCritical,
			Message: "path argument is required",
		}
	}

	repos := findGitRepos(root)
	if len(repos) == 0 {
		return &probe.Result{
			Status:  probe.StatusOK,
			Message: fmt.Sprintf("No git repositories found in %s", root),
		}
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
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%d repositories with issues:**\n\n", len(failures)))
		for _, f := range failures {
			relPath, _ := filepath.Rel(root, f.Path)
			if relPath == "" {
				relPath = filepath.Base(f.Path)
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", relPath, strings.Join(f.Issues, ", ")))
		}
		return &probe.Result{
			Status:  probe.StatusCritical,
			Message: sb.String(),
			Metrics: metrics,
			Data:    data,
		}
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

	return &probe.Result{
		Status:  probe.StatusOK,
		Message: msg,
		Metrics: metrics,
		Data:    data,
	}
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

		if isGitIgnored(path) {
			return filepath.SkipDir
		}

		return nil
	})
	return repos
}

func isGitIgnored(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = filepath.Dir(path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	repoRoot := strings.TrimSpace(string(out))

	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}

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

	hasUncommitted, err := hasUncommittedChanges(repoPath, excludeAIFiles)
	if err != nil {
		return []string{fmt.Sprintf("failed to check status: %v", err)}, false
	}

	if hasUncommitted && hoursSinceCommit > uncommittedHours {
		issues = append(issues, fmt.Sprintf("uncommitted changes (%s)", formatDuration(hoursSinceCommit)))
		hasFailure = true
	}

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

	for _, line := range lines {
		if line == "" {
			continue
		}
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
			if strings.HasPrefix(filename, pattern) || strings.Contains(filename, "/"+pattern) {
				return true
			}
		} else {
			if filename == pattern || strings.HasSuffix(filename, "/"+pattern) {
				return true
			}
		}
	}
	return false
}

func hasUnpushedCommits(repoPath string) (unpushed bool, noRemote bool, err error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := cmd.Output()
	if err != nil {
		return false, false, err
	}
	branch := strings.TrimSpace(string(branchOut))

	cmd = exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	_, err = cmd.Output()
	if err != nil {
		return false, true, nil
	}

	cmd = exec.Command("git", "-C", repoPath, "log", branch+"@{upstream}..HEAD", "--oneline")
	out, err := cmd.Output()
	if err != nil {
		return false, false, err
	}

	return len(strings.TrimSpace(string(out))) > 0, false, nil
}
