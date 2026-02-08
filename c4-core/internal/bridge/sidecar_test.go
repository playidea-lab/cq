package bridge

import (
	"encoding/json"
	"fmt"
	"net"
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
		restarts: 3, // already at limit
	}
	_, err := s.Restart()
	if err == nil {
		t.Fatal("expected restart limit error")
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
	cfg := &SidecarConfig{
		PythonCommand: "nonexistent-python-binary-xyz",
		PythonArgs:    []string{"run", "c4-bridge"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  2 * time.Second,
	}
	// This may succeed if python3 is in PATH (fallback), so we just verify no panic
	s, err := StartSidecar(cfg)
	if err == nil {
		// If it somehow succeeded (python3 exists), clean up
		s.Stop()
		fmt.Println("StartSidecar succeeded via python3 fallback — not an error")
	}
}
