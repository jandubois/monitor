package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/jandubois/monitor/internal/probe"
	"github.com/jandubois/monitor/internal/probes"
	"github.com/jandubois/monitor/internal/probes/command"
	"github.com/jandubois/monitor/internal/probes/debug"
	"github.com/jandubois/monitor/internal/probes/diskspace"
	"github.com/jandubois/monitor/internal/probes/github"
	"github.com/spf13/cobra"
)

// disk-space probe
var diskSpaceCmd = &cobra.Command{
	Use:   diskspace.Name,
	Short: "Check available disk space on a path",
	Run: func(cmd *cobra.Command, args []string) {
		path, _ := cmd.Flags().GetString("path")
		minFreeGB, _ := cmd.Flags().GetFloat64("min-free-gb")
		minFreePercent, _ := cmd.Flags().GetFloat64("min-free-percent")

		result := diskspace.Run(path, minFreeGB, minFreePercent)
		outputResult(result)
	},
}

// command probe
var commandCmd = &cobra.Command{
	Use:   command.Name,
	Short: "Run a command and check its exit code",
	Run: func(cmd *cobra.Command, args []string) {
		cmdStr, _ := cmd.Flags().GetString("command")
		shell, _ := cmd.Flags().GetString("shell")
		okCodes, _ := cmd.Flags().GetString("ok-codes")
		warningCodes, _ := cmd.Flags().GetString("warning-codes")
		captureOutput, _ := cmd.Flags().GetBool("capture-output")

		result := command.Run(cmdStr, shell, okCodes, warningCodes, captureOutput)
		outputResult(result)
	},
}

// debug probe
var debugCmd = &cobra.Command{
	Use:   debug.Name,
	Short: "Debug probe for testing failure modes",
	Run: func(cmd *cobra.Command, args []string) {
		mode, _ := cmd.Flags().GetString("mode")
		message, _ := cmd.Flags().GetString("message")
		delayMs, _ := cmd.Flags().GetInt("delay-ms")

		result := debug.Run(mode, message, delayMs)
		outputResult(result)
	},
}

// github probe
var githubCmd = &cobra.Command{
	Use:   github.Name,
	Short: "Check GitHub repository commit activity",
	Run: func(cmd *cobra.Command, args []string) {
		repo, _ := cmd.Flags().GetString("repo")
		branch, _ := cmd.Flags().GetString("branch")
		maxAgeHours, _ := cmd.Flags().GetInt("max-age-hours")
		minFiles, _ := cmd.Flags().GetInt("min-files")
		minAdditions, _ := cmd.Flags().GetInt("min-additions")

		token := getGitHubToken()

		result := github.Run(repo, branch, token, maxAgeHours, minFiles, minAdditions)
		outputResult(result)
	},
}

func init() {
	// Add flags to root
	rootCmd.Flags().BoolP("version", "v", false, "Print version and exit")
	rootCmd.Flags().Bool("describe", false, "Output built-in probe descriptions as JSON array")

	// Override Run to handle flags
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Printf("monitor version %s\n", Version)
			return
		}
		if describe, _ := cmd.Flags().GetBool("describe"); describe {
			printDescriptions()
			return
		}
		_ = cmd.Help()
	}

	// Add probe subcommands
	diskSpaceCmd.GroupID = probeGroupID
	commandCmd.GroupID = probeGroupID
	debugCmd.GroupID = probeGroupID
	githubCmd.GroupID = probeGroupID
	rootCmd.AddCommand(diskSpaceCmd)
	rootCmd.AddCommand(commandCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(githubCmd)

	// disk-space flags
	diskSpaceCmd.Flags().String("path", "", "Path to check")
	diskSpaceCmd.Flags().Float64("min-free-gb", 10, "Minimum free gigabytes")
	diskSpaceCmd.Flags().Float64("min-free-percent", 0, "Minimum free percentage (0-100)")

	// command flags
	commandCmd.Flags().String("command", "", "Command to run")
	commandCmd.Flags().String("shell", "/bin/sh", "Shell to use for execution")
	commandCmd.Flags().String("ok-codes", "0", "Comma-separated exit codes that indicate success")
	commandCmd.Flags().String("warning-codes", "", "Comma-separated exit codes that indicate warning")
	commandCmd.Flags().Bool("capture-output", true, "Include command output in result data")

	// debug flags
	debugCmd.Flags().String("mode", "ok", "Probe behavior mode")
	debugCmd.Flags().String("message", "", "Custom message to return")
	debugCmd.Flags().Int("delay-ms", 0, "Delay before responding (milliseconds)")

	// github flags
	githubCmd.Flags().String("repo", "", "Repository (owner/name)")
	githubCmd.Flags().String("branch", "main", "Branch name")
	githubCmd.Flags().Int("max-age-hours", 24, "Maximum commit age in hours (0 to disable)")
	githubCmd.Flags().Int("min-files", 0, "Minimum changed files (0 to disable)")
	githubCmd.Flags().Int("min-additions", 0, "Minimum added lines (0 to disable)")

}

func printDescriptions() {
	descs := probes.GetAllDescriptions()
	_ = json.NewEncoder(os.Stdout).Encode(descs)
}

func outputResult(result *probe.Result) {
	_ = json.NewEncoder(os.Stdout).Encode(result)
}

func getGitHubToken() string {
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	// Try gh auth token
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		slog.Error("failed to get GitHub token", "error", err,
			"hint", "set GH_TOKEN or run 'gh auth login'")
		return ""
	}
	return strings.TrimSpace(string(out))
}
