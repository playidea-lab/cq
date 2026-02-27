package mailhandler_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mailbox"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/mailhandler"
)

func setupMailTest(t *testing.T) (*mcp.Registry, *mailbox.MailStore) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mailbox.db")
	ms, err := mailbox.NewMailStore(dbPath)
	if err != nil {
		t.Fatalf("NewMailStore: %v", err)
	}
	t.Cleanup(func() { ms.Close() })

	reg := mcp.NewRegistry()
	mailhandler.Register(reg, ms)
	return reg, ms
}

func callMailHandler(t *testing.T, reg *mcp.Registry, tool string, args map[string]any) (any, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return reg.Call(tool, json.RawMessage(raw))
}

// toInt64 converts an int64 or float64 map value to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

// TestMailSendDoesNotMarkRead is the regression test verifying that c4_mail_send
// does NOT mark the message as read — a bug present in the original T-MAIL-002-0
// implementation that called ms.Read(id) to fetch created_at after insert.
func TestMailSendDoesNotMarkRead(t *testing.T) {
	reg, ms := setupMailTest(t)

	// Send a message.
	result, err := callMailHandler(t, reg, "c4_mail_send", map[string]any{
		"to":      "session-alice",
		"body":    "hello",
		"subject": "test",
		"from":    "sender",
	})
	if err != nil {
		t.Fatalf("c4_mail_send: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["id"] == nil {
		t.Fatal("expected id in send response")
	}
	if m["created_at"] == nil || m["created_at"] == "" {
		t.Fatal("expected created_at in send response")
	}

	// Regression check 1: c4_mail_ls(unread_only=true) must see the new message.
	lsResult, err := callMailHandler(t, reg, "c4_mail_ls", map[string]any{
		"session":     "session-alice",
		"unread_only": true,
	})
	if err != nil {
		t.Fatalf("c4_mail_ls: %v", err)
	}
	rawLs, _ := json.Marshal(lsResult)
	var lsSlice []map[string]any
	if err := json.Unmarshal(rawLs, &lsSlice); err != nil {
		t.Fatalf("unmarshal ls result: %v", err)
	}
	if len(lsSlice) == 0 {
		t.Error("regression: c4_mail_ls(unread_only=true) returned 0 messages after send; send must NOT call Read()")
	}

	// Regression check 2: UnreadCount must be > 0.
	count, err := ms.UnreadCount("session-alice")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count == 0 {
		t.Error("regression: UnreadCount == 0 right after send; send must NOT mark message as read")
	}
}

func TestMailSendAndRead(t *testing.T) {
	reg, _ := setupMailTest(t)

	// Send.
	sendResult, err := callMailHandler(t, reg, "c4_mail_send", map[string]any{
		"to":      "bob",
		"subject": "greet",
		"body":    "world",
		"from":    "alice",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	sm, ok := sendResult.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", sendResult)
	}
	id, ok := toInt64(sm["id"])
	if !ok || id == 0 {
		t.Fatalf("no valid id in send response; got %v (%T)", sm["id"], sm["id"])
	}
	if sm["created_at"] == nil || sm["created_at"] == "" {
		t.Fatal("no created_at in send response")
	}

	// Read — should return full body and set read_at.
	readResult, err := callMailHandler(t, reg, "c4_mail_read", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	rm, ok := readResult.(map[string]any)
	if !ok {
		t.Fatalf("expected map from read, got %T", readResult)
	}
	if rm["body"] != "world" {
		t.Errorf("expected body 'world', got %v", rm["body"])
	}
	if rm["read_at"] == "" || rm["read_at"] == nil {
		t.Error("expected read_at to be set after c4_mail_read")
	}
}

func TestMailLsAll(t *testing.T) {
	reg, _ := setupMailTest(t)

	// Use unique session names and filter explicitly to avoid CQ_SESSION_NAME interference.
	const sessA = "test-mail-ls-a"
	const sessB = "test-mail-ls-b"
	callMailHandler(t, reg, "c4_mail_send", map[string]any{"to": sessA, "body": "msg1", "from": "x"})
	callMailHandler(t, reg, "c4_mail_send", map[string]any{"to": sessB, "body": "msg2", "from": "x"})

	// List for sessA only — should return 1.
	result, err := callMailHandler(t, reg, "c4_mail_ls", map[string]any{"session": sessA})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	raw, _ := json.Marshal(result)
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 message for %s, got %d", sessA, len(items))
	}
}

func TestMailRm(t *testing.T) {
	reg, _ := setupMailTest(t)

	sendResult, err := callMailHandler(t, reg, "c4_mail_send", map[string]any{"to": "x", "body": "bye", "from": "y"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	sm, _ := sendResult.(map[string]any)
	id, ok := toInt64(sm["id"])
	if !ok || id == 0 {
		t.Fatalf("no valid id in send response; sm=%v", sm)
	}

	rmResult, err := callMailHandler(t, reg, "c4_mail_rm", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	rm, ok := rmResult.(map[string]any)
	if !ok {
		t.Fatalf("expected map from rm, got %T", rmResult)
	}
	if rm["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", rm["deleted"])
	}

	// Verify gone.
	_, err = callMailHandler(t, reg, "c4_mail_read", map[string]any{"id": id})
	if err == nil {
		t.Error("expected error reading deleted message")
	}
}

func TestMailSendFromStar(t *testing.T) {
	reg, _ := setupMailTest(t)

	_, err := callMailHandler(t, reg, "c4_mail_send", map[string]any{
		"to":   "someone",
		"body": "test",
		"from": "*",
	})
	if err == nil {
		t.Error("expected error when from='*'")
	}
}

func TestMailSendEmptyTo(t *testing.T) {
	reg, _ := setupMailTest(t)

	_, err := callMailHandler(t, reg, "c4_mail_send", map[string]any{
		"to":   "",
		"body": "test",
	})
	if err == nil {
		t.Error("expected error when to=''")
	}
}

func TestMailReadNotFound(t *testing.T) {
	reg, _ := setupMailTest(t)

	_, err := callMailHandler(t, reg, "c4_mail_read", map[string]any{"id": int64(999)})
	if err == nil {
		t.Error("expected error for non-existent message ID")
	}
}

func TestMailRmNotFound(t *testing.T) {
	reg, _ := setupMailTest(t)

	_, err := callMailHandler(t, reg, "c4_mail_rm", map[string]any{"id": int64(999)})
	if err == nil {
		t.Error("expected error for non-existent message ID")
	}
}
