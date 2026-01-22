package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

const launchAgentLabel = "io.github.jandubois.monitor"

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
        <string>{{.APIPort}}</string>
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

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)

	installCmd.Flags().String("name", "", "Unique watcher name (defaults to hostname)")
	installCmd.Flags().String("push-url", "http://localhost:8080", "URL of the web service")
	installCmd.Flags().Int("api-port", 8081, "Port for watcher API")
	installCmd.Flags().String("callback-url", "", "Callback URL override (default: http://<hostname>:<api-port>)")
	installCmd.Flags().String("auth-token", "", "Authentication token (or AUTH_TOKEN env var)")
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

	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Set up paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	logDir := filepath.Join(homeDir, "Library", "Logs", "monitor")
	plistPath := filepath.Join(launchAgentsDir, launchAgentLabel+".plist")

	// Create directories if needed
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if already installed
	if _, err := os.Stat(plistPath); err == nil {
		// Unload existing service first
		exec.Command("launchctl", "unload", plistPath).Run()
	}

	// Generate plist
	data := plistData{
		Label:       launchAgentLabel,
		Executable:  executable,
		Name:        name,
		PushURL:     pushURL,
		CallbackURL: callbackURL,
		APIPort:     apiPort,
		AuthToken:   authToken,
		LogDir:      logDir,
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

	// Load the service
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	fmt.Printf("Installed and started %s\n", launchAgentLabel)
	fmt.Printf("Logs: %s/monitor.log\n", logDir)
	fmt.Printf("Plist: %s\n", plistPath)
	return nil
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("uninstall command is only supported on macOS")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", launchAgentLabel+".plist")

	// Check if installed
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("service is not installed")
	}

	// Unload the service
	if err := exec.Command("launchctl", "unload", plistPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to unload service: %v\n", err)
	}

	// Remove the plist
	if err := os.Remove(plistPath); err != nil {
		return fmt.Errorf("failed to remove plist: %w", err)
	}

	fmt.Printf("Uninstalled %s\n", launchAgentLabel)
	return nil
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
