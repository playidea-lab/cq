package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sidecar manages the Python bridge sidecar process.
type Sidecar struct {
	mu            sync.Mutex
	cfg           *SidecarConfig
	cmd           *exec.Cmd
	addr          string // "host:port" once started
	stopped       bool
	restarts      int
	lastRestartAt time.Time    // time of last restart, for counter reset
	healthStop    chan struct{} // channel to stop health check goroutine
	healthDone    chan struct{} // channel to signal health check goroutine exited
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
	// PidFile is the path to write the sidecar PID for orphan cleanup.
	// If set, StartSidecar will kill any stale process from a previous run.
	PidFile string
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

// cleanupStaleSidecar kills a leftover sidecar process from a previous run
// using the PID file and pgrep fallback. This handles the SIGKILL case where
// the Go parent dies without running defer/signal handlers.
func cleanupStaleSidecar(pidFile string) {
	// 1. PID file cleanup
	if pidFile != "" {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				if isProcessAlive(pid) {
					fmt.Fprintf(os.Stderr, "c4: killing stale sidecar (pgid %d) from previous session\n", pid)
					killStaleGroup(pid)
				}
			}
			_ = os.Remove(pidFile)
		}
	}

	// 2. pgrep fallback: find orphan c4-bridge processes (PPID=1)
	out, err := exec.Command("pgrep", "-f", "c4-bridge").Output()
	if err != nil || len(out) == 0 {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 0 {
			continue
		}
		// Check if this is an orphan (PPID=1) via /proc or ps
		ppidOut, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(strings.TrimSpace(string(ppidOut)))
		if err != nil || ppid != 1 {
			continue
		}
		fmt.Fprintf(os.Stderr, "c4: killing orphan c4-bridge process (pid %d, ppid=1)\n", pid)
		killOrphan(pid)
	}
}

// writePidFile writes the sidecar PID to disk for orphan cleanup on next startup.
func writePidFile(pidFile string, pid int) {
	if pidFile == "" {
		return
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		log.Printf("c4: sidecar: failed to write PID file %s: %v", pidFile, err)
	}
}

// StartSidecar spawns the Python bridge sidecar and waits for it to be ready.
// It reads the "C4_BRIDGE_PORT=<port>" line from stdout to discover the port.
func StartSidecar(cfg *SidecarConfig) (*Sidecar, error) {
	if cfg == nil {
		cfg = DefaultSidecarConfig()
	}

	// Kill any orphan sidecar from a previous session (SIGKILL recovery)
	cleanupStaleSidecar(cfg.PidFile)

	pythonPath, err := exec.LookPath(cfg.PythonCommand)
	if err != nil {
		// Try PYTHON env, then python3 as final fallback
		if envPython := os.Getenv("PYTHON"); envPython != "" {
			pythonPath, err = exec.LookPath(envPython)
		}
		if err != nil {
			pythonPath, err = exec.LookPath("python3")
		}
		if err != nil {
			return nil, fmt.Errorf("python not found: neither %q, $PYTHON, nor python3 in PATH", cfg.PythonCommand)
		}
		cfg.PythonArgs = []string{"-m", "c4.bridge.sidecar"}
	}

	args := cfg.PythonArgs
	// Always pass --port so Python uses OS-assigned port when Port=0
	// instead of falling back to its hard-coded default (50051).
	args = append(args, "--port", fmt.Sprintf("%d", cfg.Port))

	cmd := exec.Command(pythonPath, args...)
	cmd.Stderr = os.Stderr
	// Pass Go parent PID so the sidecar can self-terminate when orphaned.
	// The sidecar monitors this PID and exits if it dies (SIGKILL recovery).
	cmd.Env = append(os.Environ(), fmt.Sprintf("C4_PARENT_PID=%d", os.Getpid()))
	// Put sidecar in its own process group so Stop() can kill
	// the entire tree (uv wrapper + python child) with a single signal.
	setSysProcAttr(cmd)

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

	// Record PID for orphan cleanup on next startup (SIGKILL recovery)
	writePidFile(cfg.PidFile, cmd.Process.Pid)

	return s, nil
}

// Addr returns the address of the running sidecar ("host:port").
func (s *Sidecar) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Stop gracefully shuts down the sidecar process group.
// Sends SIGTERM to the entire process group (uv + python) first,
// then SIGKILL if it doesn't exit within 5 seconds.
func (s *Sidecar) Stop() error {
	// Stop health check goroutine first (before acquiring lock)
	s.StopHealthCheck()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	s.stopped = true

	pgid := s.cmd.Process.Pid

	// Clean up PID file
	if s.cfg != nil {
		_ = os.Remove(s.cfg.PidFile)
	}

	// Send SIGTERM to the entire process group (negative PID = process group).
	// On Windows killGroup is a no-op; we fall back to Process.Kill directly.
	if !killGroup(pgid, false) {
		_ = s.cmd.Process.Kill()
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		// Force kill entire process group (or individual process on Windows)
		if !killGroup(pgid, true) {
			_ = s.cmd.Process.Kill()
		}
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

// StartHealthCheck starts a goroutine that periodically pings the sidecar.
// If Ping fails, it attempts Restart. On success, onRestart callback is called with new address.
// Implements exponential backoff: 30s → 60s → 120s → max 5min on consecutive failures.
// Successful Ping resets backoff. Call StopHealthCheck to terminate the goroutine.
func (s *Sidecar) StartHealthCheck(interval time.Duration, onRestart func(string)) {
	s.mu.Lock()
	// Don't start if already running
	if s.healthStop != nil {
		s.mu.Unlock()
		return
	}
	s.healthStop = make(chan struct{})
	s.healthDone = make(chan struct{})
	s.mu.Unlock()

	go func() {
		// Capture stop channel to local var at goroutine start to avoid race with StopHealthCheck
		s.mu.Lock()
		stopChan := s.healthStop
		doneChan := s.healthDone
		s.mu.Unlock()

		defer close(doneChan)
		currentInterval := interval
		maxInterval := 5 * time.Minute

		timer := time.NewTimer(currentInterval)
		defer timer.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-timer.C:
				// Skip if stopped
				s.mu.Lock()
				stopped := s.stopped
				s.mu.Unlock()
				if stopped {
					timer.Reset(currentInterval)
					continue
				}

				if err := s.Ping(); err != nil {
					fmt.Fprintf(os.Stderr, "c4: sidecar health check failed: %v\n", err)
					// Try restart
					newAddr, restartErr := s.Restart()
					if restartErr != nil {
						fmt.Fprintf(os.Stderr, "c4: sidecar restart failed: %v\n", restartErr)
						// Exponential backoff
						currentInterval = min(currentInterval*2, maxInterval)
					} else {
						fmt.Fprintf(os.Stderr, "c4: sidecar restarted via health check at %s\n", newAddr)
						currentInterval = interval // reset backoff
						if onRestart != nil {
							onRestart(newAddr)
						}
					}
				} else {
					// Healthy — reset backoff
					currentInterval = interval
				}
				timer.Reset(currentInterval)
			}
		}
	}()
}

// StopHealthCheck stops the health check goroutine if running.
func (s *Sidecar) StopHealthCheck() {
	s.mu.Lock()
	stop := s.healthStop
	done := s.healthDone
	s.healthStop = nil
	s.mu.Unlock()

	if stop != nil {
		close(stop)
		<-done // wait for goroutine to exit
	}
}

// Restart stops the current sidecar and starts a new one.
// Returns the new address. Max 5 restarts to avoid infinite loops.
// The restart counter resets after 10 minutes of stability.
func (s *Sidecar) Restart() (string, error) {
	s.mu.Lock()
	// Time-based counter reset: if last restart was >10min ago, reset counter
	if !s.lastRestartAt.IsZero() && time.Since(s.lastRestartAt) > 10*time.Minute {
		s.restarts = 0
	}
	if s.restarts >= 5 {
		s.mu.Unlock()
		return "", fmt.Errorf("sidecar restart limit reached (%d)", s.restarts)
	}
	s.restarts++
	s.lastRestartAt = time.Now()
	cfg := s.cfg

	// Stop existing process group while holding lock to prevent concurrent access
	if !s.stopped && s.cmd != nil && s.cmd.Process != nil {
		pgid := s.cmd.Process.Pid
		proc := s.cmd.Process
		if !killGroup(pgid, false) {
			_ = proc.Kill()
		}
		s.stopped = true
		// Don't wait for process exit under lock — it's best-effort
		go func(cmd *exec.Cmd, pgid int) {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				if !killGroup(pgid, true) {
					_ = cmd.Process.Kill()
				}
			}
		}(s.cmd, pgid)
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
