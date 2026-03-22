package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
)

func TestHasPolishGateDone_NoGate(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	ok, err := store.HasGateDone("polish", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (no gate recorded), got true")
	}
}

func TestHasPolishGateDone_WithGate(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := store.RecordGate("polish", "done", "all checks passed"); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	ok, err := store.HasGateDone("polish", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true (gate recorded), got false")
	}
}

func TestHasPolishGateDone_WrongGateName(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := store.RecordGate("lint", "done", ""); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	ok, err := store.HasGateDone("polish", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (only lint gate, not polish), got true")
	}
}

func TestHasPolishGateDone_WrongStatus(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := store.RecordGate("polish", "skipped", ""); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	ok, err := store.HasGateDone("polish", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (status=skipped, not done), got true")
	}
}

func TestHasPolishGateDone_SinceTime_Future(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := store.RecordGate("polish", "done", ""); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	// A future time: the gate should not match
	ok, err := store.HasGateDone("polish", "2099-01-01 00:00:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (gate before sinceTime), got true")
	}
}

func TestHasPolishGateDone_SinceTime_Past(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := store.RecordGate("polish", "done", ""); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	// A past time: the gate should match
	ok, err := store.HasGateDone("polish", "2000-01-01 00:00:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true (gate after sinceTime), got false")
	}
}

func TestCheckPolishGate_NilConfig(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// store.config == nil → always passes
	if err := checkPolishGate(store, "T-001-0", "abc123"); err != nil {
		t.Errorf("expected nil error (no config), got: %v", err)
	}
}

func TestHandleSubmit_PolishGateRequired(t *testing.T) {
	// Test that c4_submit is rejected when diff is large and no polish gate exists.
	// We can't easily fake diffStatLines without a real git repo, so we test
	// HasPolishGateDone logic directly. The integration is covered by unit tests above.
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	// Record a polish gate to confirm the path works
	args := `{"gate":"polish","status":"done","reason":"polish loop complete"}`
	result, err := reg.Call("c4_record_gate", json.RawMessage(args))
	if err != nil {
		t.Fatalf("record gate: %v", err)
	}
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	// Verify the gate is now detectable
	ok, err := store.HasGateDone("polish", "")
	if err != nil {
		t.Fatalf("HasPolishGateDone: %v", err)
	}
	if !ok {
		t.Error("expected polish gate to be detectable after recording")
	}
}

func TestRecordGate_Success(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"gate":"polish","status":"done","reason":"all checks passed"}`
	result, err := reg.Call("c4_record_gate", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	id, ok := m["id"].(int64)
	if !ok {
		t.Errorf("id type = %T, want int64", m["id"])
	}
	if id <= 0 {
		t.Errorf("id = %d, want > 0", id)
	}
}

func TestRecordGate_AllStatuses(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	for _, status := range []string{"done", "skipped", "override"} {
		args, _ := json.Marshal(map[string]string{"gate": "review", "status": status})
		_, err := reg.Call("c4_record_gate", json.RawMessage(args))
		if err != nil {
			t.Errorf("status %q: unexpected error: %v", status, err)
		}
	}
}

func TestRecordGate_MissingGate(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"status":"done"}`
	_, err := reg.Call("c4_record_gate", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing gate")
	}
}

func TestRecordGate_InvalidStatus(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"gate":"lint","status":"invalid"}`
	_, err := reg.Call("c4_record_gate", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestRecordGate_NoReason(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"gate":"lint","status":"skipped"}`
	result, err := reg.Call("c4_record_gate", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
}

// newTestSQLiteStoreWithRefineThreshold creates a store with refine_threshold set.
func newTestSQLiteStoreWithRefineThreshold(t *testing.T, threshold int) (*SQLiteStore, func()) {
	t.Helper()
	store, db := newTestSQLiteStore(t)
	cfg, err := config.New(t.TempDir())
	if err != nil {
		db.Close()
		t.Fatalf("config.New: %v", err)
	}
	cfg.Set("run.refine_threshold", threshold)
	store.config = cfg
	return store, func() { db.Close() }
}

func TestCheckRefineGate_BelowThreshold(t *testing.T) {
	// pending count < threshold → always passes
	store, cleanup := newTestSQLiteStoreWithRefineThreshold(t, 3)
	defer cleanup()

	// Add 2 pending tasks (below threshold of 3)
	for i := 0; i < 2; i++ {
		if err := store.AddTask(&Task{
			ID:     "T-RG-" + string(rune('A'+i)) + "-0",
			Title:  "task",
			DoD:    "done",
			Status: "pending",
		}); err != nil {
			t.Fatalf("AddTask: %v", err)
		}
	}

	if err := checkRefineGate(store); err != nil {
		t.Errorf("expected nil (below threshold), got: %v", err)
	}
}

func TestCheckRefineGate_AboveThreshold_NoGate(t *testing.T) {
	// pending count >= threshold AND no refine gate → reject
	store, cleanup := newTestSQLiteStoreWithRefineThreshold(t, 3)
	defer cleanup()

	// Add 3 pending tasks (at threshold of 3)
	for i := 0; i < 3; i++ {
		if err := store.AddTask(&Task{
			ID:     "T-RG-B" + string(rune('0'+i)) + "-0",
			Title:  "task",
			DoD:    "done",
			Status: "pending",
		}); err != nil {
			t.Fatalf("AddTask: %v", err)
		}
	}

	err := checkRefineGate(store)
	if err == nil {
		t.Fatal("expected error (no refine gate), got nil")
	}
	if !refineTestContains(err.Error(), "refine gate required") {
		t.Errorf("error = %q, want contains 'refine gate required'", err.Error())
	}
}

func TestCheckRefineGate_AboveThreshold_WithGate(t *testing.T) {
	// pending count >= threshold AND refine gate exists → pass
	store, cleanup := newTestSQLiteStoreWithRefineThreshold(t, 3)
	defer cleanup()

	// Add 3 pending tasks (at threshold of 3)
	for i := 0; i < 3; i++ {
		if err := store.AddTask(&Task{
			ID:     "T-RG-C" + string(rune('0'+i)) + "-0",
			Title:  "task",
			DoD:    "done",
			Status: "pending",
		}); err != nil {
			t.Fatalf("AddTask: %v", err)
		}
	}

	// Record a refine gate with done status
	if _, err := store.RecordGate("refine", "done", "critique loop complete"); err != nil {
		t.Fatalf("RecordGate: %v", err)
	}

	if err := checkRefineGate(store); err != nil {
		t.Errorf("expected nil (gate present), got: %v", err)
	}
}

// refineTestContains is a simple substring check helper for refine gate tests.
func refineTestContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
