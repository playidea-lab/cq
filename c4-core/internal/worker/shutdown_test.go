package worker

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestShutdownStore_Basic(t *testing.T) {
	store, err := NewShutdownStore(testDB(t))
	if err != nil {
		t.Fatal(err)
	}

	// No signal initially
	_, ok := store.ConsumeSignal("worker-1")
	if ok {
		t.Fatal("expected no signal")
	}

	// Store and consume
	if err := store.StoreSignal("worker-1", "user requested"); err != nil {
		t.Fatal(err)
	}
	reason, ok := store.ConsumeSignal("worker-1")
	if !ok {
		t.Fatal("expected signal")
	}
	if reason != "user requested" {
		t.Errorf("reason = %q, want %q", reason, "user requested")
	}

	// Already consumed — should return false
	_, ok = store.ConsumeSignal("worker-1")
	if ok {
		t.Fatal("expected signal to be consumed")
	}
}

func TestShutdownStore_Replace(t *testing.T) {
	store, err := NewShutdownStore(testDB(t))
	if err != nil {
		t.Fatal(err)
	}

	store.StoreSignal("w1", "reason-1")
	store.StoreSignal("w1", "reason-2") // replace

	reason, ok := store.ConsumeSignal("w1")
	if !ok || reason != "reason-2" {
		t.Errorf("got (%q, %v), want (reason-2, true)", reason, ok)
	}
}

func TestShutdownStore_MultipleWorkers(t *testing.T) {
	store, err := NewShutdownStore(testDB(t))
	if err != nil {
		t.Fatal(err)
	}

	store.StoreSignal("w1", "stop-1")
	store.StoreSignal("w2", "stop-2")

	r1, ok1 := store.ConsumeSignal("w1")
	r2, ok2 := store.ConsumeSignal("w2")

	if !ok1 || r1 != "stop-1" {
		t.Errorf("w1: got (%q, %v)", r1, ok1)
	}
	if !ok2 || r2 != "stop-2" {
		t.Errorf("w2: got (%q, %v)", r2, ok2)
	}
}
