package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleTaskEvents_NoEventsDir(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
	events := m["events"].([]any)
	if len(events) != 0 {
		t.Errorf("events len = %d, want 0", len(events))
	}
}

func TestHandleTaskEvents_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	eventsDir := filepath.Join(tmpDir, ".c4", "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

func TestHandleTaskEvents_ReadsAndDeletes(t *testing.T) {
	tmpDir := t.TempDir()
	eventsDir := filepath.Join(tmpDir, ".c4", "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write two task event files
	event1 := map[string]any{"task_id": "T-001", "status": "done"}
	event2 := map[string]any{"task_id": "T-002", "status": "in_progress"}

	writeEventFile := func(name string, event any) {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(eventsDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeEventFile("task-001.json", event1)
	writeEventFile("task-002.json", event2)

	result, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
	events := m["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}

	// Verify files are deleted
	remaining, err := os.ReadDir(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining files = %d, want 0 after read", len(remaining))
	}
}

func TestHandleTaskEvents_SkipsNonTaskFiles(t *testing.T) {
	tmpDir := t.TempDir()
	eventsDir := filepath.Join(tmpDir, ".c4", "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a task event and a non-task file
	taskEvent := map[string]any{"task_id": "T-003", "status": "done"}
	data, _ := json.Marshal(taskEvent)
	if err := os.WriteFile(filepath.Join(eventsDir, "task-003.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eventsDir, "other-event.json"), []byte(`{"foo":"bar"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count = %v, want 1 (only task-*.json)", m["count"])
	}

	// other-event.json should NOT be deleted
	if _, err := os.Stat(filepath.Join(eventsDir, "other-event.json")); os.IsNotExist(err) {
		t.Error("other-event.json was deleted but should not have been")
	}
}

func TestHandleTaskEvents_SecondCallEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	eventsDir := filepath.Join(tmpDir, ".c4", "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	event := map[string]any{"task_id": "T-004", "status": "done"}
	data, _ := json.Marshal(event)
	if err := os.WriteFile(filepath.Join(eventsDir, "task-004.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// First call returns the event
	result1, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	m1 := result1.(map[string]any)
	if m1["count"] != 1 {
		t.Errorf("first call count = %v, want 1", m1["count"])
	}

	// Second call returns empty (files deleted)
	result2, err := handleTaskEvents(tmpDir)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	m2 := result2.(map[string]any)
	if m2["count"] != 0 {
		t.Errorf("second call count = %v, want 0", m2["count"])
	}
}
