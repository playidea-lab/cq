package harness

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/changmin/c4-core/internal/c1push"
)

// newTempCursorAdapter creates a CursorAdapter wired to a temp proc DB and
// a fake Cursor DB at the given path (which may not exist yet).
func newTempCursorAdapter(t *testing.T, dbPath string, pusher ChannelPusher) *CursorAdapter {
	t.Helper()
	procDBPath := filepath.Join(t.TempDir(), "cursor_proc.db")
	procDB, err := openProcDB(procDBPath)
	if err != nil {
		t.Fatalf("openProcDB: %v", err)
	}
	t.Cleanup(func() { procDB.Close() })
	return &CursorAdapter{
		pusher:   pusher,
		dbPath:   dbPath,
		procDB:   procDB,
		done:     make(chan struct{}),
		tenantID: "default",
	}
}

// createFakeCursorDB creates a minimal state.vscdb with cursorDiskKV table.
func createFakeCursorDB(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "state.vscdb")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fake cursor db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return dbPath
}

// insertBubble inserts a composer metadata key and a bubble into the fake DB.
func insertBubble(t *testing.T, dbPath, composerID, bubbleID string, btype int, text string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	composerKey := "composerData:" + composerID
	composerVal := `{"composerID":"` + composerID + `"}`
	_, err = db.Exec(`INSERT OR REPLACE INTO cursorDiskKV (key, value) VALUES (?, ?)`, composerKey, composerVal)
	if err != nil {
		t.Fatalf("insert composerData: %v", err)
	}

	bubble := map[string]interface{}{
		"type":   btype,
		"text":   text,
		"unixMs": 1000,
	}
	val, _ := json.Marshal(bubble)
	bubbleKey := "bubbleId:" + composerID + ":" + bubbleID
	_, err = db.Exec(`INSERT OR REPLACE INTO cursorDiskKV (key, value) VALUES (?, ?)`, bubbleKey, string(val))
	if err != nil {
		t.Fatalf("insert bubble: %v", err)
	}
}

// TestCursorAdapter_NoDBFile verifies Start returns nil when DB doesn't exist.
func TestCursorAdapter_NoDBFile(t *testing.T) {
	mock := &mockPusher{}
	a := newTempCursorAdapter(t, "/nonexistent/path/state.vscdb", mock)
	err := a.Start(context.Background())
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestParseCursorBubble_User verifies type=1 maps to role "user".
func TestParseCursorBubble_User(t *testing.T) {
	val := `{"type":1,"text":"hello","unixMs":1000}`
	msg := parseCursorBubble(val)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.SenderType != "user" {
		t.Errorf("SenderType=%q, want %q", msg.SenderType, "user")
	}
	if msg.Content != "hello" {
		t.Errorf("Content=%q, want %q", msg.Content, "hello")
	}
}

// TestParseCursorBubble_Assistant verifies type=2 maps to role "assistant".
func TestParseCursorBubble_Assistant(t *testing.T) {
	val := `{"type":2,"text":"world","unixMs":2000}`
	msg := parseCursorBubble(val)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.SenderType != "assistant" {
		t.Errorf("SenderType=%q, want %q", msg.SenderType, "assistant")
	}
	if msg.Content != "world" {
		t.Errorf("Content=%q, want %q", msg.Content, "world")
	}
}

// TestParseCursorBubble_Unknown verifies type=3 returns nil.
func TestParseCursorBubble_Unknown(t *testing.T) {
	val := `{"type":3,"text":"ignored","unixMs":3000}`
	msg := parseCursorBubble(val)
	if msg != nil {
		t.Errorf("expected nil for unknown type, got %+v", msg)
	}
}

// TestCursorAdapter_SyncPushesMessages verifies full sync flow.
func TestCursorAdapter_SyncPushesMessages(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createFakeCursorDB(t, tmpDir)

	composerID := "composer-abc"
	insertBubble(t, dbPath, composerID, "b001", 1, "Hi Cursor")
	insertBubble(t, dbPath, composerID, "b002", 2, "Hello from assistant")

	mock := &mockPusher{}
	a := newTempCursorAdapter(t, dbPath, mock)

	ctx := context.Background()
	a.sync(ctx)

	if len(mock.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(mock.messages))
	}
	if mock.messages[0].SenderType != "user" {
		t.Errorf("msg[0] SenderType=%q, want user", mock.messages[0].SenderType)
	}
	if mock.messages[1].SenderType != "assistant" {
		t.Errorf("msg[1] SenderType=%q, want assistant", mock.messages[1].SenderType)
	}

	// Verify channel name.
	if len(mock.channels) == 0 || mock.channels[0] != "cursor:"+composerID {
		t.Errorf("channel=%v, want [cursor:%s]", mock.channels, composerID)
	}
}

// TestCursorAdapter_DeduplicatesComposer verifies processed composers are skipped.
func TestCursorAdapter_DeduplicatesComposer(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createFakeCursorDB(t, tmpDir)

	composerID := "composer-dup"
	insertBubble(t, dbPath, composerID, "b001", 1, "First message")

	mock := &mockPusher{}
	a := newTempCursorAdapter(t, dbPath, mock)

	ctx := context.Background()
	a.sync(ctx)
	if len(mock.messages) != 1 {
		t.Fatalf("first sync: expected 1 message, got %d", len(mock.messages))
	}

	// Second sync should skip the already-processed composer.
	mock.messages = nil
	mock.channels = nil
	a.sync(ctx)
	if len(mock.messages) != 0 {
		t.Errorf("second sync: expected 0 messages (dedup), got %d", len(mock.messages))
	}
}

// TestCursorAdapter_SkipsUnknownBubbles verifies type=3 bubbles are not pushed.
func TestCursorAdapter_SkipsUnknownBubbles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createFakeCursorDB(t, tmpDir)

	composerID := "composer-unknown"
	insertBubble(t, dbPath, composerID, "b001", 3, "Should be ignored")
	insertBubble(t, dbPath, composerID, "b002", 1, "Real message")

	mock := &mockPusher{}
	a := newTempCursorAdapter(t, dbPath, mock)

	ctx := context.Background()
	a.sync(ctx)

	// Only 1 valid message (type=1), type=3 is skipped.
	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}
	if mock.messages[0].Content != "Real message" {
		t.Errorf("Content=%q, want %q", mock.messages[0].Content, "Real message")
	}
}

// TestCursorAdapter_PlatformCursor verifies correct platform is used.
func TestCursorAdapter_PlatformCursor(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createFakeCursorDB(t, tmpDir)

	composerID := "composer-plat"
	insertBubble(t, dbPath, composerID, "b001", 1, "Platform check")

	var capturedPlatform c1push.Platform
	capture := &capturePlatformPusher{platform: &capturedPlatform}
	a := newTempCursorAdapter(t, dbPath, capture)

	ctx := context.Background()
	a.sync(ctx)

	if capturedPlatform != c1push.PlatformCursor {
		t.Errorf("platform=%q, want %q", capturedPlatform, c1push.PlatformCursor)
	}
}

// capturePlatformPusher records the platform used in EnsureChannel.
type capturePlatformPusher struct {
	platform *c1push.Platform
}

func (p *capturePlatformPusher) EnsureChannel(_ context.Context, _, _, _ string, platform c1push.Platform) (string, error) {
	*p.platform = platform
	return "chan-id", nil
}

func (p *capturePlatformPusher) AppendMessages(_ context.Context, _ string, _ []c1push.PushMessage) error {
	return nil
}
