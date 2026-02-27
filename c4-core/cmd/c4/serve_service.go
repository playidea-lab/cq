package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

// newServiceConfig returns a service.Config for the cq-serve service.
func newServiceConfig(execPath, configPath string) service.Config {
	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	opt := service.KeyValue{}
	// macOS: user LaunchAgent (~/.Library/LaunchAgents/) — no sudo required.
	// Linux systemd: user unit (~/.config/systemd/user/) — no sudo required.
	// On unsupported Linux init systems (SysV, OpenRC), Install() will return an error.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		opt["UserService"] = true
	}
	return service.Config{
		Name:        "cq-serve",
		DisplayName: "CQ Serve",
		Description: "CQ long-running service (StaleChecker, EventBus, EventSink, HubPoller, Agent, GPU)",
		Executable:  execPath,
		Arguments:   args,
		Option:      opt,
	}
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
	// Resolve config path at install time (absolute) so OS service can find it
	// regardless of HOME or working directory differences.
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

// installServeService registers cq as an OS service.
// Returns nil if already installed (detected by string-matching the error message).
func installServeService(_ context.Context, _ bool) error {
	execPath, configPath, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	svcConfig := newServiceConfig(execPath, configPath)
	svc, err := service.New(newServiceWrapper(), &svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := svc.Install(); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "already installed") {
			fmt.Println("WARN: cq-serve service already installed, skipping.")
			return nil
		}
		return fmt.Errorf("install service: %w", err)
	}
	fmt.Println("cq-serve service installed.")
	if configPath != "" {
		fmt.Printf("Config: %s\n", configPath)
	}
	return nil
}

func runServeInstall(cmd *cobra.Command, args []string) error {
	return installServeService(cmd.Context(), false)
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
