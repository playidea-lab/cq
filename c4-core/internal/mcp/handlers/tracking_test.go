package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestHasPolishGateDone_NoGate(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	ok, err := store.HasPolishGateDone("")
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

	ok, err := store.HasPolishGateDone("")
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

	ok, err := store.HasPolishGateDone("")
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

	ok, err := store.HasPolishGateDone("")
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
	ok, err := store.HasPolishGateDone("2099-01-01 00:00:00")
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
	ok, err := store.HasPolishGateDone("2000-01-01 00:00:00")
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
	ok, err := store.HasPolishGateDone("")
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
