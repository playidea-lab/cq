package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// TestParseSemver verifies int-based parsing.
func TestParseSemver(t *testing.T) {
	cases := []struct {
		in      string
		want    [3]int
		wantErr bool
	}{
		{"v0.62.0", [3]int{0, 62, 0}, false},
		{"v0.9.0", [3]int{0, 9, 0}, false},
		{"v0.10.0", [3]int{0, 10, 0}, false},
		{"1.2.3", [3]int{1, 2, 3}, false},
		{"bad", [3]int{}, true},
		{"v1.x.0", [3]int{}, true},
	}
	for _, c := range cases {
		got, err := parseSemver(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSemver(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSemver(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSemver(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestSemverLess verifies int comparison avoids lexicographic bugs.
func TestSemverLess(t *testing.T) {
	// v0.9.0 < v0.10.0 (int comparison — not lexicographic)
	a, _ := parseSemver("v0.9.0")
	b, _ := parseSemver("v0.10.0")
	if !semverLess(a, b) {
		t.Errorf("expected v0.9.0 < v0.10.0")
	}
	if semverLess(b, a) {
		t.Errorf("expected v0.10.0 not < v0.9.0")
	}

	// equal versions are not less
	c, _ := parseSemver("v0.62.0")
	if semverLess(c, c) {
		t.Errorf("expected v0.62.0 not < v0.62.0")
	}
}

// newVersionTestServer creates a test server with a fresh SQLite store.
func newVersionTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{Store: st, Version: "test"})
}

func registerWorkerVersion(t *testing.T, srv *Server, version string) string {
	t.Helper()
	body := `{"hostname":"test-host","version":"` + version + `"}`
	req := httptest.NewRequest("POST", "/v1/workers/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleWorkerRegister(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register worker: status %d, body: %s", w.Code, w.Body.String())
	}
	var resp model.WorkerRegisterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp.WorkerID
}

func acquireLease(t *testing.T, srv *Server, workerID string) model.LeaseAcquireResponse {
	t.Helper()
	body := `{"worker_id":"` + workerID + `"}`
	req := httptest.NewRequest("POST", "/v1/leases/acquire", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleLeaseAcquire(w, req)

	var resp model.LeaseAcquireResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode lease response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// TestVersionGate tests C5_MIN_VERSION gate scenarios.
func TestVersionGate(t *testing.T) {
	t.Run("below_min_version_returns_upgrade", func(t *testing.T) {
		srv := newVersionTestServer(t)
		workerID := registerWorkerVersion(t, srv, "v0.61.0")
		t.Setenv("C5_MIN_VERSION", "v0.62.0")
		resp := acquireLease(t, srv, workerID)
		if resp.Control == nil {
			t.Fatal("expected control message, got nil")
		}
		if resp.Control.Action != "upgrade" {
			t.Errorf("expected action=upgrade, got %q", resp.Control.Action)
		}
		if resp.JobID != "" {
			t.Errorf("expected no job_id, got %q", resp.JobID)
		}
	})

	t.Run("equal_to_min_version_no_upgrade", func(t *testing.T) {
		srv := newVersionTestServer(t)
		workerID := registerWorkerVersion(t, srv, "v0.62.0")
		t.Setenv("C5_MIN_VERSION", "v0.62.0")
		resp := acquireLease(t, srv, workerID)
		if resp.Control != nil && resp.Control.Action == "upgrade" {
			t.Errorf("expected no upgrade control for version == min, got %v", resp.Control)
		}
	})

	t.Run("no_min_version_no_gate", func(t *testing.T) {
		srv := newVersionTestServer(t)
		workerID := registerWorkerVersion(t, srv, "v0.61.0")
		t.Setenv("C5_MIN_VERSION", "")
		resp := acquireLease(t, srv, workerID)
		if resp.Control != nil && resp.Control.Action == "upgrade" {
			t.Errorf("expected no gate when C5_MIN_VERSION unset")
		}
	})

	t.Run("int_comparison_v0_9_lt_v0_10", func(t *testing.T) {
		srv := newVersionTestServer(t)
		workerID := registerWorkerVersion(t, srv, "v0.9.0")
		t.Setenv("C5_MIN_VERSION", "v0.10.0")
		resp := acquireLease(t, srv, workerID)
		if resp.Control == nil || resp.Control.Action != "upgrade" {
			t.Errorf("expected upgrade for v0.9.0 < v0.10.0 (int check), got control=%v", resp.Control)
		}
	})
}
