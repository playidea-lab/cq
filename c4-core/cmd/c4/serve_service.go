package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/serve"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

// serviceWrapper implements service.Interface for kardianos/service.
type serviceWrapper struct{}

func newServiceWrapper() service.Interface {
	return &serviceWrapper{}
}

func (s *serviceWrapper) Start(svc service.Service) error {
	// The OS service manager invokes `cq serve` directly as the process.
	return nil
}

func (s *serviceWrapper) Stop(svc service.Service) error {
	// Graceful stop is handled by OS SIGTERM to the process.
	return nil
}

// runCommand executes a command and returns combined stdout+stderr output.
func runCommand(name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// newServiceConfig returns a service.Config for the cq-serve service.
func newServiceConfig(execPath, configPath string) service.Config {
	// Use --watchdog so the OS service manager supervises a watchdog process that
	// automatically restarts the inner cq serve on crash (in addition to the OS-level restart).
	args := []string{"serve", "--watchdog"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}

	// Resolve working directory for --dir.
	// Prefer home directory (global serve) to avoid binding to a single project.
	homeDir, _ := os.UserHomeDir()
	dir := homeDir
	if dir == "" {
		dir = projectDir
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if dir != "" {
		args = append(args, "--dir", dir)
	}

	opt := service.KeyValue{}
	// macOS: user LaunchAgent (~/.Library/LaunchAgents/) — no sudo required.
	// Linux systemd: user unit (~/.config/systemd/user/) — no sudo required.
	// On unsupported Linux init systems (SysV, OpenRC), Install() will return an error.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		opt["UserService"] = true
	}
	// macOS: KeepAlive restarts the service on crash.
	if runtime.GOOS == "darwin" {
		opt["KeepAlive"] = true
		opt["RunAtLoad"] = true
	}
	// Linux systemd: restart on crash, enable at boot via WantedBy=default.target.
	if runtime.GOOS == "linux" {
		opt["Restart"] = "always"
		opt["RestartSec"] = "5"
		opt["SuccessExitStatus"] = "143"
	}

	// Log directory: ~/Library/Logs/ (macOS) or ~/.local/state/cq/ (Linux).
	home, _ := os.UserHomeDir()
	var logDir string
	if runtime.GOOS == "darwin" && home != "" {
		logDir = filepath.Join(home, "Library", "Logs")
	} else if home != "" {
		logDir = filepath.Join(home, ".local", "state", "cq")
	}
	if logDir != "" {
		os.MkdirAll(logDir, 0755)
		opt["LogDirectory"] = logDir
	}

	cfg := service.Config{
		Name:        "cq-serve",
		DisplayName: "CQ Serve",
		Description: "CQ long-running service (StaleChecker, EventBus, EventSink, HubPoller, Agent, GPU)",
		Executable:  execPath,
		Arguments:   args,
		Option:      opt,
	}
	if dir != "" {
		cfg.WorkingDirectory = dir
	}
	return cfg
}

// resolveInstallPaths returns the executable path and optional config path.
// Resolves symlinks so the service manager uses the real binary.
// Resolves config from projectDir or CWD for OS service HOME compatibility.
func resolveInstallPaths() (execPath, configPath string, err error) {
	execPath, err = os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable: %w", err)
	}
	// Resolve symlinks so the service manager uses the real binary.
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", "", fmt.Errorf("eval symlinks: %w", err)
	}
	// Resolve config path: prefer global (~/.c4/config.yaml) over project-specific.
	// Global config ensures cq serve works regardless of which project started it.
	home, _ := os.UserHomeDir()
	if home != "" {
		globalConfig := filepath.Join(home, ".c4", "config.yaml")
		if _, statErr := os.Stat(globalConfig); statErr == nil {
			configPath = globalConfig
			return execPath, configPath, nil
		}
	}
	// Fallback: project-specific config.
	dir := projectDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	candidate := filepath.Join(dir, ".c4", "config.yaml")
	if _, statErr := os.Stat(candidate); statErr == nil {
		configPath = candidate
	}
	return execPath, configPath, nil
}

var serveInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install cq serve as an OS service (macOS LaunchAgent / Linux systemd / Windows Service)",
	RunE:  runServeInstall,
}

var serveUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the cq serve OS service",
	RunE:  runServeUninstall,
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show OS service status and manual serve process status",
	RunE:  runServeStatus,
}

// installServeService registers cq as an OS service and optionally starts it.
// Returns nil if already installed (detected by string-matching the error message).
func installServeService(_ context.Context, start bool) error {
	execPath, configPath, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	svcConfig := newServiceConfig(execPath, configPath)
	svc, err := service.New(newServiceWrapper(), &svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	alreadyInstalled := false
	if err := svc.Install(); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "already installed") {
			fmt.Println("cq-serve: already installed, updating...")
			// Uninstall + reinstall to pick up new config.
			svc.Stop()
			svc.Uninstall()
			if err := svc.Install(); err != nil {
				return fmt.Errorf("reinstall service: %w", err)
			}
			alreadyInstalled = true
		} else {
			return fmt.Errorf("install service: %w", err)
		}
	}

	if !alreadyInstalled {
		fmt.Println("cq-serve: service installed.")
	} else {
		fmt.Println("cq-serve: service reinstalled.")
	}
	fmt.Printf("  Executable: %s\n", execPath)
	fmt.Printf("  Arguments:  %s\n", strings.Join(svcConfig.Arguments, " "))
	if svcConfig.WorkingDirectory != "" {
		fmt.Printf("  WorkDir:    %s\n", svcConfig.WorkingDirectory)
	}
	if configPath != "" {
		fmt.Printf("  Config:     %s\n", configPath)
	}
	if logDir, ok := svcConfig.Option["LogDirectory"]; ok {
		fmt.Printf("  Logs:       %s/cq-serve.{out,err}.log\n", logDir)
	}

	// Linux: enable the user unit so it starts at boot (WantedBy=default.target).
	if runtime.GOOS == "linux" {
		if out, enableErr := runCommand("systemctl", "--user", "enable", "cq-serve"); enableErr != nil {
			fmt.Fprintf(os.Stderr, "cq-serve: systemctl enable failed: %v\n%s\n", enableErr, out)
		} else {
			fmt.Println("cq-serve: enabled for auto-start at login (systemctl --user enable).")
		}
		// Enable linger so the user service survives SSH disconnection and runs at boot.
		// Try sudo first (works on NOPASSWD servers), fall back to non-sudo, then just advise.
		user := os.Getenv("USER")
		if user == "" {
			user = "$(whoami)"
		}
		if _, err := runCommand("loginctl", "show-user", user); err == nil {
			// Check current linger status
			out, _ := runCommand("loginctl", "show-user", user, "--property=Linger")
			if !strings.Contains(out, "Linger=yes") {
				// Try enabling — sudo first, then without
				if _, err := runCommand("sudo", "-n", "loginctl", "enable-linger", user); err != nil {
					if _, err := runCommand("loginctl", "enable-linger", user); err != nil {
						fmt.Fprintf(os.Stderr, "cq-serve: ⚠️  linger not enabled (service may stop when SSH disconnects)\n")
						fmt.Fprintf(os.Stderr, "  Run manually: sudo loginctl enable-linger %s\n", user)
					} else {
						fmt.Printf("cq-serve: linger enabled for %s (survives SSH disconnect).\n", user)
					}
				} else {
					fmt.Printf("cq-serve: linger enabled for %s (survives SSH disconnect).\n", user)
				}
			}
		}

		// WSL2: additionally register Windows Task Scheduler task for boot-time auto-start.
		if isWSL2() {
			fmt.Println("cq-serve: WSL2 detected — registering Windows Task Scheduler task...")
			if err := registerWindowsTask(execPath, configPath, svcConfig.WorkingDirectory); err != nil {
				fmt.Fprintf(os.Stderr, "cq-serve: Windows Task Scheduler registration failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "  (WSL systemd service is still installed; Windows auto-start skipped)")
			} else {
				fmt.Printf("cq-serve: Windows Task '%s' registered (starts on Windows boot).\n", wslTaskName)
			}
			// Check wsl.conf for systemd=true
			if !checkWslConf() {
				fmt.Fprintln(os.Stderr, "  ⚠️  /etc/wsl.conf does not have [boot] systemd=true")
				fmt.Fprintln(os.Stderr, "  Add the following to /etc/wsl.conf for systemd services:")
				fmt.Fprintln(os.Stderr, "    [boot]")
				fmt.Fprintln(os.Stderr, "    systemd=true")
			}
		}
	}

	if start {
		if err := svc.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "cq-serve: start failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "  Try: cq serve status")
			return nil
		}
		fmt.Println("cq-serve: started.")
	}

	return nil
}

var serveInstallStart bool

func init() {
	serveInstallCmd.Flags().BoolVar(&serveInstallStart, "start", true, "start the service after install")
}

func runServeInstall(cmd *cobra.Command, args []string) error {
	return installServeService(cmd.Context(), serveInstallStart)
}

// stopOSService stops the cq-serve OS service via the service manager.
func stopOSService() error {
	execPath, configPath, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	svcConfig := newServiceConfig(execPath, configPath)
	svc, err := service.New(newServiceWrapper(), &svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	return svc.Stop()
}

func runServeUninstall(cmd *cobra.Command, args []string) error {
	execPath, configPath, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	svcConfig := newServiceConfig(execPath, configPath)
	svc, err := service.New(newServiceWrapper(), &svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	fmt.Println("cq-serve service uninstalled.")

	// WSL2: also remove Windows Task Scheduler task.
	if runtime.GOOS == "linux" && isWSL2() {
		if err := unregisterWindowsTask(); err != nil {
			fmt.Fprintf(os.Stderr, "cq-serve: Windows Task removal failed: %v\n", err)
		} else {
			fmt.Printf("cq-serve: Windows Task '%s' removed.\n", wslTaskName)
		}
	}
	return nil
}

// fetchServeHealth calls the /health endpoint on the given port and returns
// the per-component health map. Returns an error if the server is unreachable.
func fetchServeHealth(port int) (map[string]serve.ComponentHealth, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var hr serve.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		return nil, fmt.Errorf("decode health response: %w", err)
	}
	return hr.Components, nil
}

func runServeStatus(cmd *cobra.Command, args []string) error {
	execPath, configPath, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	svcConfig := newServiceConfig(execPath, configPath)
	svc, err := service.New(newServiceWrapper(), &svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	status, err := svc.Status()
	switch status {
	case service.StatusRunning:
		fmt.Println("service: installed (running)")
	case service.StatusStopped:
		fmt.Println("service: installed (stopped)")
	default:
		if err != nil {
			fmt.Printf("service: not installed (%v)\n", err)
		} else {
			fmt.Println("service: not installed")
		}
	}

	// Check for manual serve process via PID file, with liveness verification.
	pidDir, pidDirErr := resolveServePIDDir()
	if pidDirErr != nil {
		return nil
	}
	pidPath := filepath.Join(pidDir, "serve.pid")
	if data, readErr := os.ReadFile(pidPath); readErr == nil {
		pid := strings.TrimSpace(string(data))
		if pidInt, parseErr := strconv.Atoi(pid); parseErr == nil {
			if proc, findErr := os.FindProcess(pidInt); findErr == nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					fmt.Printf("manual: running (pid=%s)\n", pid)
					// Fetch component health from /health endpoint.
					components, healthErr := fetchServeHealth(servePort)
					if healthErr != nil {
						fmt.Printf("  (serve not responding on port %d)\n", servePort)
					} else {
						for name, h := range components {
							if h.Status == "ok" {
								fmt.Printf("  \u2713 %-12s %s\n", name, h.Status)
							} else {
								detail := h.Detail
								if detail != "" {
									fmt.Printf("  \u2717 %-12s %s (%s)\n", name, h.Status, detail)
								} else {
									fmt.Printf("  \u2717 %-12s %s\n", name, h.Status)
								}
							}
						}
					}
				} else {
					fmt.Println("manual: stale PID file (process not running)")
					if removeErr := os.Remove(pidPath); removeErr != nil {
						fmt.Fprintf(os.Stderr, "cq: warning: remove stale pid: %v\n", removeErr)
					}
				}
			}
		}
	}

	return nil
}
