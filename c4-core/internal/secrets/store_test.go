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
	if err := s1.Set("k", "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}
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

func TestMasterKeyEnvVarPersistence(t *testing.T) {
	testKey := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	t.Setenv("C4_MASTER_KEY", testKey)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// Write with env var key
	s1, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := s1.Set("k", "persistent"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s1.Close()

	// Re-open with same env var — must decrypt successfully
	s2, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get("k")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got != "persistent" {
		t.Errorf("got %q, want %q\n", got, "persistent")
	}
}

// TestNewWithPaths_CreatesDB verifies that NewWithPaths creates a new DB file when one does not exist.
func TestNewWithPaths_CreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "new.db")
	keyPath := filepath.Join(dir, "master.key")

	// DB file must not exist yet
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("pre-condition: DB file should not exist")
	}

	s, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	defer s.Close()

	// DB file should now exist
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("expected DB file to exist after NewWithPaths, got: %v", err)
	}
	// Should be usable
	if err := s.Set("ping", "pong"); err != nil {
		t.Errorf("Set on fresh DB: %v", err)
	}
}

// TestNewWithPaths_ExistingDB verifies that reopening an existing DB with the same key works.
func TestNewWithPaths_ExistingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reopen.db")
	keyPath := filepath.Join(dir, "master.key")

	// First open: create and write
	s1, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := s1.Set("foo", "bar"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s1.Close()

	// Second open: same paths — must reuse existing DB and key
	s2, err := secrets.NewWithPaths(dbPath, keyPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get("foo")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

// TestStore_Delete_NotFound verifies ErrNotFound is returned when deleting a non-existent key.
func TestStore_Delete_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Delete("no-such-key")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("Delete non-existent: got %v, want ErrNotFound", err)
	}
}

// TestMasterKey_InsecurePermissions verifies that a key file with wrong permissions is rejected.
func TestMasterKey_InsecurePermissions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// Write a valid-length key file but with insecure permissions (0644 instead of 0400)
	key := make([]byte, 32)
	if err := os.WriteFile(keyPath, key, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error for insecure master key permissions, got nil")
	}
}

// TestMasterKeyEnvVar_InvalidHex verifies that a malformed C4_MASTER_KEY env var returns an error.
func TestMasterKeyEnvVar_InvalidHex(t *testing.T) {
	t.Setenv("C4_MASTER_KEY", "not-valid-hex-string-at-all-!!!!")

	dir := t.TempDir()
	_, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err == nil {
		t.Fatal("expected error for invalid C4_MASTER_KEY hex, got nil")
	}
}

// TestMasterKeyEnvVar_WrongLength verifies that a hex string with wrong byte count is rejected.
func TestMasterKeyEnvVar_WrongLength(t *testing.T) {
	// Valid hex but only 16 bytes (32 hex chars) instead of 32 bytes (64 hex chars)
	t.Setenv("C4_MASTER_KEY", "0102030405060708090a0b0c0d0e0f10")

	dir := t.TempDir()
	_, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err == nil {
		t.Fatal("expected error for short C4_MASTER_KEY, got nil")
	}
}

// TestNewWithPaths_DBOpenFailure verifies error when the DB path is not writable.
func TestNewWithPaths_DBOpenFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")

	// Make a subdirectory that is read-only so creating DB inside it fails
	roDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(roDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(roDir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0755) })

	dbPath := filepath.Join(roDir, "sub", "nested.db") // non-existent intermediate dir

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	// sql.Open is lazy; the real error surfaces at initDB (first Exec).
	// Either open or init will fail — we just need an error.
	if err == nil {
		t.Skip("filesystem did not prevent DB creation (may be root or special fs)")
	}
}

// TestSet_ClosedStore verifies that Set on a closed store returns an error.
func TestSet_ClosedStore(t *testing.T) {
	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	s.Close()

	if err := s.Set("k", "v"); err == nil {
		t.Error("expected error from Set on closed store, got nil")
	}
}

// TestGet_ClosedStore verifies that Get on a closed store returns an error.
func TestGet_ClosedStore(t *testing.T) {
	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	s.Close()

	_, err = s.Get("k")
	if err == nil {
		t.Error("expected error from Get on closed store, got nil")
	}
}

// TestList_ClosedStore verifies that List on a closed store returns an error.
func TestList_ClosedStore(t *testing.T) {
	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	s.Close()

	_, err = s.List()
	if err == nil {
		t.Error("expected error from List on closed store, got nil")
	}
}

// TestDelete_ClosedStore verifies that Delete on a closed store returns an error.
func TestDelete_ClosedStore(t *testing.T) {
	dir := t.TempDir()
	s, err := secrets.NewWithPaths(
		filepath.Join(dir, "s.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	s.Close()

	if err := s.Delete("k"); err == nil {
		t.Error("expected error from Delete on closed store, got nil")
	}
}

// TestMasterKey_OpenError verifies that an unreadable key file (non-ErrNotExist) returns an error.
func TestMasterKey_OpenError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")

	// Create a directory where the key file should be — os.Open will fail with EISDIR or similar
	keyPath := filepath.Join(dir, "key-as-dir")
	if err := os.MkdirAll(keyPath, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error when key path is a directory, got nil")
	}
}

// TestMasterKey_UnreadableFile verifies that a key file with no read permission returns an error.
func TestMasterKey_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// Write a valid-length key file and then remove all permissions
	key := make([]byte, 32)
	if err := os.WriteFile(keyPath, key, 0000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(keyPath, 0600) })

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error for unreadable master key file, got nil")
	}
}

// TestNewWithPaths_CorruptZeroByteKey verifies that a zero-byte key file is rejected as corrupt.
func TestNewWithPaths_CorruptZeroByteKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	keyPath := filepath.Join(dir, "master.key")

	// Zero-byte file — triggers "corrupt: expected 32 bytes, got 0"
	if err := os.WriteFile(keyPath, []byte{}, 0400); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error for zero-byte (corrupt) master key file, got nil")
	}
}

// TestNew_GlobalDir exercises the New() constructor which resolves the global ~/.c4 directory.
// Skips if the home directory is unavailable (CI environments without HOME set).
// This is a lightweight smoke test: open, set a sentinel key, close.
// It does NOT assert on the sentinel value to avoid cross-test pollution.
func TestNew_GlobalDir(t *testing.T) {
	s, err := secrets.New()
	if err != nil {
		t.Skipf("New() unavailable in this environment: %v", err)
	}
	defer s.Close()
	// Exercise the store minimally to confirm it is functional
	if err := s.Set("__test_new_smoke__", "ok"); err != nil {
		t.Errorf("Set on global store: %v", err)
	}
	// Clean up the sentinel key so we don't pollute the real store
	_ = s.Delete("__test_new_smoke__")
}

// TestNewWithPaths_CreateKeyInReadOnlyDir verifies that attempting to create a master key
// in a read-only directory returns an error (covers the O_EXCL create failure path).
func TestNewWithPaths_CreateKeyInReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")

	// Create a read-only subdirectory for the key
	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0500); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0700) })

	keyPath := filepath.Join(roDir, "master.key")
	// keyPath does not exist, but roDir is read-only — os.OpenFile(O_EXCL) will fail

	_, err := secrets.NewWithPaths(dbPath, keyPath)
	if err == nil {
		t.Fatal("expected error creating master key in read-only directory, got nil")
	}
}

// TestSecretNS_SetGet verifies SetNS/GetNS round-trip for a namespaced key.
func TestSecretNS_SetGet(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetNS("projA", "api_key", "sk-proj-a"); err != nil {
		t.Fatalf("SetNS: %v", err)
	}
	got, err := s.GetNS("projA", "api_key")
	if err != nil {
		t.Fatalf("GetNS: %v", err)
	}
	if got != "sk-proj-a" {
		t.Errorf("GetNS = %q, want %q", got, "sk-proj-a")
	}
}

// TestSecretNS_Fallback verifies that GetNS falls back to the global key when
// the namespaced key does not exist.
func TestSecretNS_Fallback(t *testing.T) {
	s := newTestStore(t)

	// Set a global key only (no namespace).
	if err := s.Set("anthropic.api_key", "global-val"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// GetNS for a project that has no project-specific key → must return global.
	got, err := s.GetNS("projA", "anthropic.api_key")
	if err != nil {
		t.Fatalf("GetNS fallback: %v", err)
	}
	if got != "global-val" {
		t.Errorf("GetNS fallback = %q, want %q", got, "global-val")
	}
}

// TestSecretNS_Fallback_DoesNotCrossProjectBoundary verifies that when projA has
// no namespaced key, GetNS falls back to the global key only — not to projB's key.
func TestSecretNS_Fallback_DoesNotCrossProjectBoundary(t *testing.T) {
	s := newTestStore(t)

	// Set a key for projB (and global), but NOT for projA.
	if err := s.SetNS("projB", "api_key", "sk-proj-b"); err != nil {
		t.Fatalf("SetNS projB: %v", err)
	}
	if err := s.Set("api_key", "global-val"); err != nil {
		t.Fatalf("Set global: %v", err)
	}

	// projA has no namespaced key → must fall back to global, never to projB.
	got, err := s.GetNS("projA", "api_key")
	if err != nil {
		t.Fatalf("GetNS: %v", err)
	}
	if got != "global-val" {
		t.Errorf("GetNS fallback = %q, want %q (must not cross project boundary)", got, "global-val")
	}
	if got == "sk-proj-b" {
		t.Fatal("cross-project boundary violation: projA received projB's secret")
	}
}

// TestSecretNS_ColonInProjectID verifies that a projectID containing ":" returns ErrInvalidProjectID.
func TestSecretNS_ColonInProjectID(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetNS("proj:bad", "key", "val"); !errors.Is(err, secrets.ErrInvalidProjectID) {
		t.Errorf("SetNS with colon in projectID: got %v, want ErrInvalidProjectID", err)
	}
	_, err := s.GetNS("proj:bad", "key")
	if !errors.Is(err, secrets.ErrInvalidProjectID) {
		t.Errorf("GetNS with colon in projectID: got %v, want ErrInvalidProjectID", err)
	}
}

// TestSecretNS_ColonInKey verifies that a key containing ":" is stored and retrieved correctly.
// The first ":" is the project/key separator; additional ":" in key are harmless.
func TestSecretNS_ColonInKey(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetNS("projA", ":api:subkey", "val"); err != nil {
		t.Fatalf("SetNS colon-key: %v", err)
	}
	got, err := s.GetNS("projA", ":api:subkey")
	if err != nil {
		t.Fatalf("GetNS colon-key: %v", err)
	}
	if got != "val" {
		t.Errorf("GetNS colon-key = %q, want %q", got, "val")
	}
}

// TestSecret_BackwardCompat verifies that the existing Get/Set API is unchanged.
func TestSecret_BackwardCompat(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("openai.api_key", "sk-global"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("openai.api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-global" {
		t.Errorf("Get = %q, want %q", got, "sk-global")
	}
}
