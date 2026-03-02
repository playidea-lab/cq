package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/c1push"
)

// mockPusher captures pushed messages for testing.
type mockPusher struct {
	channels []string
	messages []c1push.PushMessage
}

func (m *mockPusher) EnsureChannel(_ context.Context, _, _, name string, _ c1push.Platform) (string, error) {
	m.channels = append(m.channels, name)
	return "test-channel-id", nil
}

func (m *mockPusher) AppendMessages(_ context.Context, _ string, msgs []c1push.PushMessage) error {
	m.messages = append(m.messages, msgs...)
	return nil
}

// newTempPositionStore creates a PositionStore backed by a temp file.
func newTempPositionStore(t *testing.T) (*PositionStore, error) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "positions.db")
	return NewPositionStore(dbPath)
}

func TestJournalWatcher_BasicFlow(t *testing.T) {
	tmpDir := t.TempDir()
	projectsRoot := filepath.Join(tmpDir, ".claude", "projects")
	slugDir := filepath.Join(projectsRoot, "-test-project")
	if err := os.MkdirAll(slugDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionUUID := "test-session-uuid-12345678"
	jsonlPath := filepath.Join(slugDir, sessionUUID+".jsonl")
	initialContent := `{"type":"user","uuid":"msg-1","message":{"content":"Hello"},"isMeta":false}` + "\n" +
		`{"type":"assistant","uuid":"msg-2","message":{"content":[{"type":"text","text":"Hi there"}]},"isMeta":false}` + "\n"

	if err := os.WriteFile(jsonlPath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}

	posStore, err := newTempPositionStore(t)
	if err != nil {
		t.Fatal(err)
	}
	defer posStore.Close()

	mock := &mockPusher{}
	watcher := NewJournalWatcher(mock, posStore, "default")
	watcher.projectsRoot = projectsRoot

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Cold scan only — do not start fsnotify watcher in tests.
	watcher.coldScan(ctx, slugDir)

	if len(mock.messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(mock.messages))
	}

	expectedChannel := "claude_code:" + sessionUUID
	if len(mock.channels) == 0 || mock.channels[0] != expectedChannel {
		t.Errorf("channel=%v, want %q", mock.channels, expectedChannel)
	}

	// Second scan should produce no new messages (offset preserved).
	mock.messages = nil
	watcher.coldScan(ctx, slugDir)
	if len(mock.messages) != 0 {
		t.Errorf("expected 0 messages on re-scan (dedup), got %d", len(mock.messages))
	}
}

func TestJournalWatcher_SkipsMetaLines(t *testing.T) {
	tmpDir := t.TempDir()
	posStore, err := newTempPositionStore(t)
	if err != nil {
		t.Fatal(err)
	}
	defer posStore.Close()

	mock := &mockPusher{}
	watcher := NewJournalWatcher(mock, posStore, "default")

	jsonlPath := filepath.Join(tmpDir, "session.jsonl")
	content := `{"type":"summary","uuid":"s-1","message":{"content":"summary text"},"isMeta":true}` + "\n" +
		`{"type":"user","uuid":"u-1","message":{"content":"real message"},"isMeta":false}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	watcher.processFile(ctx, jsonlPath)

	// Only 1 message (the non-meta user message).
	if len(mock.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(mock.messages))
	}
}

func TestJournalWatcher_NilPusherNoOp(t *testing.T) {
	posStore, err := newTempPositionStore(t)
	if err != nil {
		t.Fatal(err)
	}
	defer posStore.Close()

	// nil pusher — Start should return nil without panicking.
	watcher := NewJournalWatcher(nil, posStore, "default")
	err = watcher.Start(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
