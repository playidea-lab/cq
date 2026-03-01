package handlers

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newScopeLockStore creates a fresh in-memory SQLiteStore for scope lock tests.
func newScopeLockStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// TestScopeLock_SameScope_SecondFails verifies that when Worker1 holds a scope lock,
// Worker2 for the same scope cannot acquire it.
func TestScopeLock_SameScope_SecondFails(t *testing.T) {
	s := newScopeLockStore(t)

	scope := "c4-core/internal/mcp/handlers/sqlite_store.go"
	ttl := 10 * time.Minute

	// Worker1 acquires the lock.
	ok1, err := s.TryAcquireScopeLock(scope, "worker-1", ttl)
	if err != nil {
		t.Fatalf("worker-1 acquire: %v", err)
	}
	if !ok1 {
		t.Fatal("worker-1 should have acquired the lock")
	}

	// Worker2 on the same scope should fail.
	ok2, err := s.TryAcquireScopeLock(scope, "worker-2", ttl)
	if err != nil {
		t.Fatalf("worker-2 acquire: %v", err)
	}
	if ok2 {
		t.Fatal("worker-2 should NOT have acquired the lock (worker-1 holds it)")
	}
}

// TestScopeLock_DifferentScope_BothSucceed verifies that two workers with
// different scopes can both acquire their respective locks simultaneously.
func TestScopeLock_DifferentScope_BothSucceed(t *testing.T) {
	s := newScopeLockStore(t)

	ttl := 10 * time.Minute

	ok1, err := s.TryAcquireScopeLock("file-a.go", "worker-1", ttl)
	if err != nil {
		t.Fatalf("worker-1 acquire: %v", err)
	}
	if !ok1 {
		t.Fatal("worker-1 should have acquired lock for file-a.go")
	}

	ok2, err := s.TryAcquireScopeLock("file-b.go", "worker-2", ttl)
	if err != nil {
		t.Fatalf("worker-2 acquire: %v", err)
	}
	if !ok2 {
		t.Fatal("worker-2 should have acquired lock for file-b.go")
	}
}

// TestScopeLock_Stale_AutoEvicted verifies that a stale lock (TTL=0) is evicted
// and the next worker can acquire the lock.
func TestScopeLock_Stale_AutoEvicted(t *testing.T) {
	s := newScopeLockStore(t)

	scope := "c4-core/internal/store/sqlite.go"

	// Worker1 acquires a lock that expires immediately (TTL=-1s so it's already stale).
	ok1, err := s.TryAcquireScopeLock(scope, "worker-stale", -time.Second)
	if err != nil {
		t.Fatalf("stale worker acquire: %v", err)
	}
	if !ok1 {
		t.Fatal("stale worker should have acquired the lock initially")
	}

	// Worker2 attempts to acquire; the stale lock should be evicted automatically.
	ok2, err := s.TryAcquireScopeLock(scope, "worker-new", 10*time.Minute)
	if err != nil {
		t.Fatalf("new worker acquire: %v", err)
	}
	if !ok2 {
		t.Fatal("new worker should have acquired the lock after stale eviction")
	}
}

// TestScopeLock_EmptyScope_AlwaysSucceeds verifies that scope="" is a no-op
// (always returns true without touching the DB).
func TestScopeLock_EmptyScope_AlwaysSucceeds(t *testing.T) {
	s := newScopeLockStore(t)

	for i := 0; i < 3; i++ {
		ok, err := s.TryAcquireScopeLock("", "worker-any", 10*time.Minute)
		if err != nil {
			t.Fatalf("empty scope acquire attempt %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("empty scope attempt %d: should always return true", i)
		}
	}

	// Release is also a no-op.
	if err := s.ReleaseScopeLock("", "worker-any"); err != nil {
		t.Fatalf("release empty scope: %v", err)
	}
}
