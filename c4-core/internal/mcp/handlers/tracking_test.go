package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

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
