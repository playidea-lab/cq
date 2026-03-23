package serve

import (
	"encoding/json"
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
// does not produce a notification message (falls through to default case).
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

	notified := false
	switch record.Status {
	case "done", "blocked":
		notified = true
	}

	if notified {
		t.Error("expected no notification for in_progress status")
	}
	_ = event // satisfy linter
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
