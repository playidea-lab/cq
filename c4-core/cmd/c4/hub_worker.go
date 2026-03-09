package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// workerConfigPath returns the default path for the worker config file.
// Overridable in tests via workerConfigPathOverride.
var workerConfigPathOverride string

func workerConfigPath() string {
	if workerConfigPathOverride != "" {
		return workerConfigPathOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".c5/config.yaml"
	}
	return filepath.Join(home, ".c5", "config.yaml")
}

// workerYAML is the schema for ~/.c5/config.yaml written by `cq hub worker init`.
type workerYAML struct {
	HubURL       string   `yaml:"hub_url"`
	APIKey       string   `yaml:"api_key"`
	Capabilities string   `yaml:"capabilities,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
	Name         string   `yaml:"name,omitempty"`
	Binary       string   `yaml:"binary,omitempty"` // override c5 binary path
}

var (
	workerInitHubURL         string
	workerInitAPIKey         string
	workerInitNonInteractive bool
	workerInstallDryRun      bool
	workerInstallUser        bool
)

// execCommandFunc is the interface used to run external binaries.
// Overridable in tests to inject a mock.
type execCommandFunc func(name string, args ...string) *exec.Cmd

// defaultExecCommand is the real implementation.
func defaultExecCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// workerExecCommand is used by runWorkerStart; replaced in tests.
var workerExecCommand execCommandFunc = defaultExecCommand

var hubWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Manage C5 worker on this machine",
	Long: `Manage the C5 Hub worker on this machine.

Subcommands:
  init    - Configure worker credentials (hub URL + API key)
  install - Install as a system service (systemd / launchd)
  start   - Start c5 worker subprocess
  status  - List workers registered with the Hub`,
}

var hubWorkerInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure worker credentials",
	Long: `Interactively configure the worker connection to the C5 Hub.

Saves credentials to ~/.c5/config.yaml.
Use --non-interactive with --hub-url and --api-key for automation.

Example:
  cq hub worker init
  cq hub worker init --non-interactive --hub-url https://hub.example.com --api-key secret`,
	RunE: runWorkerInit,
}

var hubWorkerInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install worker as a system service",
	Long: `Install the C5 worker as a system service.

On Linux: creates a systemd unit file.
On macOS: creates a launchd plist.

Use --dry-run to preview the service file without writing it.

Example:
  cq hub worker install
  cq hub worker install --dry-run
  cq hub worker install --user   (Linux only: user-level systemd unit)`,
	RunE: runWorkerInstall,
}

var hubWorkerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start c5 worker subprocess",
	Long: `Start the c5 worker process, reading config from ~/.c5/config.yaml.

Resolves the c5 binary via:
  1. PATH ("c5")
  2. $C5_BIN environment variable
  3. config.yaml hub.binary field

Example:
  cq hub worker start`,
	RunE: runWorkerStart,
}

var hubWorkerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "List workers registered with the Hub",
	Long: `Fetch and display all workers registered with the C5 Hub.

Reads hub_url and api_key from ~/.c5/config.yaml.

Example:
  cq hub worker status`,
	RunE: runWorkerStatus,
}

func init() {
	hubWorkerInitCmd.Flags().StringVar(&workerInitHubURL, "hub-url", "", "Hub URL (non-interactive mode)")
	hubWorkerInitCmd.Flags().StringVar(&workerInitAPIKey, "api-key", "", "API key (non-interactive mode)")
	hubWorkerInitCmd.Flags().BoolVar(&workerInitNonInteractive, "non-interactive", false, "Skip prompts; use --hub-url and --api-key flags")

	hubWorkerInstallCmd.Flags().BoolVar(&workerInstallDryRun, "dry-run", false, "Print service file to stdout without writing")
	hubWorkerInstallCmd.Flags().BoolVar(&workerInstallUser, "user", false, "Install as user-level systemd unit (Linux only)")

	hubWorkerCmd.AddCommand(hubWorkerInitCmd)
	hubWorkerCmd.AddCommand(hubWorkerInstallCmd)
	hubWorkerCmd.AddCommand(hubWorkerStartCmd)
	hubWorkerCmd.AddCommand(hubWorkerStatusCmd)
	hubCmd.AddCommand(hubWorkerCmd)
}

// =========================================================================
// cq hub worker init
// =========================================================================

func runWorkerInit(cmd *cobra.Command, args []string) error {
	cfgPath := workerConfigPath()

	// Auto non-interactive: if both flags are provided, skip prompts without requiring --non-interactive.
	if workerInitHubURL != "" && workerInitAPIKey != "" {
		workerInitNonInteractive = true
	}

	// Load existing config for defaults.
	existing := workerYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	var hubURL, apiKey string

	if workerInitNonInteractive {
		hubURL = workerInitHubURL
		apiKey = workerInitAPIKey
		if hubURL == "" || apiKey == "" {
			return errors.New("--non-interactive requires both --hub-url and --api-key")
		}
	} else {
		// Interactive prompts with existing values as defaults.
		// Share one reader to avoid losing buffered stdin between calls.
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

	// GPU detection — warn but continue on failure.
	gpuInfo := detectWorkerGPU()
	if gpuInfo == "" {
		fmt.Fprintln(os.Stderr, "Warning: GPU not detected (nvidia-smi unavailable) — proceeding as CPU-only worker")
	} else {
		fmt.Printf("GPU detected: %s\n", gpuInfo)
	}

	cfg := workerYAML{
		HubURL: hubURL,
		APIKey: apiKey,
		Name:   existing.Name,
		Tags:   existing.Tags,
	}
	if existing.Capabilities != "" {
		cfg.Capabilities = existing.Capabilities
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Atomic write: write to temp file then rename to avoid corrupt config on crash.
	tmpPath := cfgPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		os.Remove(tmpPath) // best-effort cleanup of partial write
		return fmt.Errorf("write config temp: %w", err)
	}
	if err := os.Rename(tmpPath, cfgPath); err != nil {
		os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("install config: %w", err)
	}

	fmt.Printf("Worker config saved: %s\n", cfgPath)
	return nil
}

// prompt prints a prompt with optional default and reads one line from reader.
// Callers should share a single bufio.Reader over os.Stdin to avoid losing buffered input.
func prompt(label, defaultVal string, reader *bufio.Reader) (string, error) {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

// detectWorkerGPU runs nvidia-smi with a 5 s timeout and returns a one-line summary, or "" if unavailable.
func detectWorkerGPU() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// =========================================================================
// cq hub worker install
// =========================================================================

func runWorkerInstall(cmd *cobra.Command, args []string) error {
	cfgPath := workerConfigPath()

	// Read config for the ExecStart command.
	cfg := workerYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	// Build the exec start command: c5 worker --server <url> [--capabilities <file>]
	// systemd ExecStart uses whitespace as the argument delimiter with no shell quoting,
	// so validate that neither HubURL nor Capabilities contain whitespace.
	if strings.ContainsAny(cfg.HubURL, " \t") {
		return errors.New("hub_url must not contain whitespace (incompatible with systemd ExecStart)")
	}
	if strings.ContainsAny(cfg.Capabilities, " \t") {
		return errors.New("capabilities path must not contain whitespace (incompatible with systemd ExecStart)")
	}
	execArgs := []string{"c5", "worker"}
	if cfg.HubURL != "" {
		execArgs = append(execArgs, "--server", cfg.HubURL)
	}
	if cfg.Capabilities != "" {
		execArgs = append(execArgs, "--capabilities", cfg.Capabilities)
	}
	execStart := strings.Join(execArgs, " ")

	var content, destPath string

	switch runtime.GOOS {
	case "linux":
		if workerInstallUser {
			home, _ := os.UserHomeDir()
			destPath = filepath.Join(home, ".config", "systemd", "user", "cq-worker.service")
		} else {
			destPath = "/etc/systemd/system/cq-worker.service"
		}
		content = buildSystemdUnit(execStart, cfg.HubURL, cfg.APIKey)

	case "darwin":
		home, _ := os.UserHomeDir()
		destPath = filepath.Join(home, "Library", "LaunchAgents", "cq.worker.plist")
		content = buildLaunchdPlist(execArgs, cfg.HubURL, cfg.APIKey)

	default:
		return fmt.Errorf("unsupported OS: %s (supported: linux, darwin)", runtime.GOOS)
	}

	if workerInstallDryRun {
		fmt.Printf("# dry-run: would write to %s\n", destPath)
		fmt.Print(content)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create service dir: %w", err)
	}
	// User-mode units live in the user's home; restrict to owner only.
	// System-mode units in /etc/systemd/system must be world-readable (0644).
	perm := fs.FileMode(0o644)
	if workerInstallUser {
		perm = 0o600
	}
	if err := os.WriteFile(destPath, []byte(content), perm); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	fmt.Printf("Service file written: %s\n", destPath)
	if runtime.GOOS == "linux" {
		if workerInstallUser {
			fmt.Println("Enable with: systemctl --user enable --now cq-worker")
		} else {
			fmt.Println("Enable with: sudo systemctl enable --now cq-worker")
		}
	} else {
		fmt.Printf("Enable with: launchctl load %s\n", destPath)
	}
	return nil
}

func buildSystemdUnit(execStart, hubURL, apiKey string) string {
	// Strip newlines; escape backslash and double-quote inside the quoted
	// Environment= value to prevent systemd unit-file injection.
	sanitize := strings.NewReplacer("\n", "", "\r", "").Replace
	sanitizeEnv := strings.NewReplacer("\n", "", "\r", "", `\`, `\\`, `"`, `\"`).Replace
	execStart = sanitize(execStart)
	hubURL = sanitize(hubURL)
	apiKey = sanitizeEnv(apiKey)

	desc := "CQ Hub Worker"
	if hubURL != "" {
		desc = fmt.Sprintf("CQ Hub Worker (%s)", hubURL)
	}
	envLine := ""
	if apiKey != "" {
		envLine = fmt.Sprintf("Environment=\"C5_API_KEY=%s\"\n", apiKey)
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
`, desc, envLine, execStart)
}

// xmlEscapeAttr escapes a string for safe use inside XML element content.
func xmlEscapeAttr(s string) string {
	return html.EscapeString(s)
}

func buildLaunchdPlist(execArgs []string, hubURL, apiKey string) string {
	label := "cq.worker"
	var argsXML strings.Builder
	for _, a := range execArgs {
		argsXML.WriteString(fmt.Sprintf("        <string>%s</string>\n", xmlEscapeAttr(a)))
	}
	desc := "CQ Hub Worker"
	if hubURL != "" {
		desc = fmt.Sprintf("CQ Hub Worker (%s)", hubURL)
	}
	envBlock := ""
	if apiKey != "" {
		envBlock = fmt.Sprintf(`    <key>EnvironmentVariables</key>
    <dict>
        <key>C5_API_KEY</key>
        <string>%s</string>
    </dict>
`, xmlEscapeAttr(apiKey))
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
    <string>/tmp/cq-worker.err</string>
    <key>Comment</key>
    <string>%s</string>
</dict>
</plist>
`, xmlEscapeAttr(label), argsXML.String(), envBlock, xmlEscapeAttr(desc))
}

// =========================================================================
// cq hub worker start
// =========================================================================

// resolvec5Binary returns the path to the c5 binary.
// Resolution order: PATH → $C5_BIN env → config hub.binary field → "c5".
func resolvec5Binary(cfg workerYAML) string {
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

func runWorkerStart(cmd *cobra.Command, args []string) error {
	cfgPath := workerConfigPath()

	cfg := workerYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	binary := resolvec5Binary(cfg)

	// Build args: c5 worker [--server <url>] [--capabilities <file>] [--name <name>]
	cmdArgs := []string{"worker"}
	if cfg.HubURL != "" {
		cmdArgs = append(cmdArgs, "--server", cfg.HubURL)
	}
	if cfg.Capabilities != "" {
		cmdArgs = append(cmdArgs, "--capabilities", cfg.Capabilities)
	}
	if cfg.Name != "" {
		cmdArgs = append(cmdArgs, "--name", cfg.Name)
	}

	c := workerExecCommand(binary, cmdArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	// Inject C5_API_KEY from config into subprocess env.
	c.Env = append(os.Environ(), "C5_API_KEY="+cfg.APIKey)

	fmt.Printf("Starting: %s %s\n", binary, strings.Join(cmdArgs, " "))
	return c.Run()
}

// =========================================================================
// cq hub worker status
// =========================================================================

// workerStatusRow holds the display fields for one worker row.
type workerStatusRow struct {
	ID           string   `json:"id"`
	Hostname     string   `json:"hostname"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags"`
	UptimeSec    int64    `json:"uptime_sec"`
	LastJobAt    string   `json:"last_job_at"`
	Capabilities []string `json:"capabilities"`
}

func runWorkerStatus(cmd *cobra.Command, args []string) error {
	cfgPath := workerConfigPath()

	cfg := workerYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	if cfg.HubURL == "" {
		return fmt.Errorf("hub_url not set in %s — run: cq hub worker init", cfgPath)
	}

	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/workers"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var workers []workerStatusRow
	if err := json.Unmarshal(body, &workers); err != nil {
		return fmt.Errorf("decode workers: %w", err)
	}

	if len(workers) == 0 {
		fmt.Println("No workers registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSTATUS\tUPTIME\tLAST JOB\tCAPABILITIES\n")
	for _, wk := range workers {
		name := wk.Name
		if name == "" {
			name = wk.Hostname
		}
		if name == "" {
			name = wk.ID
		}
		caps := wk.Capabilities
		if len(caps) == 0 {
			caps = wk.Tags
		}
		capsStr := "-"
		if len(caps) > 0 {
			capsStr = strings.Join(caps, ",")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			name, wk.Status, formatUptime(wk.UptimeSec), formatLastJob(wk.LastJobAt), capsStr)
	}
	w.Flush()
	return nil
}
