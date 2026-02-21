package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/daemon"
	"github.com/spf13/cobra"
)

// isDaemonServeRunning checks if cq serve is managing the job scheduler.
// It reads ~/.c4/serve/serve.pid and verifies the process is alive via
// signal(0), then confirms via HTTP GET to localhost:4140/health.
func isDaemonServeRunning() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	pidPath := filepath.Join(home, ".c4", "serve", "serve.pid")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:4140/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

var (
	daemonPort    int
	daemonDataDir string
	daemonMaxJobs int
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the local job scheduler daemon",
	Long: `Start a local job scheduler that manages GPU/CPU job execution.

Provides a PiQ-compatible REST API for job submission, status tracking,
log streaming, and GPU monitoring.

The daemon runs until interrupted (Ctrl+C) or stopped via POST /daemon/stop.

Example:
  c4 daemon
  c4 daemon --port 7123
  c4 daemon --port 7123 --data-dir ~/.c4/daemon --max-jobs 4`,
	RunE: runDaemon,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running daemon",
	Long: `Send a stop request to a running c4 daemon.

Example:
  c4 daemon stop
  c4 daemon stop --port 7123`,
	RunE: runDaemonStop,
}

func init() {
	daemonCmd.Flags().IntVar(&daemonPort, "port", 7123, "HTTP server port")
	daemonCmd.Flags().StringVar(&daemonDataDir, "data-dir", "", "data directory (default: ~/.c4/daemon)")
	daemonCmd.Flags().IntVar(&daemonMaxJobs, "max-jobs", 0, "max concurrent CPU jobs (0 = unlimited)")

	daemonStopCmd.Flags().IntVar(&daemonPort, "port", 7123, "daemon port to stop")

	daemonCmd.AddCommand(daemonStopCmd)
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	// Deprecation warning: prefer cq serve when it is already running
	if isDaemonServeRunning() {
		fmt.Fprintln(os.Stderr, "WARNING: cq serve is running and manages the job scheduler.")
		fmt.Fprintln(os.Stderr, "         Use 'cq serve' instead of 'cq daemon'.")
		fmt.Fprintln(os.Stderr, "         'cq daemon' will be removed in a future release.")
	}

	// Resolve data directory
	dataDir := daemonDataDir
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".c4", "daemon")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// PID file lock
	pidPath := filepath.Join(dataDir, "daemon.pid")
	if err := acquirePIDLock(pidPath); err != nil {
		return err
	}
	defer os.Remove(pidPath)

	// Open store
	dbPath := filepath.Join(dataDir, "daemon.db")
	store, err := daemon.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// GPU monitor
	gpu := daemon.NewGpuMonitor()
	gpuCount := gpu.GPUCount()

	// Scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := daemon.NewScheduler(store, daemon.SchedulerConfig{
		DataDir:       dataDir,
		GPUCount:      gpuCount,
		MaxConcurrent: daemonMaxJobs,
	})
	sched.Start(ctx)
	defer sched.Stop()

	// HTTP server
	srv := daemon.NewServer(daemon.ServerConfig{
		Store:      store,
		Scheduler:  sched,
		GpuMonitor: gpu,
		Version:    version,
		CancelFunc: cancel,
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", daemonPort),
		Handler: srv.Handler(),
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nc4 daemon: shutting down (signal)...")
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "c4 daemon: shutting down (API stop)...")
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	// Start
	gpuStr := "no GPU"
	if gpuCount > 0 {
		gpuStr = fmt.Sprintf("%d GPU(s)", gpuCount)
	}
	fmt.Fprintf(os.Stderr, "c4 daemon: listening on :%d (%s, data: %s)\n", daemonPort, gpuStr, dataDir)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}

	fmt.Fprintln(os.Stderr, "c4 daemon: stopped")
	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	url := fmt.Sprintf("http://localhost:%d/daemon/stop", daemonPort)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("daemon not reachable on port %d: %w", daemonPort, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Daemon on port %d stopping...\n", daemonPort)
	} else {
		fmt.Printf("Unexpected response: %d\n", resp.StatusCode)
	}
	return nil
}

// acquirePIDLock writes the current PID to a file and checks for existing daemon.
func acquirePIDLock(pidPath string) error {
	// Check if a daemon is already running
	if data, err := os.ReadFile(pidPath); err == nil {
		pid, err := strconv.Atoi(string(data))
		if err == nil {
			// Check if process is alive
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("daemon already running (PID %d). Stop it with: c4 daemon stop", pid)
				}
			}
		}
		// Stale PID file — remove it
		os.Remove(pidPath)
	}

	// Write our PID
	return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}
