package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/serve"
	"github.com/spf13/cobra"
)

var (
	servePort   int
	servePIDDir string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run CQ as a long-running service with managed components",
	Long: `Start CQ as a foreground process that manages long-running components
(EventBus, EventSink, HubPoller, Agent, GPU).

Each component can be enabled/disabled via .c4/config.yaml under the serve: section.
The process exposes a health endpoint and manages graceful shutdown.

Writes a PID file to prevent duplicate instances.

Example:
  cq serve
  cq serve --port 4140
  cq serve --pid-dir ~/.c4/serve`,
	RunE: runServe,
}

var serveStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running cq serve process",
	Long: `Send SIGTERM to a running cq serve process using its PID file.

Example:
  cq serve stop
  cq serve stop --pid-dir ~/.c4/serve`,
	RunE: runServeStop,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 4140, "health endpoint port")
	serveCmd.Flags().StringVar(&servePIDDir, "pid-dir", "", "PID file directory (default: ~/.c4/serve)")

	serveStopCmd.Flags().StringVar(&servePIDDir, "pid-dir", "", "PID file directory (default: ~/.c4/serve)")

	serveCmd.AddCommand(serveStopCmd)
	rootCmd.AddCommand(serveCmd)
}

func resolveServePIDDir() (string, error) {
	if servePIDDir != "" {
		return servePIDDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "serve"), nil
}

func runServe(cmd *cobra.Command, args []string) error {
	pidDir, err := resolveServePIDDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}

	// PID file lock (reuse daemon pattern)
	pidPath := filepath.Join(pidDir, "serve.pid")
	if err := acquireServePIDLock(pidPath); err != nil {
		return err
	}
	defer os.Remove(pidPath)

	// Component manager
	mgr := serve.NewManager()

	// TODO: Register components based on config (future tasks)
	// e.g. if cfg.Serve.EventBus.Enabled { mgr.Register(eventbusComponent) }

	// Start all components
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.StartAll(ctx); err != nil {
		return fmt.Errorf("start components: %w", err)
	}

	// HTTP health server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", serve.HealthHandler(mgr))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", servePort),
		Handler: mux,
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\ncq serve: shutting down (signal: %s)...\n", sig)
		cancel()

		// Stop components
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()

		if err := mgr.StopAll(stopCtx); err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: component stop error: %v\n", err)
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "cq serve: health endpoint on :%d (%d components, pid: %d)\n",
		servePort, mgr.ComponentCount(), os.Getpid())

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("health server: %w", err)
	}

	fmt.Fprintln(os.Stderr, "cq serve: stopped")
	return nil
}

func runServeStop(cmd *cobra.Command, args []string) error {
	pidDir, err := resolveServePIDDir()
	if err != nil {
		return err
	}

	pidPath := filepath.Join(pidDir, "serve.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("no running cq serve found (PID file: %s): %w", pidPath, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid PID file content: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Process may already be dead; clean up stale PID file
		os.Remove(pidPath)
		return fmt.Errorf("failed to signal process %d (removed stale PID file): %w", pid, err)
	}

	fmt.Fprintf(os.Stderr, "cq serve: sent SIGTERM to PID %d\n", pid)
	return nil
}

// acquireServePIDLock writes the current PID to a file and checks for existing process.
func acquireServePIDLock(pidPath string) error {
	if data, err := os.ReadFile(pidPath); err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("cq serve already running (PID %d). Stop it with: cq serve stop", pid)
				}
			}
		}
		// Stale PID file - remove it
		os.Remove(pidPath)
	}

	return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}
