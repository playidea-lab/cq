package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
)

// mockRestarter implements Restarter for testing.
type mockRestarter struct {
	called  int
	newAddr string
	err     error
}

func (m *mockRestarter) Restart() (string, error) {
	m.called++
	if m.err != nil {
		return "", m.err
	}
	return m.newAddr, nil
}

// startMockSidecar starts a minimal JSON-RPC server that responds to any method.
func startMockSidecar(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

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
				resp := map[string]any{
					"result": map[string]any{"status": "ok"},
					"error":  nil,
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

// TestProxyCallSuccess verifies normal proxy call works.
func TestProxyCallSuccess(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	result, err := proxy.Call("Ping", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected ok, got %v", result["status"])
	}
}

// TestProxyCallEmptyAddr verifies empty addr fails immediately.
func TestProxyCallEmptyAddr(t *testing.T) {
	proxy := NewBridgeProxy("")
	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty addr")
	}
}

// TestProxyAutoRestartSuccess verifies: conn fail → restart → retry succeeds.
func TestProxyAutoRestartSuccess(t *testing.T) {
	// Start a mock sidecar that the restarter will "switch to"
	goodAddr, cleanup := startMockSidecar(t)
	defer cleanup()

	// Start proxy pointing to a dead address
	proxy := NewBridgeProxy("127.0.0.1:1") // dead

	restarter := &mockRestarter{newAddr: goodAddr}
	proxy.SetRestarter(restarter)

	result, err := proxy.Call("Ping", map[string]any{})
	if err != nil {
		t.Fatalf("Call should succeed after restart, got: %v", err)
	}
	if restarter.called != 1 {
		t.Fatalf("expected 1 restart, got %d", restarter.called)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected ok, got %v", result["status"])
	}
}

// TestProxyAutoRestartFails verifies: conn fail → restart fails → original error returned.
func TestProxyAutoRestartFails(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:1") // dead

	restarter := &mockRestarter{err: fmt.Errorf("restart failed")}
	proxy.SetRestarter(restarter)

	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error when restart fails")
	}
	if restarter.called != 1 {
		t.Fatalf("expected 1 restart attempt, got %d", restarter.called)
	}
}

// TestProxyNoRestartOnBridgeError verifies: bridge-level error (not conn) doesn't trigger restart.
func TestProxyNoRestartOnBridgeError(t *testing.T) {
	// Start a mock sidecar that returns an error response
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				if n == 0 {
					return
				}
				errMsg := "method not found"
				resp := map[string]any{
					"result": nil,
					"error":  &errMsg,
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	proxy := NewBridgeProxy(ln.Addr().String())
	restarter := &mockRestarter{newAddr: ln.Addr().String()}
	proxy.SetRestarter(restarter)

	_, err = proxy.Call("Unknown", map[string]any{})
	if err == nil {
		t.Fatal("expected bridge error")
	}
	// Bridge error (not conn error) should NOT trigger restart
	if restarter.called != 0 {
		t.Fatalf("restart should not be called for bridge errors, got %d calls", restarter.called)
	}
}

// TestProxyIsAvailable verifies IsAvailable checks.
func TestProxyIsAvailable(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	if !proxy.IsAvailable() {
		t.Fatal("expected available")
	}

	emptyProxy := NewBridgeProxy("")
	if emptyProxy.IsAvailable() {
		t.Fatal("expected unavailable for empty addr")
	}
}

// TestProxyUpdateAddr verifies addr can be updated.
func TestProxyUpdateAddr(t *testing.T) {
	proxy := NewBridgeProxy("old:1234")
	proxy.UpdateAddr("new:5678")

	proxy.mu.Lock()
	got := proxy.addr
	proxy.mu.Unlock()

	if got != "new:5678" {
		t.Fatalf("expected new:5678, got %s", got)
	}
}

// TestProxyNoRestarterConfigured verifies graceful handling when no restarter set.
func TestProxyNoRestarterConfigured(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:1") // dead, no restarter
	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should fail without panic (no restarter = no retry)
}
