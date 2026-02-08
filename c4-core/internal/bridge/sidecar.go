package bridge

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Sidecar manages the Python bridge sidecar process.
type Sidecar struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	addr    string // "host:port" once started
	stopped bool
}

// SidecarConfig holds configuration for the Python sidecar.
type SidecarConfig struct {
	// PythonCommand is the command to run Python (default: "uv").
	PythonCommand string
	// PythonArgs are the arguments for the sidecar entry point.
	PythonArgs []string
	// Host is the bind host for the sidecar (default: "localhost").
	Host string
	// Port is the requested port (0 = auto-assign).
	Port int
	// StartTimeout is how long to wait for the sidecar to be ready.
	StartTimeout time.Duration
}

// DefaultSidecarConfig returns sensible defaults.
func DefaultSidecarConfig() *SidecarConfig {
	return &SidecarConfig{
		PythonCommand: "uv",
		PythonArgs:    []string{"run", "c4-bridge"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  10 * time.Second,
	}
}

// StartSidecar spawns the Python bridge sidecar and waits for it to be ready.
// It reads the "C4_BRIDGE_PORT=<port>" line from stdout to discover the port.
func StartSidecar(cfg *SidecarConfig) (*Sidecar, error) {
	if cfg == nil {
		cfg = DefaultSidecarConfig()
	}

	pythonPath, err := exec.LookPath(cfg.PythonCommand)
	if err != nil {
		// Try fallback to python3
		pythonPath, err = exec.LookPath("python3")
		if err != nil {
			return nil, fmt.Errorf("python not found: neither %q nor python3 in PATH", cfg.PythonCommand)
		}
		cfg.PythonArgs = []string{"-m", "c4.bridge.sidecar"}
	}

	args := cfg.PythonArgs
	if cfg.Port > 0 {
		args = append(args, "--port", fmt.Sprintf("%d", cfg.Port))
	}

	cmd := exec.Command(pythonPath, args...)
	cmd.Stderr = os.Stderr

	// Capture stdout to read the port announcement
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting sidecar: %w", err)
	}

	s := &Sidecar{cmd: cmd}

	// Read port from stdout
	portCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "C4_BRIDGE_PORT=") {
				port := strings.TrimPrefix(line, "C4_BRIDGE_PORT=")
				portCh <- port
				return
			}
		}
		errCh <- fmt.Errorf("sidecar stdout closed without port announcement")
	}()

	select {
	case port := <-portCh:
		s.addr = net.JoinHostPort(cfg.Host, port)
	case err := <-errCh:
		_ = cmd.Process.Kill()
		return nil, err
	case <-time.After(cfg.StartTimeout):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("sidecar failed to start within %s", cfg.StartTimeout)
	}

	// Verify connectivity
	if err := s.waitReady(3 * time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("sidecar not reachable at %s: %w", s.addr, err)
	}

	return s, nil
}

// Addr returns the address of the running sidecar ("host:port").
func (s *Sidecar) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Stop gracefully shuts down the sidecar process.
func (s *Sidecar) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	s.stopped = true

	// Send SIGTERM for graceful shutdown
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		// Force kill if signal fails
		_ = s.cmd.Process.Kill()
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
		return nil
	}
}

// IsRunning returns true if the sidecar process is still alive.
func (s *Sidecar) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	// Check if process is still alive by sending signal 0
	return s.cmd.Process.Signal(nil) == nil
}

// waitReady polls the TCP address until connection succeeds or timeout.
func (s *Sidecar) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", s.addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", s.addr)
}
