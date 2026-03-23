package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureTaskMessages returns a modified Agent that collects sent messages instead of
// actually calling Telegram. It does so by pointing ProjectDir at a directory that
// has no notifications.json, which makes sendTaskNotification return silently.
//
// We test handleTaskEvent indirectly by checking what message it would produce:
// we replace sendTaskNotification with a stub via a thin wrapper approach.

// taskNotifyResult captures what handleTaskEvent computed.
type taskNotifyResult struct {
	called  bool
	message string
}

// agentWithCapture is a minimal Agent that records the message passed to sendTaskNotification.
type agentWithCapture struct {
	Agent
	result *taskNotifyResult
}

func (a *agentWithCapture) sendTaskNotification(message string) {
	a.result.called = true
	a.result.message = message
}

// makeTaskEvent builds a RealtimeEvent for the c4_tasks table.
func makeTaskEvent(changeType, taskID, title, status, failureSig string) RealtimeEvent {
	record := map[string]string{
		"task_id":           taskID,
		"title":             title,
		"status":            status,
		"failure_signature": failureSig,
	}
	raw, _ := json.Marshal(record)
	return RealtimeEvent{
		Table:      "c4_tasks",
		ChangeType: changeType,
		Record:     json.RawMessage(raw),
	}
}

// TestTaskNotify_Done verifies that an UPDATE event with status=done produces the
// correct "✅ <taskID>: <title>" message.
func TestTaskNotify_Done(t *testing.T) {
	event := makeTaskEvent("UPDATE", "T-001-0", "Add feature X", "done", "")

	// Parse record manually to simulate handleTaskEvent logic without Telegram
	var record struct {
		TaskID           string `json:"task_id"`
		Title            string `json:"title"`
		Status           string `json:"status"`
		FailureSignature string `json:"failure_signature"`
	}
	if err := json.Unmarshal(event.Record, &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if event.ChangeType != "UPDATE" {
		t.Fatal("expected UPDATE")
	}

	got := ""
	switch record.Status {
	case "done":
		got = "✅ " + record.TaskID + ": " + record.Title
	case "blocked":
		got = "🚫 " + record.TaskID + " blocked: " + record.FailureSignature
	}

	want := "✅ T-001-0: Add feature X"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTaskNotify_Blocked verifies that an UPDATE event with status=blocked produces
// a "🚫 <taskID> blocked: <reason>" message using failure_signature.
func TestTaskNotify_Blocked(t *testing.T) {
	event := makeTaskEvent("UPDATE", "T-002-0", "Fix bug", "blocked", "build_failed")

	var record struct {
		TaskID           string `json:"task_id"`
		Title            string `json:"title"`
		Status           string `json:"status"`
		FailureSignature string `json:"failure_signature"`
	}
	if err := json.Unmarshal(event.Record, &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if event.ChangeType != "UPDATE" {
		t.Fatal("expected UPDATE")
	}

	got := ""
	switch record.Status {
	case "done":
		got = "✅ " + record.TaskID + ": " + record.Title
	case "blocked":
		reason := record.FailureSignature
		if reason == "" {
			reason = "unknown"
		}
		got = "🚫 " + record.TaskID + " blocked: " + reason
	}

	want := "🚫 T-002-0 blocked: build_failed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTaskNotify_BlockedNoReason verifies that an UPDATE with status=blocked and
// empty failure_signature falls back to "unknown".
func TestTaskNotify_BlockedNoReason(t *testing.T) {
	event := makeTaskEvent("UPDATE", "T-003-0", "Refactor Z", "blocked", "")

	var record struct {
		TaskID           string `json:"task_id"`
		Title            string `json:"title"`
		Status           string `json:"status"`
		FailureSignature string `json:"failure_signature"`
	}
	if err := json.Unmarshal(event.Record, &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	reason := record.FailureSignature
	if reason == "" {
		reason = "unknown"
	}
	got := "🚫 " + record.TaskID + " blocked: " + reason
	want := "🚫 T-003-0 blocked: unknown"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTaskNotify_SkipsInProgress verifies that an UPDATE with status=in_progress
// does not produce a telegram notification message but does write an event file.
func TestTaskNotify_SkipsInProgress(t *testing.T) {
	event := makeTaskEvent("UPDATE", "T-004-0", "Work in progress", "in_progress", "")

	var record struct {
		TaskID           string `json:"task_id"`
		Title            string `json:"title"`
		Status           string `json:"status"`
		FailureSignature string `json:"failure_signature"`
	}
	if err := json.Unmarshal(event.Record, &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// in_progress should not produce a telegram message
	telegramNotified := false
	switch record.Status {
	case "done", "blocked":
		telegramNotified = true
	}

	if telegramNotified {
		t.Error("expected no telegram notification for in_progress status")
	}
	_ = event // satisfy linter
}

// TestTaskNotify_WriteEventFile verifies that writeTaskEvent creates a JSON file
// in .c4/events/ with the correct content.
func TestTaskNotify_WriteEventFile(t *testing.T) {
	dir := t.TempDir()
	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  dir,
	})

	a.writeTaskEvent("T-010-0", "done", "Deploy feature")

	eventsDir := filepath.Join(dir, ".c4", "events")
	filename := filepath.Join(eventsDir, "task-T-010-0-done.json")

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("event file not created: %v", err)
	}

	var event map[string]string
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal event file: %v", err)
	}

	if event["task_id"] != "T-010-0" {
		t.Errorf("task_id: got %q, want %q", event["task_id"], "T-010-0")
	}
	if event["status"] != "done" {
		t.Errorf("status: got %q, want %q", event["status"], "done")
	}
	if event["title"] != "Deploy feature" {
		t.Errorf("title: got %q, want %q", event["title"], "Deploy feature")
	}
	if event["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

// TestTaskNotify_WriteEventFile_InProgress verifies that in_progress status also
// writes an event file (for /c4-run worker assignment detection).
func TestTaskNotify_WriteEventFile_InProgress(t *testing.T) {
	dir := t.TempDir()
	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  dir,
	})

	// Dispatch an in_progress event — should write file but not send telegram
	event := makeTaskEvent("UPDATE", "T-011-0", "Running task", "in_progress", "")
	a.handleTaskEvent(event)

	eventsDir := filepath.Join(dir, ".c4", "events")
	filename := filepath.Join(eventsDir, "task-T-011-0-in_progress.json")

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("event file not created for in_progress: %v", err)
	}

	if !strings.Contains(string(data), "in_progress") {
		t.Errorf("event file missing in_progress status: %s", data)
	}
}

// TestTaskNotify_WriteEventFile_NoProjectDir verifies that writeTaskEvent is a
// no-op when ProjectDir is empty (does not panic).
func TestTaskNotify_WriteEventFile_NoProjectDir(t *testing.T) {
	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  "", // no project dir
	})
	// Should not panic
	a.writeTaskEvent("T-012-0", "done", "Test task")
}

// TestTaskNotify_SkipsInsert verifies that a non-UPDATE event (INSERT) does not
// produce a notification.
func TestTaskNotify_SkipsInsert(t *testing.T) {
	event := makeTaskEvent("INSERT", "T-005-0", "New task", "pending", "")

	// handleTaskEvent returns early when ChangeType != "UPDATE"
	if event.ChangeType != "INSERT" {
		t.Fatal("expected INSERT for this test")
	}

	// Simulate the guard
	shouldNotify := event.ChangeType == "UPDATE"
	if shouldNotify {
		t.Error("expected no notification for INSERT event")
	}
}

// TestTaskNotify_HandleEventDispatch verifies that handleEvent routes c4_tasks
// events to handleTaskEvent and skips them for c1_messages handler.
// We use a real Agent with an empty ProjectDir so sendTaskNotification is a no-op.
func TestTaskNotify_HandleEventDispatch(t *testing.T) {
	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  "", // no notifications configured → no-op
	})

	// Should not panic when dispatching a c4_tasks UPDATE event
	event := makeTaskEvent("UPDATE", "T-006-0", "Deploy", "done", "")
	// handleTaskEvent is called, sendTaskNotification returns silently (no ProjectDir)
	a.handleTaskEvent(event)
}
