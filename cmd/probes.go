package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jandubois/monitor/internal/probe"
	"github.com/jandubois/monitor/internal/probes"
	"github.com/jandubois/monitor/internal/probes/command"
	"github.com/jandubois/monitor/internal/probes/debug"
	"github.com/jandubois/monitor/internal/probes/diskspace"
	"github.com/jandubois/monitor/internal/probes/github"
	"github.com/jandubois/monitor/internal/probes/gitstatus"
	"github.com/spf13/cobra"
)

// disk-space probe
var diskSpaceCmd = &cobra.Command{
	Use:   diskspace.Name,
	Short: "Check available disk space on a path",
	Run: func(cmd *cobra.Command, args []string) {
		path, _ := cmd.Flags().GetString("path")
		minFreeGB, _ := cmd.Flags().GetFloat64("min_free_gb")
		minFreePercent, _ := cmd.Flags().GetFloat64("min_free_percent")

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
		okCodes, _ := cmd.Flags().GetString("ok_codes")
		warningCodes, _ := cmd.Flags().GetString("warning_codes")
		captureOutput, _ := cmd.Flags().GetBool("capture_output")

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
		delayMs, _ := cmd.Flags().GetInt("delay_ms")

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
		maxAgeHours, _ := cmd.Flags().GetInt("max_age_hours")
		minFiles, _ := cmd.Flags().GetInt("min_files")
		minAdditions, _ := cmd.Flags().GetInt("min_additions")

		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}

		result := github.Run(repo, branch, token, maxAgeHours, minFiles, minAdditions)
		outputResult(result)
	},
}

// git-status probe
var gitStatusCmd = &cobra.Command{
	Use:   gitstatus.Name,
	Short: "Check git repositories for uncommitted changes and unpushed commits",
	Run: func(cmd *cobra.Command, args []string) {
		path, _ := cmd.Flags().GetString("path")
		uncommittedHours, _ := cmd.Flags().GetFloat64("uncommitted_hours")
		unpushedHours, _ := cmd.Flags().GetFloat64("unpushed_hours")
		excludeAIFiles, _ := cmd.Flags().GetBool("exclude_ai_files")

		result := gitstatus.Run(path, uncommittedHours, unpushedHours, excludeAIFiles)
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
		cmd.Help()
	}

	// Add probe subcommands
	diskSpaceCmd.GroupID = probeGroupID
	commandCmd.GroupID = probeGroupID
	debugCmd.GroupID = probeGroupID
	githubCmd.GroupID = probeGroupID
	gitStatusCmd.GroupID = probeGroupID
	rootCmd.AddCommand(diskSpaceCmd)
	rootCmd.AddCommand(commandCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(githubCmd)
	rootCmd.AddCommand(gitStatusCmd)

	// disk-space flags
	diskSpaceCmd.Flags().String("path", "", "Path to check")
	diskSpaceCmd.Flags().Float64("min_free_gb", 10, "Minimum free gigabytes")
	diskSpaceCmd.Flags().Float64("min_free_percent", 0, "Minimum free percentage (0-100)")

	// command flags
	commandCmd.Flags().String("command", "", "Command to run")
	commandCmd.Flags().String("shell", "/bin/sh", "Shell to use for execution")
	commandCmd.Flags().String("ok_codes", "0", "Comma-separated exit codes that indicate success")
	commandCmd.Flags().String("warning_codes", "", "Comma-separated exit codes that indicate warning")
	commandCmd.Flags().Bool("capture_output", true, "Include command output in result data")

	// debug flags
	debugCmd.Flags().String("mode", "ok", "Probe behavior mode")
	debugCmd.Flags().String("message", "", "Custom message to return")
	debugCmd.Flags().Int("delay_ms", 0, "Delay before responding (milliseconds)")

	// github flags
	githubCmd.Flags().String("repo", "", "Repository (owner/name)")
	githubCmd.Flags().String("branch", "main", "Branch name")
	githubCmd.Flags().Int("max_age_hours", 24, "Maximum commit age in hours (0 to disable)")
	githubCmd.Flags().Int("min_files", 0, "Minimum changed files (0 to disable)")
	githubCmd.Flags().Int("min_additions", 0, "Minimum added lines (0 to disable)")

	// git-status flags
	gitStatusCmd.Flags().String("path", "", "Directory to scan for git repositories")
	gitStatusCmd.Flags().Float64("uncommitted_hours", 1, "Hours after which uncommitted changes are a failure")
	gitStatusCmd.Flags().Float64("unpushed_hours", 4, "Hours after which unpushed commits are a failure")
	gitStatusCmd.Flags().Bool("exclude_ai_files", false, "Exclude AI agent files from uncommitted changes check")
}

func printDescriptions() {
	descs := probes.GetAllDescriptions()
	json.NewEncoder(os.Stdout).Encode(descs)
}

func outputResult(result *probe.Result) {
	json.NewEncoder(os.Stdout).Encode(result)
}
