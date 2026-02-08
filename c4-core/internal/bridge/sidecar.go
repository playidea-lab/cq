package bridge

import (
	"bufio"
	"encoding/json"
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
	cfg     *SidecarConfig
	cmd     *exec.Cmd
	addr    string // "host:port" once started
	stopped bool
	restarts int
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

	s := &Sidecar{cfg: cfg, cmd: cmd}

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

// Ping sends a JSON-RPC Ping request to verify the sidecar is responsive.
func (s *Sidecar) Ping() error {
	s.mu.Lock()
	addr := s.addr
	s.mu.Unlock()

	if addr == "" {
		return fmt.Errorf("sidecar not running")
	}

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("ping dial: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	req := map[string]any{"method": "Ping", "params": map[string]any{}}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("ping write: %w", err)
	}

	// Read response line using Scanner (same pattern as doCall's loop-until-newline)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("ping read: %w", err)
		}
		return fmt.Errorf("ping read: connection closed")
	}

	var resp struct {
		Result map[string]any `json:"result"`
		Error  *string        `json:"error"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("ping parse: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("ping error: %s", *resp.Error)
	}

	// Successful ping resets restart counter — sidecar is healthy
	s.mu.Lock()
	s.restarts = 0
	s.mu.Unlock()

	return nil
}

// Restart stops the current sidecar and starts a new one.
// Returns the new address. Max 3 restarts to avoid infinite loops.
func (s *Sidecar) Restart() (string, error) {
	s.mu.Lock()
	if s.restarts >= 3 {
		s.mu.Unlock()
		return "", fmt.Errorf("sidecar restart limit reached (%d)", s.restarts)
	}
	s.restarts++
	cfg := s.cfg

	// Stop existing process while holding lock to prevent concurrent access
	if !s.stopped && s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(os.Interrupt)
		s.stopped = true
		// Don't wait for process exit under lock — it's best-effort
		go func(cmd *exec.Cmd) {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		}(s.cmd)
	}

	// Reset state for new process
	s.cmd = nil
	s.addr = ""
	s.stopped = false
	s.mu.Unlock()

	// Start new sidecar (blocking, outside lock)
	newSidecar, err := StartSidecar(cfg)
	if err != nil {
		return "", fmt.Errorf("restart failed: %w", err)
	}

	// Transfer ownership atomically
	s.mu.Lock()
	s.cmd = newSidecar.cmd
	s.addr = newSidecar.addr
	s.stopped = false
	addr := s.addr
	restarts := s.restarts
	s.mu.Unlock()

	// Release temporary sidecar to prevent double-close
	newSidecar.mu.Lock()
	newSidecar.cmd = nil
	newSidecar.stopped = true
	newSidecar.mu.Unlock()

	fmt.Fprintf(os.Stderr, "c4: sidecar restarted at %s (restart #%d)\n", addr, restarts)
	return addr, nil
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
