package main

import (
	"bufio"
	"bytes"
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

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp/handlers/cfghandler"
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
		return ".c4/hub-worker.yaml"
	}
	return filepath.Join(home, ".c4", "hub-worker.yaml")
}

// workerYAML is the schema for ~/.c4/hub-worker.yaml written by `cq hub worker init`.
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

Saves credentials to ~/.c4/hub-worker.yaml.
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
	Long: `Start the hub worker process, reading config from ~/.c4/hub-worker.yaml.

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

Reads hub_url and api_key from ~/.c4/hub-worker.yaml.

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
	// System-level install on Linux requires root. If not root and not --user,
	// re-exec ourselves via sudo with the full binary path.
	if runtime.GOOS == "linux" && !workerInstallUser && !workerInstallDryRun && os.Getuid() != 0 {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot find own binary path: %w", err)
		}
		fmt.Println("Root required — escalating with sudo...")
		sudoArgs := []string{self, "hub", "worker", "install"}
		sudoCmd := exec.Command("sudo", sudoArgs...)
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		sudoCmd.Stdin = os.Stdin
		return sudoCmd.Run()
	}

	// Pre-flight: ensure Docker + NVIDIA Container Toolkit on Linux
	if runtime.GOOS == "linux" && !workerInstallDryRun {
		if err := ensureDockerRuntime(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Docker setup incomplete: %v\n", err)
			fmt.Fprintln(os.Stderr, "Docker runtime jobs will fall back to host execution.")
		}
	}

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

	// Auto-enable and start the service.
	if runtime.GOOS == "linux" {
		if workerInstallUser {
			fmt.Println("Enabling service (user mode)...")
			cmd := exec.Command("systemctl", "--user", "daemon-reload")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			cmd = exec.Command("systemctl", "--user", "enable", "--now", "cq-worker")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: systemctl --user enable --now cq-worker\n", err)
			} else {
				fmt.Println("Worker service started.")
			}
		} else {
			fmt.Println("Enabling service...")
			cmd := exec.Command("sudo", "systemctl", "daemon-reload")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			cmd = exec.Command("sudo", "systemctl", "enable", "--now", "cq-worker")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: sudo systemctl enable --now cq-worker\n", err)
			} else {
				fmt.Println("Worker service started.")
				fmt.Println("Logs: sudo journalctl -fu cq-worker")
			}
		}
	} else if runtime.GOOS == "darwin" {
		fmt.Println("Loading service...")
		cmd := exec.Command("launchctl", "load", destPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-start failed: %v\nManually run: launchctl load %s\n", err, destPath)
		} else {
			fmt.Println("Worker service started.")
		}
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
	// Docker group access: if docker group exists, add SupplementaryGroups
	dockerGroup := ""
	if _, err := exec.LookPath("docker"); err == nil {
		dockerGroup = "SupplementaryGroups=docker\n"
	}

	return fmt.Sprintf(`[Unit]
Description=%s
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
%s%sExecStart=%s
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
`, desc, envLine, dockerGroup, execStart)
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

// ensureDockerRuntime checks and installs Docker + NVIDIA Container Toolkit.
// Each step is idempotent — already-installed components are skipped.
func ensureDockerRuntime() error {
	// Step 1: Docker
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("Docker not found. Installing...")
		cmd := exec.Command("sudo", "sh", "-c", "curl -fsSL https://get.docker.com | sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker install failed: %w", err)
		}
		fmt.Println("Docker installed.")
	} else {
		fmt.Println("Docker: OK")
	}

	// Step 2: Add current user to docker group
	if u := os.Getenv("USER"); u != "" {
		// Check if already in docker group
		check := exec.Command("id", "-nG", u)
		out, _ := check.Output()
		if !strings.Contains(string(out), "docker") {
			fmt.Printf("Adding user %s to docker group...\n", u)
			cmd := exec.Command("sudo", "usermod", "-aG", "docker", u)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("WARNING: failed to add user to docker group: %v\n", err)
				fmt.Println("  Run manually: sudo usermod -aG docker $USER")
			} else {
				fmt.Println("NOTE: Run 'newgrp docker' or re-login for group change to take effect.")
			}
		}
	}

	// Step 3: NVIDIA Container Toolkit (GPU support)
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		// GPU exists — check nvidia-ctk
		if _, err := exec.LookPath("nvidia-ctk"); err != nil {
			fmt.Println("NVIDIA Container Toolkit not found. Installing...")
			script := "curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg && " +
				"curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | " +
				"sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' > /etc/apt/sources.list.d/nvidia-container-toolkit.list && " +
				"apt-get update -qq && apt-get install -y -qq nvidia-container-toolkit && " +
				"nvidia-ctk runtime configure --runtime=docker && " +
				"systemctl restart docker"
			cmd := exec.Command("sudo", "sh", "-c", script)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("nvidia-container-toolkit install failed: %w", err)
			}
			fmt.Println("NVIDIA Container Toolkit installed.")
		} else {
			fmt.Println("NVIDIA Container Toolkit: OK")
		}
	} else {
		fmt.Println("No GPU detected (nvidia-smi not found). Skipping NVIDIA toolkit.")
	}

	// Step 4: Verify docker runs
	verifyCmd := exec.Command("docker", "info", "--format", "{{.Runtimes}}")
	if out, err := verifyCmd.CombinedOutput(); err == nil {
		fmt.Printf("Docker runtimes: %s\n", strings.TrimSpace(string(out)))
	}

	return nil
}

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
	// Auto-fix docker group: if docker.sock exists but not accessible,
	// and user is in docker group (just not active in this session),
	// re-exec self under "sg docker" to activate the group.
	if runtime.GOOS == "linux" && os.Getenv("CQ_DOCKER_GROUP_FIXED") == "" {
		sock := "/var/run/docker.sock"
		if _, err := os.Stat(sock); err == nil {
			if f, err := os.OpenFile(sock, os.O_RDONLY, 0); err != nil {
				// Check if user is in docker group in /etc/group (but not active in session)
				u := os.Getenv("USER")
				check := exec.Command("id", "-nG", u)
				if out, e := check.Output(); e == nil && !strings.Contains(string(out), "docker") {
					// Check /etc/group directly — usermod may have added but session not refreshed
					grepCmd := exec.Command("grep", "-q", "docker.*"+u, "/etc/group")
					if grepCmd.Run() == nil {
						fmt.Println("Docker group not active in session — re-launching with newgrp docker...")
						self, _ := os.Executable()
						// newgrp docker -c is not standard; use sh -c under newgrp via sg
						// newgrp changes primary group and is password-free for members
						shCmd := exec.Command("bash", "-c",
							fmt.Sprintf("newgrp docker <<'NEWGRP_EOF'\nexec %s hub worker start\nNEWGRP_EOF", self))
						shCmd.Stdout = os.Stdout
						shCmd.Stderr = os.Stderr
						shCmd.Stdin = os.Stdin
						shCmd.Env = append(os.Environ(), "CQ_DOCKER_GROUP_FIXED=1")
						return shCmd.Run()
					}
				}
			} else {
				f.Close()
			}
		}
	}

	// Resolve Supabase URL from cloud config.
	supabaseURL, supabaseKey := resolveSupabaseConfig()
	if supabaseURL == "" {
		return fmt.Errorf("cloud not configured — run: cq auth login")
	}

	// Supabase auth: apikey=anon_key (always), Authorization=Bearer JWT.
	anonKey := supabaseKey
	jwt := loadCloudSessionJWT()
	if jwt == "" {
		return fmt.Errorf("no auth token — run: cq auth login")
	}
	// For backward compat, apiKey is used in claimAndRun — set to anonKey.
	apiKey := anonKey
	_ = apiKey // used in claimAndRun

	// Sync hub settings to .c4/config.yaml so MCP tools are available.
	if supabaseURL != "" {
		cfgYAMLPath := filepath.Join(projectDir, ".c4", "config.yaml")
		_ = cfghandler.UpdateYAMLValue(cfgYAMLPath, "hub.enabled", "true")
		_ = cfghandler.UpdateYAMLValue(cfgYAMLPath, "cloud.url", supabaseURL)
		fmt.Printf("cq: hub.enabled=true, cloud.url=%s\n", supabaseURL)
	}

	// Resolve direct URL for LISTEN/NOTIFY (port 5432).
	directURL := os.Getenv("C4_CLOUD_DIRECT_URL")
	if directURL == "" {
		// Try config.yaml cloud.direct_url
		home, _ := os.UserHomeDir()
		cfgYAMLPath := filepath.Join(home, ".c4", "config.yaml")
		if cfgData, readErr := os.ReadFile(cfgYAMLPath); readErr == nil {
			var cfgMap map[string]any
			if yaml.Unmarshal(cfgData, &cfgMap) == nil {
				if cloud, ok := cfgMap["cloud"].(map[string]any); ok {
					if u, ok := cloud["direct_url"].(string); ok {
						directURL = u
					}
				}
			}
		}
	}

	// Worker name from config or hostname.
	cfgPath := workerConfigPath()
	cfg := workerYAML{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}
	workerName := cfg.Name
	if workerName == "" {
		workerName, _ = os.Hostname()
	}

	fmt.Printf("cq: worker starting (name=%s, supabase=%s)\n", workerName, supabaseURL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register worker via Supabase PostgREST.
	registerURL := supabaseURL + "/rest/v1/rpc/register_worker"
	regBody, _ := json.Marshal(map[string]any{
		"p_worker_id":   workerName,
		"p_hostname":    workerName,
		"p_capabilities": []string{},
		"p_mcp_url":    "",
		"p_project_id": "",
	})
	regReq, _ := http.NewRequestWithContext(ctx, "POST", registerURL, strings.NewReader(string(regBody)))
	regReq.Header.Set("Content-Type", "application/json")
	regReq.Header.Set("apikey", anonKey)
	regReq.Header.Set("Authorization", "Bearer "+jwt)
	if resp, err := http.DefaultClient.Do(regReq); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("cq: worker registered (status=%d)\n", resp.StatusCode)
	} else {
		fmt.Fprintf(os.Stderr, "cq: warning: worker registration failed: %v\n", err)
	}

	// Poll loop: check for QUEUED jobs every 30 seconds.
	// If direct_url is set, also use LISTEN/NOTIFY for real-time.
	pollInterval := 30 * time.Second
	fmt.Printf("cq: polling for jobs every %s", pollInterval)
	if directURL != "" {
		fmt.Printf(" + LISTEN/NOTIFY on %s", directURL[:30]+"...")
	}
	fmt.Println()

	// Start LISTEN/NOTIFY in background if direct_url available.
	if directURL != "" {
		listener := hub.NewJobListener(directURL)
		go func() {
			err := listener.Listen(ctx, func(n hub.JobNotification) error {
				if n.Payload != "" {
					fmt.Printf("cq: NOTIFY received: job %s\n", n.Payload)
				}
				// Try to claim a job.
				claimAndRun(ctx, supabaseURL, anonKey, jwt, workerName)
				return nil
			})
			if err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "cq: LISTEN/NOTIFY stopped: %v\n", err)
			}
		}()
	}

	// Main poll loop.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Initial poll.
	claimAndRun(ctx, supabaseURL, anonKey, jwt, workerName)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("cq: worker stopped")
			return nil
		case <-ticker.C:
			claimAndRun(ctx, supabaseURL, anonKey, jwt, workerName)
		}
	}
}

// resolveSupabaseConfig reads Supabase URL and anon key from env vars or config.
func resolveSupabaseConfig() (url, key string) {
	url = os.Getenv("C4_CLOUD_URL")
	if url == "" {
		url = os.Getenv("SUPABASE_URL")
	}
	key = os.Getenv("C4_CLOUD_ANON_KEY")
	if key == "" {
		key = os.Getenv("SUPABASE_KEY")
	}
	if url == "" || key == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		cfgPath := filepath.Join(home, ".c4", "config.yaml")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return
		}
		var cfgMap map[string]any
		if yaml.Unmarshal(data, &cfgMap) != nil {
			return
		}
		if cloud, ok := cfgMap["cloud"].(map[string]any); ok {
			if url == "" {
				if u, ok := cloud["url"].(string); ok {
					url = u
				}
			}
			if key == "" {
				if k, ok := cloud["anon_key"].(string); ok {
					key = k
				}
			}
		}
	}
	return
}

// claimAndRun tries to claim a QUEUED job via Supabase RPC and execute it.
func claimAndRun(ctx context.Context, supabaseURL, anonKey, jwt, workerID string) {
	claimURL := supabaseURL + "/rest/v1/rpc/claim_job"
	body, _ := json.Marshal(map[string]any{
		"p_worker_id":   workerID,
		"p_capabilities": []string{},
		"p_project_id":  "",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", claimURL, strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: claim_job failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 || len(respBody) == 0 || string(respBody) == "null" {
		return // no job available
	}

	var result struct {
		Job     json.RawMessage `json:"job"`
		LeaseID string          `json:"lease_id"`
	}
	if json.Unmarshal(respBody, &result) != nil || result.LeaseID == "" {
		return // no job
	}

	var job struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	json.Unmarshal(result.Job, &job)

	fmt.Printf("cq: claimed job %s (%s) lease=%s\n", job.ID, job.Name, result.LeaseID)

	// Execute the job command, capturing output for log storage.
	if job.Command != "" {
		cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
		if job.Workdir != "" {
			if info, statErr := os.Stat(job.Workdir); statErr == nil && info.IsDir() {
				cmd.Dir = job.Workdir
			} else {
				fmt.Fprintf(os.Stderr, "cq: workdir %q not found, using current dir\n", job.Workdir)
			}
		}

		// Capture stdout+stderr while also printing to terminal.
		var outputBuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
		cmdErr := cmd.Run()

		// Complete the job.
		status := "COMPLETE"
		exitCode := 0
		if cmdErr != nil {
			status = "FAILED"
			var exitErr *exec.ExitError
			if errors.As(cmdErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "cq: job %s failed: %v\n", job.ID, cmdErr)
		} else {
			fmt.Printf("cq: job %s completed\n", job.ID)
		}

		// Store job log lines in Supabase (best-effort, max 500 lines).
		logLines := strings.Split(outputBuf.String(), "\n")
		if len(logLines) > 500 {
			logLines = logLines[len(logLines)-500:]
		}
		var logRows []map[string]any
		for _, line := range logLines {
			if line == "" {
				continue
			}
			logRows = append(logRows, map[string]any{
				"job_id": job.ID,
				"line":   line,
				"stream": "stdout",
			})
		}
		if len(logRows) > 0 {
			logURL := supabaseURL + "/rest/v1/hub_job_logs"
			logBody, _ := json.Marshal(logRows)
			logReq, _ := http.NewRequestWithContext(ctx, "POST", logURL, strings.NewReader(string(logBody)))
			logReq.Header.Set("Content-Type", "application/json")
			logReq.Header.Set("apikey", anonKey)
			logReq.Header.Set("Authorization", "Bearer "+jwt)
			if r, e := http.DefaultClient.Do(logReq); e == nil {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}
		}

		completeURL := supabaseURL + "/rest/v1/rpc/complete_job"
		completeBody, _ := json.Marshal(map[string]any{
			"p_job_id":    job.ID,
			"p_status":    status,
			"p_exit_code": exitCode,
			"p_worker_id": workerID,
		})
		completeReq, _ := http.NewRequestWithContext(ctx, "POST", completeURL, strings.NewReader(string(completeBody)))
		completeReq.Header.Set("Content-Type", "application/json")
		completeReq.Header.Set("apikey", anonKey)
		completeReq.Header.Set("Authorization", "Bearer "+jwt)
		if r, e := http.DefaultClient.Do(completeReq); e == nil {
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
		}
	}
}

// refreshCloudSession attempts to refresh the cloud session using the refresh_token.
// On success, session.json is updated with a new access_token.
func refreshCloudSession() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	sessionPath := filepath.Join(home, ".c4", "session.json")
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return fmt.Errorf("read session: %w", err)
	}

	var session struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("parse session: %w", err)
	}
	if session.RefreshToken == "" {
		return fmt.Errorf("no refresh_token in session")
	}

	// Read Supabase URL from config for token refresh endpoint.
	supabaseURL := os.Getenv("C4_CLOUD_URL")
	if supabaseURL == "" {
		supabaseURL = os.Getenv("SUPABASE_URL")
	}
	if supabaseURL == "" {
		// Try reading from config.yaml
		cfgPath := filepath.Join(home, ".c4", "config.yaml")
		if cfgData, readErr := os.ReadFile(cfgPath); readErr == nil {
			var cfgMap map[string]any
			if yaml.Unmarshal(cfgData, &cfgMap) == nil {
				if cloud, ok := cfgMap["cloud"].(map[string]any); ok {
					if u, ok := cloud["url"].(string); ok {
						supabaseURL = u
					}
				}
			}
		}
	}
	if supabaseURL == "" {
		return fmt.Errorf("supabase URL not found (set C4_CLOUD_URL or cloud.url in config)")
	}

	anonKey := os.Getenv("C4_CLOUD_ANON_KEY")
	if anonKey == "" {
		anonKey = os.Getenv("SUPABASE_KEY")
	}
	if anonKey == "" {
		cfgPath := filepath.Join(home, ".c4", "config.yaml")
		if cfgData, readErr := os.ReadFile(cfgPath); readErr == nil {
			var cfgMap map[string]any
			if yaml.Unmarshal(cfgData, &cfgMap) == nil {
				if cloud, ok := cfgMap["cloud"].(map[string]any); ok {
					if k, ok := cloud["anon_key"].(string); ok {
						anonKey = k
					}
				}
			}
		}
	}

	// Supabase GoTrue token refresh
	refreshURL := strings.TrimRight(supabaseURL, "/") + "/auth/v1/token?grant_type=refresh_token"
	body, _ := json.Marshal(map[string]string{"refresh_token": session.RefreshToken})

	req, err := http.NewRequest("POST", refreshURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var newTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &newTokens); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}

	// Update session.json — preserve existing fields (user info etc.)
	existingData, _ := os.ReadFile(sessionPath)
	updatedSession := map[string]any{}
	if len(existingData) > 0 {
		_ = json.Unmarshal(existingData, &updatedSession)
	}
	updatedSession["access_token"] = newTokens.AccessToken
	updatedSession["refresh_token"] = newTokens.RefreshToken
	updatedSession["expires_at"] = time.Now().Unix() + newTokens.ExpiresIn
	updatedData, _ := json.MarshalIndent(updatedSession, "", "  ")
	tmpPath := sessionPath + ".tmp"
	if err := os.WriteFile(tmpPath, updatedData, 0o600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	if err := os.Rename(tmpPath, sessionPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("install session: %w", err)
	}

	return nil
}

// loadCloudSessionJWT reads the cloud session JWT from ~/.c4/session.json.
// Returns the access_token if valid, or empty string if unavailable.
func loadCloudSessionJWT() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".c4", "session.json"))
	if err != nil {
		return ""
	}
	var session struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return ""
	}
	return session.AccessToken
}

// =========================================================================
// cq hub worker status
// =========================================================================

// workerAffinityRecord holds one project's affinity data for a worker.
type workerAffinityRecord struct {
	ProjectID    string `json:"project_id"`
	SuccessCount int    `json:"success_count"`
	FailCount    int    `json:"fail_count"`
}

// workerStatusRow holds the display fields for one worker row.
type workerStatusRow struct {
	ID           string                 `json:"id"`
	Hostname     string                 `json:"hostname"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	Tags         []string               `json:"tags"`
	UptimeSec    int64                  `json:"uptime_sec"`
	LastJobAt    string                 `json:"last_job_at"`
	Capabilities []string               `json:"capabilities"`
	Affinity     []workerAffinityRecord `json:"affinity"`
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
	fmt.Fprintf(w, "NAME\tSTATUS\tUPTIME\tLAST JOB\tCAPABILITIES\tAFFINITY\n")
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
		affStr := formatWorkerAffinity(wk.Affinity)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			name, wk.Status, formatUptime(wk.UptimeSec), formatLastJob(wk.LastJobAt), capsStr, affStr)
	}
	w.Flush()
	return nil
}

// formatWorkerAffinity formats affinity records as "project(N✓) ..." or "(none)".
func formatWorkerAffinity(recs []workerAffinityRecord) string {
	if len(recs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(recs))
	for _, r := range recs {
		parts = append(parts, fmt.Sprintf("%s(%d✓)", r.ProjectID, r.SuccessCount))
	}
	return strings.Join(parts, " ")
}
