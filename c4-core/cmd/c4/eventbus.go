package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/changmin/c4-core/internal/eventbus"
	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var (
	eventbusSocket  string
	eventbusDataDir string
)

var eventbusCmd = &cobra.Command{
	Use:   "eventbus",
	Short: "Run the C3 EventBus gRPC daemon",
	Long: `Start the C3 EventBus daemon for event-driven pipelines between C4 components.

Provides a gRPC API over Unix Domain Socket for publishing events,
subscribing to event streams, and managing event routing rules.

The daemon runs until interrupted (Ctrl+C) or stopped via 'c4 eventbus stop'.

Example:
  c4 eventbus
  c4 eventbus --socket ~/.c4/eventbus/c3.sock
  c4 eventbus --data-dir ~/.c4/eventbus`,
	RunE: runEventbus,
}

var eventbusStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running eventbus daemon",
	Long: `Send a stop request to a running C3 EventBus daemon.

Example:
  c4 eventbus stop`,
	RunE: runEventbusStop,
}

func init() {
	eventbusCmd.Flags().StringVar(&eventbusSocket, "socket", "", "Unix socket path (default: ~/.c4/eventbus/c3.sock)")
	eventbusCmd.Flags().StringVar(&eventbusDataDir, "data-dir", "", "data directory (default: ~/.c4/eventbus)")

	eventbusCmd.AddCommand(eventbusStopCmd)
	rootCmd.AddCommand(eventbusCmd)
}

func resolveEventbusDataDir() (string, error) {
	if eventbusDataDir != "" {
		return eventbusDataDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "eventbus"), nil
}

func resolveSocketPath(dataDir string) string {
	if eventbusSocket != "" {
		return eventbusSocket
	}
	return filepath.Join(dataDir, "c3.sock")
}

func runEventbus(cmd *cobra.Command, args []string) error {
	dataDir, err := resolveEventbusDataDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	sockPath := resolveSocketPath(dataDir)

	// PID file lock
	pidPath := filepath.Join(dataDir, "eventbus.pid")
	if err := acquireEventbusPIDLock(pidPath); err != nil {
		return err
	}
	defer os.Remove(pidPath)

	// Remove stale socket file before listening
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	// Open store
	dbPath := filepath.Join(dataDir, "events.db")
	store, err := eventbus.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Dispatcher
	dispatcher := eventbus.NewDispatcher(store)

	// gRPC server
	srv := eventbus.NewServer(eventbus.ServerConfig{
		Store:      store,
		Dispatcher: dispatcher,
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", sockPath, err)
	}
	defer os.Remove(sockPath)

	grpcServer := grpc.NewServer()
	pb.RegisterEventBusServer(grpcServer, srv)

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nc4 eventbus: shutting down (signal)...")
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "c4 eventbus: shutting down...")
		}
		grpcServer.GracefulStop()
	}()

	fmt.Fprintf(os.Stderr, "c4 eventbus: listening on unix:%s (data: %s)\n", sockPath, dataDir)

	if err := grpcServer.Serve(ln); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	fmt.Fprintln(os.Stderr, "c4 eventbus: stopped")
	return nil
}

func runEventbusStop(cmd *cobra.Command, args []string) error {
	dataDir, err := resolveEventbusDataDir()
	if err != nil {
		return err
	}

	pidPath := filepath.Join(dataDir, "eventbus.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("eventbus not running (no PID file at %s)", pidPath)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to signal process %d: %w", pid, err)
	}

	fmt.Printf("c4 eventbus: sent SIGTERM to PID %d\n", pid)
	return nil
}

// acquireEventbusPIDLock writes the current PID to a file and checks for existing daemon.
func acquireEventbusPIDLock(pidPath string) error {
	if data, err := os.ReadFile(pidPath); err == nil {
		pid, err := strconv.Atoi(string(data))
		if err == nil {
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("eventbus already running (PID %d). Stop it with: c4 eventbus stop", pid)
				}
			}
		}
		os.Remove(pidPath)
	}

	return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}
