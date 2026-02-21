package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// mockComponent implements Component for testing.
type mockComponent struct {
	name       string
	startErr   error
	stopErr    error
	health     ComponentHealth
	startOrder *[]string // shared slice to record start order
	stopOrder  *[]string // shared slice to record stop order
	mu         sync.Mutex
	started    bool
	stopped    bool
}

func (m *mockComponent) Name() string { return m.name }

func (m *mockComponent) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	if m.startOrder != nil {
		*m.startOrder = append(*m.startOrder, m.name)
	}
	return nil
}

func (m *mockComponent) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	if m.stopOrder != nil {
		*m.stopOrder = append(*m.stopOrder, m.name)
	}
	return m.stopErr
}

func (m *mockComponent) Health() ComponentHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.health
}

func TestManagerStartAllStopAll_Order(t *testing.T) {
	var startOrder, stopOrder []string

	a := &mockComponent{name: "alpha", health: ComponentHealth{Status: "ok"}, startOrder: &startOrder, stopOrder: &stopOrder}
	b := &mockComponent{name: "beta", health: ComponentHealth{Status: "ok"}, startOrder: &startOrder, stopOrder: &stopOrder}
	c := &mockComponent{name: "gamma", health: ComponentHealth{Status: "ok"}, startOrder: &startOrder, stopOrder: &stopOrder}

	mgr := NewManager()
	mgr.Register(a)
	mgr.Register(b)
	mgr.Register(c)

	ctx := context.Background()

	// StartAll
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	// Verify start order
	if len(startOrder) != 3 {
		t.Fatalf("expected 3 starts, got %d", len(startOrder))
	}
	if startOrder[0] != "alpha" || startOrder[1] != "beta" || startOrder[2] != "gamma" {
		t.Errorf("start order = %v, want [alpha beta gamma]", startOrder)
	}

	// StopAll
	if err := mgr.StopAll(ctx); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	// Verify stop order (reverse)
	if len(stopOrder) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(stopOrder))
	}
	if stopOrder[0] != "gamma" || stopOrder[1] != "beta" || stopOrder[2] != "alpha" {
		t.Errorf("stop order = %v, want [gamma beta alpha]", stopOrder)
	}
}

func TestManagerStartAll_Rollback(t *testing.T) {
	var startOrder, stopOrder []string
	errFail := errors.New("component failed")

	a := &mockComponent{name: "alpha", health: ComponentHealth{Status: "ok"}, startOrder: &startOrder, stopOrder: &stopOrder}
	b := &mockComponent{name: "beta", startErr: errFail, startOrder: &startOrder, stopOrder: &stopOrder}
	c := &mockComponent{name: "gamma", health: ComponentHealth{Status: "ok"}, startOrder: &startOrder, stopOrder: &stopOrder}

	mgr := NewManager()
	mgr.Register(a)
	mgr.Register(b)
	mgr.Register(c)

	err := mgr.StartAll(context.Background())
	if err == nil {
		t.Fatal("expected error from StartAll")
	}
	if !errors.Is(err, errFail) {
		t.Errorf("error = %v, want wrapping %v", err, errFail)
	}

	// Only alpha should have started
	if len(startOrder) != 1 || startOrder[0] != "alpha" {
		t.Errorf("startOrder = %v, want [alpha]", startOrder)
	}

	// Alpha should have been stopped (rollback)
	if len(stopOrder) != 1 || stopOrder[0] != "alpha" {
		t.Errorf("stopOrder = %v, want [alpha]", stopOrder)
	}

	// Gamma should never have started
	if c.started {
		t.Error("gamma should not have been started")
	}
}

func TestHealthHandler_AllOK(t *testing.T) {
	mgr := NewManager()
	mgr.Register(&mockComponent{name: "a", health: ComponentHealth{Status: "ok"}})
	mgr.Register(&mockComponent{name: "b", health: ComponentHealth{Status: "ok"}})

	// Need to call StartAll so components are tracked
	_ = mgr.StartAll(context.Background())

	handler := HealthHandler(mgr)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("overall status = %q, want %q", resp.Status, "ok")
	}
	if len(resp.Components) != 2 {
		t.Errorf("components count = %d, want 2", len(resp.Components))
	}
}

func TestHealthHandler_Degraded(t *testing.T) {
	mgr := NewManager()
	mgr.Register(&mockComponent{name: "a", health: ComponentHealth{Status: "ok"}})
	mgr.Register(&mockComponent{name: "b", health: ComponentHealth{Status: "error", Detail: "connection lost"}})

	_ = mgr.StartAll(context.Background())

	handler := HealthHandler(mgr)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("overall status = %q, want %q", resp.Status, "degraded")
	}
}

func TestHealthHandler_NoComponents(t *testing.T) {
	mgr := NewManager()

	handler := HealthHandler(mgr)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("overall status = %q, want %q", resp.Status, "ok")
	}
}

func TestManagerComponentCount(t *testing.T) {
	mgr := NewManager()
	if mgr.ComponentCount() != 0 {
		t.Errorf("count = %d, want 0", mgr.ComponentCount())
	}

	mgr.Register(&mockComponent{name: "a"})
	mgr.Register(&mockComponent{name: "b"})

	if mgr.ComponentCount() != 2 {
		t.Errorf("count = %d, want 2", mgr.ComponentCount())
	}
}

func TestManagerStopAll_ReturnsFirstError(t *testing.T) {
	var stopOrder []string
	errFirst := errors.New("first stop error")
	errSecond := errors.New("second stop error")

	a := &mockComponent{name: "alpha", health: ComponentHealth{Status: "ok"}, stopErr: errSecond, stopOrder: &stopOrder}
	b := &mockComponent{name: "beta", health: ComponentHealth{Status: "ok"}, stopErr: errFirst, stopOrder: &stopOrder}

	mgr := NewManager()
	mgr.Register(a)
	mgr.Register(b)

	_ = mgr.StartAll(context.Background())
	err := mgr.StopAll(context.Background())

	if err == nil {
		t.Fatal("expected error from StopAll")
	}
	// Stop order is reverse: beta first, alpha second
	// First error encountered should be from beta
	if !errors.Is(err, errFirst) {
		t.Errorf("error = %v, want %v", err, errFirst)
	}

	// Both should have been attempted
	if len(stopOrder) != 2 {
		t.Errorf("stop attempts = %d, want 2", len(stopOrder))
	}
}
