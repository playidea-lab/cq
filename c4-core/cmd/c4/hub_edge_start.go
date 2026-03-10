package main

import (
	"bufio"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// edgeConfigPathOverride is overridable in tests.
var edgeConfigPathOverride string

func edgeConfigPath() string {
	if edgeConfigPathOverride != "" {
		return edgeConfigPathOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".c5/edge.yaml"
	}
	return filepath.Join(home, ".c5", "edge.yaml")
}

// edgeYAML is the schema for ~/.c5/edge.yaml written by `cq hub edge init`.
type edgeYAML struct {
	HubURL          string `yaml:"hub_url"`
	APIKey          string `yaml:"api_key"`
	EdgeName        string `yaml:"edge_name,omitempty"`
	Workdir         string `yaml:"workdir,omitempty"`
	MetricsCommand  string `yaml:"metrics_command,omitempty"`
	MetricsInterval int    `yaml:"metrics_interval,omitempty"` // seconds
	DriveURL        string `yaml:"drive_url,omitempty"`
	DriveAPIKey     string `yaml:"drive_api_key,omitempty"`
	Binary          string `yaml:"binary,omitempty"` // override c5 binary path
}

var (
	edgeInitHubURL         string
	edgeInitAPIKey         string
	edgeInitNonInteractive bool
	edgeInstallDryRun      bool
	edgeInstallUser        bool
)

// edgeExecCommand is used by runEdgeStart; replaced in tests.
var edgeExecCommand execCommandFunc = defaultExecCommand

var hubEdgeInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure edge agent credentials",
	Long: `Interactively configure the edge agent connection to the C5 Hub.

Saves credentials to ~/.c5/edge.yaml.
Use --non-interactive with --hub-url and --api-key for automation.

Example:
  cq hub edge init
  cq hub edge init --non-interactive --hub-url https://hub.example.com --api-key secret`,
	RunE: runEdgeInit,
}

var hubEdgeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start c5 edge-agent subprocess",
	Long: `Start the c5 edge-agent process, reading config from ~/.c5/edge.yaml.

Resolves the c5 binary via:
  1. PATH ("c5")
  2. $C5_BIN environment variable
  3. edge.yaml binary field

Example:
  cq hub edge start`,
	RunE: runEdgeStart,
}

var hubEdgeInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install edge agent as a system service",
	Long: `Install the C5 edge agent as a system service.

On Linux: creates a systemd unit file (cq-edge.service).
On macOS: creates a launchd plist (cq.edge.plist).

Use --dry-run to preview the service file without writing it.

Example:
  cq hub edge install
  cq hub edge install --dry-run
  cq hub edge install --user   (Linux only: user-level systemd unit)`,
	RunE: runEdgeInstall,
}

func init() {
	hubEdgeInitCmd.Flags().StringVar(&edgeInitHubURL, "hub-url", "", "Hub URL (non-interactive mode)")
	hubEdgeInitCmd.Flags().StringVar(&edgeInitAPIKey, "api-key", "", "API key (non-interactive mode)")
	hubEdgeInitCmd.Flags().BoolVar(&edgeInitNonInteractive, "non-interactive", false, "Skip prompts; use --hub-url and --api-key flags")

	hubEdgeInstallCmd.Flags().BoolVar(&edgeInstallDryRun, "dry-run", false, "Print service file to stdout without writing")
	hubEdgeInstallCmd.Flags().BoolVar(&edgeInstallUser, "user", false, "Install as user-level systemd unit (Linux only)")

	hubEdgeCmd.AddCommand(hubEdgeInitCmd)
	hubEdgeCmd.AddCommand(hubEdgeStartCmd)
	hubEdgeCmd.AddCommand(hubEdgeInstallCmd)
}

// =========================================================================
// cq hub edge init
// =========================================================================

func runEdgeInit(cmd *cobra.Command, args []string) error {
	cfgPath := edgeConfigPath()

	if edgeInitHubURL != "" && edgeInitAPIKey != "" {
		edgeInitNonInteractive = true
	}

	existing := edgeYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	var hubURL, apiKey string

	if edgeInitNonInteractive {
		hubURL = edgeInitHubURL
		apiKey = edgeInitAPIKey
		if hubURL == "" || apiKey == "" {
			return errors.New("--non-interactive requires both --hub-url and --api-key")
		}
	} else {
		stdinReader := bufio.NewReader(os.Stdin)
		var err error
		hubURL, err = prompt("Hub URL", existing.HubURL, stdinReader)
		if err != nil {
			return err
		}
		apiKey, err = prompt("API key", existing.APIKey, stdinReader)
		if err != nil {
			return err
		}
	}

	cfg := edgeYAML{
		HubURL:          hubURL,
		APIKey:          apiKey,
		EdgeName:        existing.EdgeName,
		Workdir:         existing.Workdir,
		MetricsCommand:  existing.MetricsCommand,
		MetricsInterval: existing.MetricsInterval,
		DriveURL:        existing.DriveURL,
		DriveAPIKey:     existing.DriveAPIKey,
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := cfgPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write config temp: %w", err)
	}
	if err := os.Rename(tmpPath, cfgPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("install config: %w", err)
	}

	fmt.Printf("Edge agent config saved: %s\n", cfgPath)
	fmt.Println("Run `cq hub edge start` to launch the agent.")
	return nil
}

// =========================================================================
// cq hub edge start
// =========================================================================

func resolveC5BinaryForEdge(cfg edgeYAML) string {
	if p, err := exec.LookPath("c5"); err == nil {
		return p
	}
	if env := os.Getenv("C5_BIN"); env != "" {
		return env
	}
	if cfg.Binary != "" {
		return cfg.Binary
	}
	return "c5"
}

func runEdgeStart(cmd *cobra.Command, args []string) error {
	cfgPath := edgeConfigPath()

	cfg := edgeYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	// Auto-init from env vars if config missing.
	if cfg.HubURL == "" {
		envURL := os.Getenv("C5_HUB_URL")
		envKey := os.Getenv("C5_API_KEY")
		if envURL != "" && envKey != "" {
			fmt.Println("No edge config found — auto-initializing from C5_HUB_URL / C5_API_KEY...")
			edgeInitHubURL = envURL
			edgeInitAPIKey = envKey
			edgeInitNonInteractive = true
			if err := runEdgeInit(nil, nil); err != nil {
				return fmt.Errorf("auto-init: %w", err)
			}
			if data, err := os.ReadFile(cfgPath); err == nil {
				_ = yaml.Unmarshal(data, &cfg)
			}
		} else {
			return errors.New("hub_url not set — run: cq hub edge init, or set C5_HUB_URL + C5_API_KEY env vars")
		}
	}

	binary := resolveC5BinaryForEdge(cfg)

	// Build args: c5 edge-agent [flags...]
	cmdArgs := []string{"edge-agent"}
	if cfg.HubURL != "" {
		cmdArgs = append(cmdArgs, "--hub-url", cfg.HubURL)
	}
	if cfg.EdgeName != "" {
		cmdArgs = append(cmdArgs, "--edge-name", cfg.EdgeName)
	}
	if cfg.Workdir != "" {
		cmdArgs = append(cmdArgs, "--workdir", cfg.Workdir)
	}
	if cfg.MetricsCommand != "" {
		cmdArgs = append(cmdArgs, "--metrics-command", cfg.MetricsCommand)
	}
	if cfg.MetricsInterval > 0 {
		cmdArgs = append(cmdArgs, "--metrics-interval", fmt.Sprintf("%d", cfg.MetricsInterval))
	}
	if cfg.DriveURL != "" {
		cmdArgs = append(cmdArgs, "--drive-url", cfg.DriveURL)
	}
	// DriveAPIKey is passed via C5_DRIVE_API_KEY env var (not CLI arg) to avoid
	// ps-visible exposure. c5 edge-agent reads it from env as a fallback.

	fmt.Printf("Starting: %s %s\n", binary, strings.Join(cmdArgs, " "))

	c := edgeExecCommand(binary, cmdArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	env := os.Environ()
	if cfg.APIKey != "" {
		env = append(env, "C5_API_KEY="+cfg.APIKey)
	}
	if cfg.DriveAPIKey != "" {
		env = append(env, "C5_DRIVE_API_KEY="+cfg.DriveAPIKey)
	}
	c.Env = env
	return c.Run()
}

// =========================================================================
// cq hub edge install
// =========================================================================

func runEdgeInstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS == "linux" && !edgeInstallUser && !edgeInstallDryRun && os.Getuid() != 0 {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot find own binary path: %w", err)
		}
		fmt.Println("Root required — escalating with sudo...")
		sudoCmd := exec.Command("sudo", self, "hub", "edge", "install")
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		sudoCmd.Stdin = os.Stdin
		return sudoCmd.Run()
	}

	cfgPath := edgeConfigPath()
	cfg := edgeYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	binary := resolveC5BinaryForEdge(cfg)

	// Build ExecStart args.
	execArgs := []string{binary, "edge-agent"}
	if cfg.HubURL != "" {
		execArgs = append(execArgs, "--hub-url", cfg.HubURL)
	}
	if cfg.EdgeName != "" {
		execArgs = append(execArgs, "--edge-name", cfg.EdgeName)
	}
	if cfg.Workdir != "" {
		execArgs = append(execArgs, "--workdir", cfg.Workdir)
	}
	if cfg.MetricsCommand != "" {
		execArgs = append(execArgs, "--metrics-command", cfg.MetricsCommand)
	}
	if cfg.MetricsInterval > 0 {
		execArgs = append(execArgs, "--metrics-interval", fmt.Sprintf("%d", cfg.MetricsInterval))
	}
	if cfg.DriveURL != "" {
		execArgs = append(execArgs, "--drive-url", cfg.DriveURL)
	}

	var content, destPath string

	switch runtime.GOOS {
	case "linux":
		if edgeInstallUser {
			home, _ := os.UserHomeDir()
			destPath = filepath.Join(home, ".config", "systemd", "user", "cq-edge.service")
		} else {
			destPath = "/etc/systemd/system/cq-edge.service"
		}
		execStart := strings.Join(execArgs, " ")
		content = buildEdgeSystemdUnit(execStart, cfg.HubURL, cfg.APIKey, cfg.DriveAPIKey)

	case "darwin":
		home, _ := os.UserHomeDir()
		destPath = filepath.Join(home, "Library", "LaunchAgents", "cq.edge.plist")
		content = buildEdgeLaunchdPlist(execArgs, cfg.HubURL, cfg.APIKey, cfg.DriveAPIKey)

	default:
		return errors.New("unsupported OS: " + runtime.GOOS + " (supported: linux, darwin)")
	}

	if edgeInstallDryRun {
		fmt.Printf("# dry-run: would write to %s\n", destPath)
		fmt.Print(content)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create service dir: %w", err)
	}
	perm := os.FileMode(0o644)
	if edgeInstallUser {
		perm = 0o600
	}
	if err := os.WriteFile(destPath, []byte(content), perm); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	fmt.Printf("Service file written: %s\n", destPath)

	if runtime.GOOS == "linux" {
		unitName := "cq-edge"
		if edgeInstallUser {
			exec.Command("systemctl", "--user", "daemon-reload").Run() //nolint:errcheck
			startCmd := exec.Command("systemctl", "--user", "enable", "--now", unitName)
			startCmd.Stdout = os.Stdout
			startCmd.Stderr = os.Stderr
			if err := startCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: systemctl --user enable --now %s\n", err, unitName)
			} else {
				fmt.Println("Edge agent service started.")
			}
		} else {
			exec.Command("sudo", "systemctl", "daemon-reload").Run() //nolint:errcheck
			startCmd := exec.Command("sudo", "systemctl", "enable", "--now", unitName)
			startCmd.Stdout = os.Stdout
			startCmd.Stderr = os.Stderr
			if err := startCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: sudo systemctl enable --now %s\n", err, unitName)
			} else {
				fmt.Println("Edge agent service started.")
				fmt.Println("Logs: sudo journalctl -fu cq-edge")
			}
		}
	} else if runtime.GOOS == "darwin" {
		loadCmd := exec.Command("launchctl", "load", destPath)
		loadCmd.Stdout = os.Stdout
		loadCmd.Stderr = os.Stderr
		if err := loadCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: launchctl load %s\n", err, destPath)
		} else {
			fmt.Println("Edge agent service started.")
		}
	}
	return nil
}

func buildEdgeSystemdUnit(execStart, hubURL, apiKey, driveAPIKey string) string {
	sanitize := strings.NewReplacer("\n", "", "\r", "").Replace
	sanitizeEnv := strings.NewReplacer("\n", "", "\r", "", `\`, `\\`, `"`, `\"`).Replace
	execStart = sanitize(execStart)
	hubURL = sanitize(hubURL)
	apiKey = sanitizeEnv(apiKey)
	driveAPIKey = sanitizeEnv(driveAPIKey)

	desc := "CQ Hub Edge Agent"
	if hubURL != "" {
		desc = fmt.Sprintf("CQ Hub Edge Agent (%s)", hubURL)
	}
	var envLines string
	if apiKey != "" {
		envLines += fmt.Sprintf("Environment=\"C5_API_KEY=%s\"\n", apiKey)
	}
	if driveAPIKey != "" {
		envLines += fmt.Sprintf("Environment=\"C5_DRIVE_API_KEY=%s\"\n", driveAPIKey)
	}
	return fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
%sExecStart=%s
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
`, desc, envLines, execStart)
}

func buildEdgeLaunchdPlist(execArgs []string, hubURL, apiKey, driveAPIKey string) string {
	label := "cq.edge"
	var argsXML strings.Builder
	for _, a := range execArgs {
		argsXML.WriteString(fmt.Sprintf("        <string>%s</string>\n", html.EscapeString(a)))
	}
	desc := "CQ Hub Edge Agent"
	if hubURL != "" {
		desc = fmt.Sprintf("CQ Hub Edge Agent (%s)", hubURL)
	}
	envBlock := ""
	if apiKey != "" || driveAPIKey != "" {
		var entries string
		if apiKey != "" {
			entries += fmt.Sprintf("        <key>C5_API_KEY</key>\n        <string>%s</string>\n", html.EscapeString(apiKey))
		}
		if driveAPIKey != "" {
			entries += fmt.Sprintf("        <key>C5_DRIVE_API_KEY</key>\n        <string>%s</string>\n", html.EscapeString(driveAPIKey))
		}
		envBlock = fmt.Sprintf("    <key>EnvironmentVariables</key>\n    <dict>\n%s    </dict>\n", entries)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
%s    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/tmp/cq-edge.err</string>
    <key>Comment</key>
    <string>%s</string>
</dict>
</plist>
`, html.EscapeString(label), argsXML.String(), envBlock, html.EscapeString(desc))
}
