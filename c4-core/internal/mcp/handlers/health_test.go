package handlers

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// mockAddrGetter implements LazyAddrGetter for testing.
type mockAddrGetter struct {
	addr string
	err  error
}

func (m *mockAddrGetter) Addr() (string, error) { return m.addr, m.err }

func TestCheckSQLiteNilDB(t *testing.T) {
	c := checkSQLite(nil, 2*time.Second)
	if c.Status != "error" {
		t.Errorf("status = %q, want error", c.Status)
	}
	if c.Error == nil || *c.Error != "database not initialized" {
		t.Errorf("unexpected error message: %v", c.Error)
	}
}

func TestCheckSQLiteOK(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := checkSQLite(db, 2*time.Second)
	if c.Status != "ok" {
		t.Errorf("status = %q, want ok", c.Status)
	}
	if c.LatencyMs <= 0 {
		t.Errorf("latency = %f, want > 0", c.LatencyMs)
	}
}

func TestCheckSidecarNil(t *testing.T) {
	c := checkSidecar(nil)
	if c.Status != "error" {
		t.Errorf("status = %q, want error", c.Status)
	}
}

func TestCheckSidecarOK(t *testing.T) {
	c := checkSidecar(&mockAddrGetter{addr: "localhost:9000"})
	if c.Status != "ok" {
		t.Errorf("status = %q, want ok", c.Status)
	}
}

func TestCheckSidecarError(t *testing.T) {
	c := checkSidecar(&mockAddrGetter{err: fmt.Errorf("connection refused")})
	if c.Status != "error" {
		t.Errorf("status = %q, want error", c.Status)
	}
	if c.Error == nil || *c.Error != "connection refused" {
		t.Errorf("unexpected error: %v", c.Error)
	}
}

func TestCheckKnowledgeNil(t *testing.T) {
	c := checkKnowledge(nil, 2*time.Second)
	if c.Status != "error" {
		t.Errorf("status = %q, want error", c.Status)
	}
}

func TestHandleHealthAllNil(t *testing.T) {
	deps := &HealthDeps{StartTime: time.Now()}
	result, err := handleHealth(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	// SQLite nil → unhealthy (critical subsystem)
	if m["status"] != "unhealthy" {
		t.Errorf("status = %v, want unhealthy", m["status"])
	}
	checks := m["checks"].([]healthCheck)
	if len(checks) != 3 {
		t.Errorf("checks count = %d, want 3", len(checks))
	}
}

func TestHandleHealthSQLiteOK(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	deps := &HealthDeps{
		DB:        db,
		Sidecar:   &mockAddrGetter{addr: "localhost:9000"},
		StartTime: time.Now().Add(-5 * time.Second),
	}
	result, err := handleHealth(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	// SQLite ok, sidecar ok, knowledge nil → degraded
	if m["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", m["status"])
	}
	uptime := m["uptime_seconds"].(int)
	if uptime < 4 {
		t.Errorf("uptime = %d, want >= 4", uptime)
	}
}
