package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHubComponent_Name(t *testing.T) {
	c := NewHubComponent(HubComponentConfig{})
	if c.Name() != "hub" {
		t.Errorf("Name() = %q, want %q", c.Name(), "hub")
	}
}

func TestHubComponent_Defaults(t *testing.T) {
	c := NewHubComponent(HubComponentConfig{})
	if c.cfg.Binary != "c5" {
		t.Errorf("default Binary = %q, want %q", c.cfg.Binary, "c5")
	}
	if c.cfg.Port != 8585 {
		t.Errorf("default Port = %d, want %d", c.cfg.Port, 8585)
	}
}

// TestHubComponent_Start_BinaryNotFound verifies that when the c5 binary cannot
// be found in PATH, Start returns nil (graceful skip) and the component is NOT
// marked as running.
func TestHubComponent_Start_BinaryNotFound(t *testing.T) {
	c := NewHubComponent(HubComponentConfig{
		Binary: "c5-binary-that-does-not-exist-xyz",
		Port:   19999,
	})

	ctx := context.Background()
	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error for missing binary: %v (want nil / graceful skip)", err)
	}

	// Component should NOT be running — it was skipped.
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()

	if running {
		t.Error("component should not be running when binary not found")
	}
}

// TestHubComponent_Stop_NotRunning verifies that Stop is idempotent when the
// component was never started.
func TestHubComponent_Stop_NotRunning(t *testing.T) {
	c := NewHubComponent(HubComponentConfig{})
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on not-running component: %v", err)
	}
}

// TestHubComponent_Health_NotRunning verifies Health returns "error" status
// when the component has not been started.
func TestHubComponent_Health_NotRunning(t *testing.T) {
	c := NewHubComponent(HubComponentConfig{})
	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "error")
	}
	if !strings.Contains(h.Detail, "not running") {
		t.Errorf("Health().Detail = %q, want contains 'not running'", h.Detail)
	}
}

// TestHubComponent_Health_Running_OKEndpoint verifies that Health returns "ok"
// when the /health endpoint responds 200.
func TestHubComponent_Health_Running_OKEndpoint(t *testing.T) {
	// Spin up a local test server that mimics c5's /health response.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	port := testServerPort(t, ts.URL)

	c := &HubComponent{
		cfg: HubComponentConfig{
			Binary: "c5",
			Port:   port,
		},
		running: true,
	}

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "ok")
	}
}

// TestHubComponent_Health_Running_DegradedEndpoint verifies that Health returns
// "degraded" when the /health endpoint returns a non-200 status.
func TestHubComponent_Health_Running_DegradedEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	port := testServerPort(t, ts.URL)

	c := &HubComponent{
		cfg: HubComponentConfig{
			Binary: "c5",
			Port:   port,
		},
		running: true,
	}

	h := c.Health()
	if h.Status != "degraded" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "degraded")
	}
}

// TestHubComponent_Health_Running_Unreachable verifies that Health returns
// "degraded" when the /health endpoint is not reachable.
func TestHubComponent_Health_Running_Unreachable(t *testing.T) {
	// Use a port that has nothing listening.
	c := &HubComponent{
		cfg: HubComponentConfig{
			Binary: "c5",
			Port:   19998,
		},
		running: true,
	}

	h := c.Health()
	if h.Status != "degraded" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "degraded")
	}
}

// testServerPort extracts the numeric port from a URL like "http://127.0.0.1:NNNNN".
func testServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	// rawURL = "http://127.0.0.1:PORT"
	idx := strings.LastIndex(rawURL, ":")
	if idx < 0 {
		t.Fatalf("unexpected test server URL format: %s", rawURL)
	}
	port, err := strconv.Atoi(rawURL[idx+1:])
	if err != nil {
		t.Fatalf("parse port from %q: %v", rawURL, err)
	}
	return port
}
