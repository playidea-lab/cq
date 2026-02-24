package mailbox

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *MailStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewMailStore(filepath.Join(dir, "mailbox.db"))
	if err != nil {
		t.Fatalf("NewMailStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMailSend(t *testing.T) {
	s := newTestStore(t)

	id, _, err := s.Send("alice", "bob", "hello", "world", "proj1")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	// from="*" should be rejected
	_, _, err = s.Send("*", "bob", "hi", "body", "")
	if err == nil {
		t.Fatal("expected error for from='*', got nil")
	}

	// to="" should be rejected
	_, _, err = s.Send("alice", "", "hi", "body", "")
	if err == nil {
		t.Fatal("expected error for empty to, got nil")
	}
}

func TestUnreadCount(t *testing.T) {
	s := newTestStore(t)

	// Send direct message to bob
	if _, _, err := s.Send("alice", "bob", "subj1", "body1", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Send broadcast
	if _, _, err := s.Send("alice", "*", "broadcast", "hello everyone", ""); err != nil {
		t.Fatalf("Send broadcast: %v", err)
	}
	// Send to carol (should not count for bob)
	if _, _, err := s.Send("alice", "carol", "subj2", "body2", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}

	count, err := s.UnreadCount("bob")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	// bob has 1 direct + 1 broadcast = 2
	if count != 2 {
		t.Fatalf("expected 2 unread for bob, got %d", count)
	}

	count, err = s.UnreadCount("carol")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	// carol has 1 direct + 1 broadcast = 2
	if count != 2 {
		t.Fatalf("expected 2 unread for carol, got %d", count)
	}

	count, err = s.UnreadCount("dave")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	// dave has only the broadcast = 1
	if count != 1 {
		t.Fatalf("expected 1 unread for dave, got %d", count)
	}
}

func TestMailStoreClose(t *testing.T) {
	dir := t.TempDir()
	s, err := NewMailStore(filepath.Join(dir, "mailbox.db"))
	if err != nil {
		t.Fatalf("NewMailStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close again should not panic (may return error, which is acceptable)
	_ = s.Close()
}

// TestMailSendAndMigrationIdempotent reopens the same DB and verifies data persists.
func TestMailSendAndMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mailbox.db")

	s1, err := NewMailStore(dbPath)
	if err != nil {
		t.Fatalf("NewMailStore (1st): %v", err)
	}
	if _, _, err := s1.Send("a", "b", "s", "body", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	s1.Close()

	// Reopen — migrate() must be idempotent
	s2, err := NewMailStore(dbPath)
	if err != nil {
		t.Fatalf("NewMailStore (2nd): %v", err)
	}
	defer s2.Close()

	count, err := s2.UnreadCount("b")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 unread after reopen, got %d", count)
	}
}

func TestMailList(t *testing.T) {
	s := newTestStore(t)

	// Send messages to bob, broadcast, and carol
	if _, _, err := s.Send("alice", "bob", "direct", "body1", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, _, err := s.Send("alice", "*", "broadcast", "hello", ""); err != nil {
		t.Fatalf("Send broadcast: %v", err)
	}
	if _, _, err := s.Send("alice", "carol", "to carol", "body3", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// bob should see 2 messages: direct + broadcast
	msgs, err := s.List("bob", false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for bob, got %d", len(msgs))
	}

	// admin view: toSession="" returns all 3
	msgs, err = s.List("", false)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 total messages, got %d", len(msgs))
	}

	// mark one message read then check unreadOnly filter
	id := msgs[0].ID
	if _, err := s.Read(id); err != nil {
		t.Fatalf("Read: %v", err)
	}
	msgs, err = s.List("", true)
	if err != nil {
		t.Fatalf("List unreadOnly: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 unread after marking one read, got %d", len(msgs))
	}
}

func TestMailRead(t *testing.T) {
	s := newTestStore(t)

	id, _, err := s.Send("alice", "bob", "subj", "body", "proj")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg, err := s.Read(id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if msg.ID != id {
		t.Fatalf("expected id %d, got %d", id, msg.ID)
	}
	if msg.ReadAt == "" {
		t.Fatal("expected read_at to be set after Read")
	}
	if msg.From != "alice" || msg.To != "bob" || msg.Subject != "subj" {
		t.Fatalf("unexpected message fields: %+v", msg)
	}
}

func TestMailReadIdempotent(t *testing.T) {
	s := newTestStore(t)

	id, _, err := s.Send("alice", "bob", "subj", "body", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg1, err := s.Read(id)
	if err != nil {
		t.Fatalf("Read (1st): %v", err)
	}
	msg2, err := s.Read(id)
	if err != nil {
		t.Fatalf("Read (2nd): %v", err)
	}
	// read_at must be identical across repeated reads
	if msg1.ReadAt != msg2.ReadAt {
		t.Fatalf("read_at changed on second read: %q vs %q", msg1.ReadAt, msg2.ReadAt)
	}
}

func TestMailReadNotFound(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.Read(9999)
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if msg != nil {
		t.Fatalf("expected nil message, got %+v", msg)
	}
}

func TestMailDelete(t *testing.T) {
	s := newTestStore(t)

	id, _, err := s.Send("alice", "bob", "subj", "body", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Delete existing message
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Confirm it's gone
	_, err = s.Read(id)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete non-existent message → ErrNotFound
	if err := s.Delete(id); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for second delete, got %v", err)
	}
}

// Ensure the test file itself compiles (os import used for tempdir in CI).
var _ = os.DevNull
