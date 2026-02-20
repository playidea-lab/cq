package eventbus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestPurgeOldEvents(t *testing.T) {
	s := tempStore(t)

	// Store events
	s.StoreEvent("test.old", "test", nil, "")
	s.StoreEvent("test.new", "test", nil, "")

	// Purge with 0 duration should remove all (since they're all "now")
	// Actually let's purge with huge duration — should remove nothing
	n, err := s.PurgeOldEvents(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged (all recent), got %d", n)
	}

	events, _ := s.ListEvents("", 10, 0)
	if len(events) != 2 {
		t.Errorf("expected 2 events remaining, got %d", len(events))
	}
}

func TestPurgeByCount(t *testing.T) {
	s := tempStore(t)

	for i := 0; i < 5; i++ {
		s.StoreEvent("test.event", "test", nil, "")
	}

	n, err := s.PurgeByCount(3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 purged, got %d", n)
	}

	events, _ := s.ListEvents("", 10, 0)
	if len(events) != 3 {
		t.Errorf("expected 3 events remaining, got %d", len(events))
	}
}

func TestPurgeOldLogs(t *testing.T) {
	s := tempStore(t)

	s.LogDispatch("ev-1", "r-1", "ok", "", 10)
	s.LogDispatch("ev-2", "r-1", "error", "fail", 20)

	// All logs are recent, purge with 1h shouldn't remove any
	n, err := s.PurgeOldLogs(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged, got %d", n)
	}
}

func TestEventStats(t *testing.T) {
	s := tempStore(t)

	s.StoreEvent("test.a", "test", nil, "")
	s.StoreEvent("test.b", "test", nil, "")
	s.AddRule("rule1", "*", "", "log", "", true, 0)
	s.LogDispatch("ev-1", "r-1", "ok", "", 10)

	stats, err := s.EventStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["event_count"] != 2 {
		t.Errorf("expected event_count=2, got %v", stats["event_count"])
	}
	if stats["rule_count"] != 1 {
		t.Errorf("expected rule_count=1, got %v", stats["rule_count"])
	}
	if stats["log_count"] != 1 {
		t.Errorf("expected log_count=1, got %v", stats["log_count"])
	}
	if stats["oldest_event"] == "" {
		t.Error("expected non-empty oldest_event")
	}
}

func TestListLogs(t *testing.T) {
	s := tempStore(t)

	// Create a rule and dispatch to generate log entries
	ruleID, _ := s.AddRule("log-rule", "*", "", "log", "", true, 0)
	evID, _ := s.StoreEvent("test.event", "test", nil, "")
	s.LogDispatch(evID, ruleID, "ok", "", 42)

	logs, err := s.ListLogs("", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].EventID != evID {
		t.Errorf("expected event_id %s, got %s", evID, logs[0].EventID)
	}
	if logs[0].Status != "ok" {
		t.Errorf("expected status ok, got %s", logs[0].Status)
	}
	if logs[0].DurationMs != 42 {
		t.Errorf("expected duration 42, got %d", logs[0].DurationMs)
	}
}

func TestListLogsFilterByEvent(t *testing.T) {
	s := tempStore(t)

	s.LogDispatch("ev-1", "r-1", "ok", "", 10)
	s.LogDispatch("ev-2", "r-1", "ok", "", 20)
	s.LogDispatch("ev-1", "r-2", "error", "fail", 30)

	logs, err := s.ListLogs("ev-1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for ev-1, got %d", len(logs))
	}
}

func TestListEventsASC(t *testing.T) {
	s := tempStore(t)

	s.StoreEvent("test.a", "test", nil, "")
	s.StoreEvent("test.b", "test", nil, "")
	s.StoreEvent("test.c", "test", nil, "")

	events, err := s.ListEventsASC("", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// ASC order: first stored should come first
	if events[0].Type != "test.a" {
		t.Errorf("expected first event test.a, got %s", events[0].Type)
	}
	if events[2].Type != "test.c" {
		t.Errorf("expected last event test.c, got %s", events[2].Type)
	}
}

func TestEnsureDefaultRules(t *testing.T) {
	s := tempStore(t)

	yamlData := []byte(`rules:
  - name: test-rule-1
    event_pattern: "test.*"
    action_type: log
    enabled: true
    priority: 100
  - name: test-rule-2
    event_pattern: "drive.*"
    action_type: webhook
    action_config: '{"url":"http://example.com"}'
    enabled: false
    priority: 50
`)

	// First call should add both rules
	if err := s.EnsureDefaultRules(yamlData); err != nil {
		t.Fatal(err)
	}
	rules, _ := s.ListRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	// Second call should be idempotent — no duplicates
	if err := s.EnsureDefaultRules(yamlData); err != nil {
		t.Fatal(err)
	}
	rules, _ = s.ListRules()
	if len(rules) != 2 {
		t.Errorf("expected 2 rules after idempotent call, got %d", len(rules))
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

// --- v4: correlation_id tests ---

func TestStoreEventCorrelation(t *testing.T) {
	s := tempStore(t)

	id, err := s.StoreEvent("task.completed", "c4.core", json.RawMessage(`{"task_id":"T-001"}`), "proj1", "corr-abc-123")
	if err != nil {
		t.Fatal(err)
	}

	events, err := s.ListEvents("task.completed", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != id {
		t.Errorf("expected id %s, got %s", id, events[0].ID)
	}
	if events[0].CorrelationID != "corr-abc-123" {
		t.Errorf("expected correlation_id corr-abc-123, got %s", events[0].CorrelationID)
	}
}

func TestListEventsCorrelation(t *testing.T) {
	s := tempStore(t)

	// Store events with and without correlation_id
	s.StoreEvent("task.a", "test", nil, "", "corr-1")
	s.StoreEvent("task.b", "test", nil, "")
	s.StoreEvent("task.c", "test", nil, "", "corr-2")

	events, err := s.ListEvents("", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Check correlation IDs are persisted correctly
	corrMap := map[string]string{}
	for _, ev := range events {
		corrMap[ev.Type] = ev.CorrelationID
	}
	if corrMap["task.a"] != "corr-1" {
		t.Errorf("task.a: expected corr-1, got %s", corrMap["task.a"])
	}
	if corrMap["task.b"] != "" {
		t.Errorf("task.b: expected empty, got %s", corrMap["task.b"])
	}
	if corrMap["task.c"] != "corr-2" {
		t.Errorf("task.c: expected corr-2, got %s", corrMap["task.c"])
	}

	// ASC order also returns correlation_id
	eventsASC, err := s.ListEventsASC("", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if eventsASC[0].CorrelationID != "corr-1" {
		t.Errorf("ASC first: expected corr-1, got %s", eventsASC[0].CorrelationID)
	}
}

// --- v4: DLQ tests ---

func TestInsertDLQ(t *testing.T) {
	s := tempStore(t)

	err := s.InsertDLQ("ev-1", "rule-1", "my-rule", "task.failed", "timeout error", 3)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := s.ListDLQ(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(entries))
	}
	e := entries[0]
	if e.EventID != "ev-1" {
		t.Errorf("expected event_id ev-1, got %s", e.EventID)
	}
	if e.RuleName != "my-rule" {
		t.Errorf("expected rule_name my-rule, got %s", e.RuleName)
	}
	if e.EventType != "task.failed" {
		t.Errorf("expected event_type task.failed, got %s", e.EventType)
	}
	if e.Error != "timeout error" {
		t.Errorf("expected error 'timeout error', got %s", e.Error)
	}
	if e.RetryCount != 0 {
		t.Errorf("expected retry_count 0, got %d", e.RetryCount)
	}
	if e.MaxRetries != 3 {
		t.Errorf("expected max_retries 3, got %d", e.MaxRetries)
	}
}

func TestListDLQ(t *testing.T) {
	s := tempStore(t)

	// Insert multiple DLQ entries
	s.InsertDLQ("ev-1", "r-1", "rule-a", "task.failed", "err1", 3)
	s.InsertDLQ("ev-2", "r-2", "rule-b", "drive.error", "err2", 5)
	s.InsertDLQ("ev-3", "r-1", "rule-a", "task.failed", "err3", 3)

	entries, err := s.ListDLQ(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 DLQ entries, got %d", len(entries))
	}

	// Should be ordered by id DESC (newest first)
	if entries[0].EventID != "ev-3" {
		t.Errorf("expected newest first (ev-3), got %s", entries[0].EventID)
	}

	// Test limit
	entries, err = s.ListDLQ(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit=2, got %d", len(entries))
	}
}

func TestRetryDLQ(t *testing.T) {
	s := tempStore(t)

	s.InsertDLQ("ev-1", "r-1", "rule-a", "task.failed", "err1", 3)

	entries, _ := s.ListDLQ(10)
	dlqID := entries[0].ID

	// Increment retry count
	entry, err := s.IncrementDLQRetry(dlqID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RetryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", entry.RetryCount)
	}

	// Increment again
	entry, err = s.IncrementDLQRetry(dlqID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RetryCount != 2 {
		t.Errorf("expected retry_count 2, got %d", entry.RetryCount)
	}

	// Remove DLQ entry
	err = s.RemoveDLQ(dlqID)
	if err != nil {
		t.Fatal(err)
	}

	entries, _ = s.ListDLQ(10)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after remove, got %d", len(entries))
	}

	// Remove non-existent should error
	err = s.RemoveDLQ(99999)
	if err == nil {
		t.Error("expected error removing non-existent DLQ entry")
	}
}

func TestPurgeDLQ(t *testing.T) {
	s := tempStore(t)

	s.InsertDLQ("ev-1", "r-1", "rule-a", "task.failed", "err1", 3)
	s.InsertDLQ("ev-2", "r-2", "rule-b", "task.failed", "err2", 3)

	// All recent — purge with 1h shouldn't remove any
	n, err := s.PurgeDLQ(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged (all recent), got %d", n)
	}

	entries, _ := s.ListDLQ(10)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries remaining, got %d", len(entries))
	}
}
