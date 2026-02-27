package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

const launchAgentLabel = "com.jandubois.monitor"
const legacyLaunchAgentLabel = "io.github.jandubois.monitor"

var launchAgentPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.Executable}}</string>
        <string>watcher</string>
        <string>--name</string>
        <string>{{.Name}}</string>
        <string>--push-url</string>
        <string>{{.PushURL}}</string>
        <string>--callback-url</string>
        <string>{{.CallbackURL}}</string>
        <string>--api-port</string>
        <string>{{.APIPort}}</string>{{range .Probes}}
        <string>--probe</string>
        <string>{{.}}</string>{{end}}
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AUTH_TOKEN</key>
        <string>{{.AuthToken}}</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogDir}}/monitor.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/monitor.log</string>
</dict>
</plist>
`

type plistData struct {
	Label       string
	Executable  string
	Name        string
	PushURL     string
	CallbackURL string
	APIPort     int
	AuthToken   string
	LogDir      string
	Probes      []string
}

// installConfig holds the user-specified settings for a LaunchAgent installation.
type installConfig struct {
	Name        string
	PushURL     string
	CallbackURL string
	APIPort     int
	AuthToken   string
	Probes      []string
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install monitor watcher as a launchd service (macOS)",
	Long: `Install the monitor watcher as a macOS LaunchAgent that starts on login
and runs continuously in the background.

The service will be installed to ~/Library/LaunchAgents and will restart
automatically if it crashes.`,
	RunE: runInstall,
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall monitor watcher service (macOS)",
	Long:  `Stop and remove the monitor watcher LaunchAgent.`,
	RunE:  runUninstall,
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Reinstall monitor watcher service with current binary (macOS)",
	Long:  `Reinstall the LaunchAgent using settings from the existing installation.`,
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(updateCmd)

	installCmd.Flags().String("name", "", "Unique watcher name (defaults to hostname)")
	installCmd.Flags().String("push-url", "http://localhost:8080", "URL of the web service")
	installCmd.Flags().Int("api-port", 8081, "Port for watcher API")
	installCmd.Flags().String("callback-url", "", "Callback URL override (default: http://<hostname>:<api-port>)")
	installCmd.Flags().String("auth-token", "", "Authentication token (or AUTH_TOKEN env var)")
	installCmd.Flags().StringSlice("probe", nil, "Additional probe executable path (can be repeated)")
}

func runInstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("install command is only supported on macOS")
	}

	name, _ := cmd.Flags().GetString("name")
	pushURL, _ := cmd.Flags().GetString("push-url")
	apiPort, _ := cmd.Flags().GetInt("api-port")
	callbackURL, _ := cmd.Flags().GetString("callback-url")
	authToken, _ := cmd.Flags().GetString("auth-token")
	probes, _ := cmd.Flags().GetStringSlice("probe")

	// Default name to short hostname (without domain)
	if name == "" {
		name = getShortHostname()
	}

	// Construct callback URL from full hostname and port if not explicitly set
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://%s:%d", getFullHostname(), apiPort)
	}

	// Allow auth token from environment
	if authToken == "" {
		authToken = os.Getenv("AUTH_TOKEN")
	}
	if authToken == "" {
		return fmt.Errorf("auth token required (use --auth-token or AUTH_TOKEN env var)")
	}

	return installService(&installConfig{
		Name:        name,
		PushURL:     pushURL,
		CallbackURL: callbackURL,
		APIPort:     apiPort,
		AuthToken:   authToken,
		Probes:      probes,
	})
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("update command is only supported on macOS")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	cfg, err := readInstalledConfig(homeDir)
	if err != nil {
		return err
	}

	return installService(cfg)
}

func installService(cfg *installConfig) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	logDir := filepath.Join(homeDir, "Library", "Logs", "monitor")
	plistPath := filepath.Join(launchAgentsDir, launchAgentLabel+".plist")

	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Remove legacy and current plists before reinstalling
	removeLaunchAgent(homeDir, legacyLaunchAgentLabel)
	removeLaunchAgent(homeDir, launchAgentLabel)

	data := plistData{
		Label:       launchAgentLabel,
		Executable:  executable,
		Name:        cfg.Name,
		PushURL:     cfg.PushURL,
		CallbackURL: cfg.CallbackURL,
		APIPort:     cfg.APIPort,
		AuthToken:   cfg.AuthToken,
		LogDir:      logDir,
		Probes:      cfg.Probes,
	}

	tmpl, err := template.New("plist").Parse(launchAgentPlist)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	fmt.Printf("Installed and started %s\n", launchAgentLabel)
	fmt.Printf("Logs: %s/monitor.log\n", logDir)
	fmt.Printf("Plist: %s\n", plistPath)
	return nil
}

// readInstalledConfig extracts settings from an existing LaunchAgent plist.
func readInstalledConfig(homeDir string) (*installConfig, error) {
	agentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")

	// Try current label first, then legacy
	var plistPath string
	for _, label := range []string{launchAgentLabel, legacyLaunchAgentLabel} {
		p := filepath.Join(agentsDir, label+".plist")
		if _, err := os.Stat(p); err == nil {
			plistPath = p
			break
		}
	}
	if plistPath == "" {
		return nil, fmt.Errorf("service is not installed")
	}

	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plistPath).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read plist %s: %w", plistPath, err)
	}

	var contents struct {
		ProgramArguments     []string          `json:"ProgramArguments"`
		EnvironmentVariables map[string]string `json:"EnvironmentVariables"`
	}
	if err := json.Unmarshal(out, &contents); err != nil {
		return nil, fmt.Errorf("failed to parse plist %s: %w", plistPath, err)
	}

	cfg := &installConfig{
		AuthToken: contents.EnvironmentVariables["AUTH_TOKEN"],
	}

	// Parse flag pairs from ProgramArguments (skip executable and "watcher")
	progArgs := contents.ProgramArguments
	for i := 2; i < len(progArgs)-1; i += 2 {
		switch progArgs[i] {
		case "--name":
			cfg.Name = progArgs[i+1]
		case "--push-url":
			cfg.PushURL = progArgs[i+1]
		case "--callback-url":
			cfg.CallbackURL = progArgs[i+1]
		case "--api-port":
			cfg.APIPort, err = strconv.Atoi(progArgs[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid api-port in plist: %w", err)
			}
		case "--probe":
			cfg.Probes = append(cfg.Probes, progArgs[i+1])
		}
	}

	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("no AUTH_TOKEN found in existing plist")
	}

	return cfg, nil
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("uninstall command is only supported on macOS")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	removedLegacy := removeLaunchAgent(homeDir, legacyLaunchAgentLabel)
	removedCurrent := removeLaunchAgent(homeDir, launchAgentLabel)

	if !removedLegacy && !removedCurrent {
		return fmt.Errorf("service is not installed")
	}
	return nil
}

// removeLaunchAgent unloads and deletes a launch agent plist. Returns true if the plist existed.
func removeLaunchAgent(homeDir, label string) bool {
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return false
	}
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if err := os.Remove(plistPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", plistPath, err)
		return false
	}
	fmt.Printf("Uninstalled %s\n", label)
	return true
}

// getFullHostname returns the FQDN by doing a reverse DNS lookup on our IP.
func getFullHostname() string {
	// Find our non-loopback IPv4 address
	ifaces, err := net.Interfaces()
	if err != nil {
		return fallbackHostname()
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			// Do reverse DNS lookup
			names, err := net.LookupAddr(ipnet.IP.String())
			if err == nil && len(names) > 0 {
				// Remove trailing dot from DNS response
				return strings.TrimSuffix(names[0], ".")
			}
		}
	}

	return fallbackHostname()
}

func fallbackHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	if strings.Contains(hostname, ".") {
		return hostname
	}
	return hostname + ".local"
}
