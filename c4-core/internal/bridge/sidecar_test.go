package bridge

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

// TestPingFormat verifies the Ping JSON-RPC wire format.
func TestPingFormat(t *testing.T) {
	// Start a minimal JSON-RPC server that responds to Ping
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	// Serve one request
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		var req map[string]any
		if err := json.Unmarshal(buf[:n-1], &req); err != nil { // strip trailing \n
			return
		}

		resp := map[string]any{
			"result": map[string]any{"status": "ok"},
			"error":  nil,
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		conn.Write(data)
	}()

	// Create a sidecar with the mock addr
	s := &Sidecar{addr: addr}

	err = s.Ping()
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestPingFailsWhenNotRunning verifies Ping fails with no address.
func TestPingFailsWhenNotRunning(t *testing.T) {
	s := &Sidecar{addr: ""}
	err := s.Ping()
	if err == nil {
		t.Fatal("expected error for empty addr")
	}
}

// TestPingFailsWhenUnreachable verifies Ping fails with dead address.
func TestPingFailsWhenUnreachable(t *testing.T) {
	s := &Sidecar{addr: "127.0.0.1:1"} // port 1 is almost certainly closed
	err := s.Ping()
	if err == nil {
		t.Fatal("expected error for unreachable addr")
	}
}

// TestIsRunningOnNilCmd verifies IsRunning handles nil cmd gracefully.
func TestIsRunningOnNilCmd(t *testing.T) {
	s := &Sidecar{}
	if s.IsRunning() {
		t.Fatal("expected false for nil cmd")
	}
}

// TestRestartLimitReached verifies Restart enforces max attempts.
func TestRestartLimitReached(t *testing.T) {
	cfg := DefaultSidecarConfig()
	s := &Sidecar{
		cfg:      cfg,
		restarts: 5, // already at limit
	}
	_, err := s.Restart()
	if err == nil {
		t.Fatal("expected restart limit error")
	}
}

// TestPingResetsRestartCounter verifies that a successful Ping resets the restart counter.
func TestPingResetsRestartCounter(t *testing.T) {
	// Start a mock sidecar
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		if n == 0 {
			return
		}
		resp := `{"result":{"status":"ok"},"error":null}` + "\n"
		conn.Write([]byte(resp))
	}()

	s := &Sidecar{
		addr:     ln.Addr().String(),
		restarts: 2, // has been restarted twice
	}

	if err := s.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	s.mu.Lock()
	count := s.restarts
	s.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected restarts reset to 0, got %d", count)
	}
}

// TestRestartSuccessPath verifies Restart with a mock sidecar that binds to a port.
func TestRestartSuccessPath(t *testing.T) {
	// We can't easily test full StartSidecar without python, so we verify
	// the mechanics: stop is called, restarts counter increments, limit works.
	cfg := DefaultSidecarConfig()
	s := &Sidecar{
		cfg:      cfg,
		restarts: 0,
		stopped:  true, // simulate already stopped
	}

	// Restart will call StartSidecar which needs python — this will fail
	// but we verify the counter incremented and Stop was called
	_, err := s.Restart()
	if err == nil {
		// If it somehow succeeded (python exists), that's fine too
		t.Log("Restart succeeded (python available)")
		defer s.Stop() // Clean up the running sidecar to avoid I/O leak
	}

	s.mu.Lock()
	count := s.restarts
	s.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected restarts=1, got %d", count)
	}
}

// TestWaitReadyTimeout verifies waitReady times out on closed port.
func TestWaitReadyTimeout(t *testing.T) {
	s := &Sidecar{addr: "127.0.0.1:1"}
	start := time.Now()
	err := s.waitReady(500 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("returned too fast: %v", elapsed)
	}
}

// TestSidecarStopIdempotent verifies Stop can be called multiple times.
func TestSidecarStopIdempotent(t *testing.T) {
	s := &Sidecar{stopped: true}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	// Second stop should also be fine
	if err := s.Stop(); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

// TestAddrReturnsValue verifies Addr returns the stored address.
func TestAddrReturnsValue(t *testing.T) {
	s := &Sidecar{addr: "127.0.0.1:12345"}
	if s.Addr() != "127.0.0.1:12345" {
		t.Fatalf("expected 127.0.0.1:12345, got %s", s.Addr())
	}
}

// TestStartSidecarMissingPython verifies StartSidecar fails when python not found.
func TestStartSidecarMissingPython(t *testing.T) {
	// Set PATH to empty so neither uv nor python3 can be found
	t.Setenv("PATH", "/nonexistent")

	cfg := &SidecarConfig{
		PythonCommand: "nonexistent-python-binary-xyz",
		PythonArgs:    []string{"run", "c4-bridge"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  2 * time.Second,
	}
	_, err := StartSidecar(cfg)
	if err == nil {
		t.Fatal("expected error when python is not in PATH")
	}
	if !strings.Contains(err.Error(), "python not found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestHealthCheckStartStop verifies the health check goroutine can be started and stopped cleanly.
func TestHealthCheckStartStop(t *testing.T) {
	// Create a mock sidecar server that responds to Ping
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Handle Ping requests
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				resp := `{"result":{"status":"ok"},"error":null}` + "\n"
				c.Write([]byte(resp))
			}(conn)
		}
	}()

	s := &Sidecar{addr: ln.Addr().String()}

	// Start health check with short interval
	s.StartHealthCheck(100*time.Millisecond, nil)

	// Let it run for a bit
	time.Sleep(250 * time.Millisecond)

	// Stop health check
	s.StopHealthCheck()

	// Verify channels are cleaned up
	s.mu.Lock()
	if s.healthStop != nil {
		t.Fatal("healthStop should be nil after stop")
	}
	s.mu.Unlock()

	// Second stop should be safe
	s.StopHealthCheck()
}

// TestHealthCheckRestartsOnFailure verifies the health check attempts restart on Ping failure.
func TestHealthCheckRestartsOnFailure(t *testing.T) {
	// Create a sidecar with an unreachable address
	s := &Sidecar{
		addr:     "127.0.0.1:1", // port 1 is almost certainly closed
		cfg:      DefaultSidecarConfig(),
		stopped:  false,
		restarts: 0,
	}

	// Start health check with very short interval
	s.StartHealthCheck(50*time.Millisecond, nil)

	// Wait for at least one health check cycle
	time.Sleep(150 * time.Millisecond)

	// Verify restart was attempted (counter should have incremented)
	s.mu.Lock()
	restarts := s.restarts
	s.mu.Unlock()

	// Stop the sidecar first (this will stop any running process)
	_ = s.Stop() // Stop calls StopHealthCheck internally

	if restarts < 1 {
		t.Fatalf("expected at least 1 restart attempt, got %d", restarts)
	}

	// Note: The health check successfully restarted the sidecar (created new process)
	// s.Stop() cleans up both the health check goroutine and the sidecar process
}
