package eventbus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreEvent(t *testing.T) {
	s := tempStore(t)

	id, err := s.StoreEvent("drive.uploaded", "c4.drive", json.RawMessage(`{"path":"/test.pdf"}`), "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty event ID")
	}

	events, err := s.ListEvents("drive.uploaded", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "drive.uploaded" {
		t.Errorf("expected type drive.uploaded, got %s", events[0].Type)
	}
	if events[0].Source != "c4.drive" {
		t.Errorf("expected source c4.drive, got %s", events[0].Source)
	}
	if events[0].ProjectID != "proj1" {
		t.Errorf("expected project_id proj1, got %s", events[0].ProjectID)
	}
}

func TestListEventsFilters(t *testing.T) {
	s := tempStore(t)

	s.StoreEvent("drive.uploaded", "c4.drive", nil, "")
	s.StoreEvent("drive.deleted", "c4.drive", nil, "")
	s.StoreEvent("task.completed", "c4.core", nil, "")

	// Filter by type
	events, _ := s.ListEvents("drive.uploaded", 10, 0)
	if len(events) != 1 {
		t.Errorf("expected 1 drive.uploaded event, got %d", len(events))
	}

	// All events
	events, _ = s.ListEvents("", 10, 0)
	if len(events) != 3 {
		t.Errorf("expected 3 total events, got %d", len(events))
	}
}

func TestMarkProcessed(t *testing.T) {
	s := tempStore(t)

	id, _ := s.StoreEvent("test.event", "test", nil, "")
	if err := s.MarkProcessed(id); err != nil {
		t.Fatal(err)
	}

	events, _ := s.ListEvents("test.event", 1, 0)
	if len(events) != 1 || !events[0].Processed {
		t.Error("expected event to be marked processed")
	}
}

func TestAddAndListRules(t *testing.T) {
	s := tempStore(t)

	id, err := s.AddRule("log-all", "*", "", "log", "", true, 999)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty rule ID")
	}

	_, err = s.AddRule("parse-pdf", "drive.uploaded", `{"content_type":"application/pdf"}`, "rpc", `{"method":"parse"}`, true, 100)
	if err != nil {
		t.Fatal(err)
	}

	rules, err := s.ListRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	// Ordered by priority DESC
	if rules[0].Name != "log-all" {
		t.Errorf("expected first rule log-all (priority 999), got %s", rules[0].Name)
	}
}

func TestRemoveRule(t *testing.T) {
	s := tempStore(t)

	s.AddRule("temp-rule", "test.*", "", "log", "", true, 0)

	err := s.RemoveRule("", "temp-rule")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := s.ListRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after remove, got %d", len(rules))
	}

	// Removing non-existent rule should error
	err = s.RemoveRule("", "nonexistent")
	if err == nil {
		t.Error("expected error removing non-existent rule")
	}
}

func TestToggleRule(t *testing.T) {
	s := tempStore(t)

	s.AddRule("toggle-test", "test.*", "", "log", "", true, 0)

	if err := s.ToggleRule("toggle-test", false); err != nil {
		t.Fatal(err)
	}
	rules, _ := s.ListRules()
	if len(rules) != 1 || rules[0].Enabled {
		t.Error("expected rule to be disabled")
	}

	if err := s.ToggleRule("toggle-test", true); err != nil {
		t.Fatal(err)
	}
	rules, _ = s.ListRules()
	if len(rules) != 1 || !rules[0].Enabled {
		t.Error("expected rule to be enabled")
	}
}

func TestMatchRules(t *testing.T) {
	s := tempStore(t)

	s.AddRule("all", "*", "", "log", "", true, 999)
	s.AddRule("drive-all", "drive.*", "", "log", "", true, 100)
	s.AddRule("exact", "drive.uploaded", "", "rpc", "", true, 50)
	s.AddRule("disabled", "drive.*", "", "log", "", false, 0)

	matched, err := s.MatchRules("drive.uploaded")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 3 {
		t.Fatalf("expected 3 matching rules, got %d", len(matched))
	}

	matched, err = s.MatchRules("drive.deleted")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matching rules for drive.deleted, got %d", len(matched))
	}

	matched, err = s.MatchRules("task.completed")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 matching rule for task.completed, got %d", len(matched))
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern, eventType string
		want               bool
	}{
		{"*", "anything", true},
		{"drive.*", "drive.uploaded", true},
		{"drive.*", "drive.deleted", true},
		{"drive.*", "task.completed", false},
		{"drive.uploaded", "drive.uploaded", true},
		{"drive.uploaded", "drive.deleted", false},
		{"task.*", "task.completed", true},
		{"task.*", "task.created", true},
	}

	for _, tt := range tests {
		if got := matchPattern(tt.pattern, tt.eventType); got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
		}
	}
}

func TestLogDispatch(t *testing.T) {
	s := tempStore(t)

	err := s.LogDispatch("ev-1", "rule-1", "ok", "", 42)
	if err != nil {
		t.Fatal(err)
	}

	// Verify by counting rows
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 log entry, got %d", count)
	}
}

func TestDuplicateRuleName(t *testing.T) {
	s := tempStore(t)

	_, err := s.AddRule("unique-name", "*", "", "log", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.AddRule("unique-name", "drive.*", "", "log", "", true, 0)
	if err == nil {
		t.Error("expected error on duplicate rule name")
	}
}

func TestNewStoreCreatesDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	s, err := NewStore(filepath.Join(nested, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Error("expected nested directory to be created")
	}
}
