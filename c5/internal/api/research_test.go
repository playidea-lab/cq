package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

func newResearchTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "research_test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{
		Store:   st,
		Version: "test",
	})
}

func doResearchRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

// TestResearchState_GetInitial verifies that GET /v1/research/state returns a
// default row on first call (upsert on first GET).
func TestResearchState_GetInitial(t *testing.T) {
	srv := newResearchTestServer(t)
	w := doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var st model.ResearchState
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Round != 1 {
		t.Errorf("expected round=1, got %d", st.Round)
	}
	if st.Phase != "CONFERENCE" {
		t.Errorf("expected phase=CONFERENCE, got %s", st.Phase)
	}
	if st.Version != 0 {
		t.Errorf("expected version=0, got %d", st.Version)
	}
}

// TestResearchState_PutVersionMatch verifies that PUT with the correct version
// succeeds and increments version by 1.
func TestResearchState_PutVersionMatch(t *testing.T) {
	srv := newResearchTestServer(t)

	// Ensure row exists.
	doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)

	w := doResearchRequest(t, srv, http.MethodPut, "/v1/research/state", map[string]any{
		"round":   2,
		"phase":   "RESEARCH",
		"version": 0, // matches current version
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var st model.ResearchState
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Version != 1 {
		t.Errorf("expected version=1, got %d", st.Version)
	}
	if st.Round != 2 {
		t.Errorf("expected round=2, got %d", st.Round)
	}
	if st.Phase != "RESEARCH" {
		t.Errorf("expected phase=RESEARCH, got %s", st.Phase)
	}
}

// TestResearchState_PutVersionConflict verifies that PUT with a wrong version
// returns 409 Conflict.
func TestResearchState_PutVersionConflict(t *testing.T) {
	srv := newResearchTestServer(t)

	// Ensure row exists.
	doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)

	w := doResearchRequest(t, srv, http.MethodPut, "/v1/research/state", map[string]any{
		"round":   3,
		"phase":   "RESEARCH",
		"version": 99, // wrong version
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// TestResearchState_Lock verifies lock acquisition and release.
func TestResearchState_Lock(t *testing.T) {
	srv := newResearchTestServer(t)

	// Ensure row exists.
	doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)

	// Acquire lock.
	w := doResearchRequest(t, srv, http.MethodPost, "/v1/research/state/lock", map[string]any{
		"worker_id": "worker-1",
		"ttl_sec":   60,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("acquire lock: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if acquired, _ := resp["acquired"].(bool); !acquired {
		t.Errorf("expected acquired=true, got %v", resp)
	}

	// A different worker should fail to acquire.
	w2 := doResearchRequest(t, srv, http.MethodPost, "/v1/research/state/lock", map[string]any{
		"worker_id": "worker-2",
		"ttl_sec":   60,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("second acquire: expected 200, got %d", w2.Code)
	}
	var resp2 map[string]any
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if acquired, _ := resp2["acquired"].(bool); acquired {
		t.Errorf("expected acquired=false for second worker")
	}

	// Release lock via query param (DELETE body not reliable through HTTP intermediaries).
	wd := httptest.NewRequest(http.MethodDelete, "/v1/research/state/lock?worker_id=worker-1", nil)
	wdRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(wdRec, wd)
	if wdRec.Code != http.StatusOK {
		t.Fatalf("release lock: expected 200, got %d: %s", wdRec.Code, wdRec.Body.String())
	}

	// Now worker-2 can acquire.
	w3 := doResearchRequest(t, srv, http.MethodPost, "/v1/research/state/lock", map[string]any{
		"worker_id": "worker-2",
		"ttl_sec":   60,
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("third acquire: expected 200, got %d", w3.Code)
	}
	var resp3 map[string]any
	if err := json.NewDecoder(w3.Body).Decode(&resp3); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if acquired, _ := resp3["acquired"].(bool); !acquired {
		t.Errorf("expected acquired=true for worker-2 after release")
	}
}

// TestResearchState_LockExpiry verifies that a stale lock (past TTL) is auto-evicted
// on the next acquire attempt.
func TestResearchState_LockExpiry(t *testing.T) {
	srv := newResearchTestServer(t)

	// Ensure row exists.
	doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)

	// Acquire lock with a 1-second TTL.
	w := doResearchRequest(t, srv, http.MethodPost, "/v1/research/state/lock", map[string]any{
		"worker_id": "worker-expire",
		"ttl_sec":   1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("acquire: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if acquired, _ := resp["acquired"].(bool); !acquired {
		t.Fatalf("expected acquired=true")
	}

	// Wait for the lock to expire.
	// lock_expires_at uses ISO 8601 with T separator (stored by Go, not SQLite datetime()),
	// so string comparison is safe within UTC timezone.
	time.Sleep(2 * time.Second)

	// A different worker should now be able to acquire (stale lock auto-evicted).
	w2 := doResearchRequest(t, srv, http.MethodPost, "/v1/research/state/lock", map[string]any{
		"worker_id": "worker-new",
		"ttl_sec":   60,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("acquire after expiry: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if acquired, _ := resp2["acquired"].(bool); !acquired {
		t.Errorf("expected acquired=true after stale lock expiry, got %v", resp2)
	}
}

// TestResearchState_ConcurrentPut spawns 2 goroutines that both PUT with version=0.
// Exactly one should succeed (200) and one should get 409.
func TestResearchState_ConcurrentPut(t *testing.T) {
	srv := newResearchTestServer(t)

	// Ensure row exists.
	doResearchRequest(t, srv, http.MethodGet, "/v1/research/state", nil)

	var (
		wg      sync.WaitGroup
		success int
		conflict int
		mu      sync.Mutex
	)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := doResearchRequest(t, srv, http.MethodPut, "/v1/research/state", map[string]any{
				"round":   2,
				"phase":   "RESEARCH",
				"version": 0,
			})
			mu.Lock()
			defer mu.Unlock()
			switch w.Code {
			case http.StatusOK:
				success++
			case http.StatusConflict:
				conflict++
			default:
				t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Errorf("expected exactly 1 success, got %d", success)
	}
	if conflict != 1 {
		t.Errorf("expected exactly 1 conflict, got %d", conflict)
	}
}
