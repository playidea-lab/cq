package secrets_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/secrets"
)

func newTestStore(t *testing.T) *secrets.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSetGet(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("openai.api_key", "sk-test-123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("openai.api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-test-123" {
		t.Errorf("Get = %q, want %q", got, "sk-test-123")
	}
}

func TestSetOverwrite(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("key", "v1"); err != nil {
		t.Fatalf("Set v1: %v", err)
	}
	if err := s.Set("key", "v2"); err != nil {
		t.Fatalf("Set v2: %v", err)
	}

	got, _ := s.Get("key")
	if got != "v2" {
		t.Errorf("overwrite: got %q, want %q", got, "v2")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("nonexistent")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("b.key", "v"); err != nil {
		t.Fatalf("Set b.key: %v", err)
	}
	if err := s.Set("a.key", "v"); err != nil {
		t.Fatalf("Set a.key: %v", err)
	}
	if err := s.Set("c.key", "v"); err != nil {
		t.Fatalf("Set c.key: %v", err)
	}

	keys, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("len = %d, want 3", len(keys))
	}
	// Must be sorted
	if keys[0] != "a.key" || keys[1] != "b.key" || keys[2] != "c.key" {
		t.Errorf("not sorted: %v", keys)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("k"); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("second delete: expected ErrNotFound, got %v", err)
	}
}

func TestMasterKeyFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// First open generates key file
	s1, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Set("k", "hello")
	s1.Close()

	// Check file permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat master.key: %v", err)
	}
	if info.Mode().Perm() != 0400 {
		t.Errorf("master.key perm = %04o, want 0400", info.Mode().Perm())
	}

	// Second open reuses same key → can decrypt
	s2, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get("k")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestCorruptMasterKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// Write a key file with wrong size (corrupt)
	if err := os.WriteFile(keyPath, []byte("tooshort"), 0400); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error for corrupt master key, got nil")
	}
}

func TestWrongKeyDecryptionFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	key1Path := filepath.Join(dir, "key1.key")
	key2Path := filepath.Join(dir, "key2.key")

	// Write secret with key1
	s1, err := secrets.NewWithPaths(dbPath, key1Path)
	if err != nil {
		t.Fatalf("open with key1: %v", err)
	}
	if err := s1.Set("k", "secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s1.Close()

	// Attempt to read with key2 (different auto-generated key)
	s2, err := secrets.NewWithPaths(dbPath, key2Path)
	if err != nil {
		t.Fatalf("open with key2: %v", err)
	}
	defer s2.Close()

	_, err = s2.Get("k")
	if err == nil {
		t.Fatal("expected decryption error with wrong key, got nil")
	}
}

func TestMasterKeyEnvVar(t *testing.T) {
	// 32 bytes = 64 hex chars
	testKey := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	t.Setenv("C4_MASTER_KEY", testKey)

	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"), // won't be created
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	defer s.Close()

	s.Set("k", "val")
	got, _ := s.Get("k")
	if got != "val" {
		t.Errorf("got %q, want val", got)
	}

	// master.key file should NOT be created when env var is set
	if _, err := os.Stat(filepath.Join(dir, "master.key")); !os.IsNotExist(err) {
		t.Error("master.key should not be created when C4_MASTER_KEY env is set")
	}
}
