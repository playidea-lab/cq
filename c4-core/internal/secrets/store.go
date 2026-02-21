// Package secrets provides AES-256-GCM encrypted secret storage backed by SQLite.
// Secrets are stored globally in ~/.c4/secrets.db.
// The master key is auto-generated at ~/.c4/master.key (0400) on first use,
// or read from the C4_MASTER_KEY env var (64 hex chars) for CI environments.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a secret key does not exist.
var ErrNotFound = errors.New("secret not found")

// Store is a thread-safe encrypted secret store backed by SQLite.
type Store struct {
	db        *sql.DB
	masterKey [32]byte
}

// GlobalDir returns the global ~/.c4 directory path.
func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".c4"), nil
}

// New opens (or creates) the global secret store at ~/.c4/secrets.db.
func New() (*Store, error) {
	dir, err := GlobalDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
}

// NewWithPaths creates a Store with explicit DB and master key paths (for testing).
func NewWithPaths(dbPath, masterKeyPath string) (*Store, error) {
	masterKey, err := loadOrCreateMasterKey(masterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("master key: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := initDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init db: %w", err)
	}

	return &Store{db: db, masterKey: masterKey}, nil
}

// Set stores (or updates) an encrypted secret.
func (s *Store) Set(key, value string) error {
	ciphertext, nonce, err := s.encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`
		INSERT INTO secrets(key, nonce, ciphertext, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			nonce=excluded.nonce,
			ciphertext=excluded.ciphertext,
			updated_at=excluded.updated_at
	`, key, nonce, ciphertext, now, now)
	return err
}

// Get retrieves and decrypts a secret. Returns ErrNotFound if key doesn't exist.
func (s *Store) Get(key string) (string, error) {
	var nonce, ciphertext []byte
	err := s.db.QueryRow(
		`SELECT nonce, ciphertext FROM secrets WHERE key=?`, key,
	).Scan(&nonce, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	plain, err := s.decrypt(ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

// List returns all secret key names (not values), sorted alphabetically.
func (s *Store) List() ([]string, error) {
	rows, err := s.db.Query(`SELECT key FROM secrets ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]string, 0)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Delete removes a secret. Returns ErrNotFound if key doesn't exist.
func (s *Store) Delete(key string) error {
	res, err := s.db.Exec(`DELETE FROM secrets WHERE key=?`, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Close releases the underlying database connection and zeros the in-memory master key.
func (s *Store) Close() error {
	for i := range s.masterKey {
		s.masterKey[i] = 0
	}
	return s.db.Close()
}

func initDB(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("WAL pragma: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return fmt.Errorf("busy_timeout pragma: %w", err)
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS secrets (
		key        TEXT    PRIMARY KEY,
		nonce      BLOB    NOT NULL,
		ciphertext BLOB    NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	return err
}

func loadOrCreateMasterKey(path string) ([32]byte, error) {
	var key [32]byte

	// CI override via env var (64 hex chars = 32 bytes).
	// Normalize to lowercase to accept both "0A0B..." and "0a0b..." forms.
	if envKey := strings.TrimSpace(os.Getenv("C4_MASTER_KEY")); envKey != "" {
		b, err := hex.DecodeString(strings.ToLower(envKey))
		if err != nil || len(b) != 32 {
			for i := range b {
				b[i] = 0
			}
			return key, fmt.Errorf("C4_MASTER_KEY must be 64 hex chars (32 bytes)")
		}
		copy(key[:], b)
		for i := range b {
			b[i] = 0
		}
		return key, nil
	}

	// Load existing key file. Open the FD first, then stat on the same descriptor
	// to avoid a TOCTOU race between the permission check and the read.
	// Distinguish ENOENT (key not yet created) from other errors (e.g. EACCES).
	f, openErr := os.Open(path)
	if openErr != nil && !errors.Is(openErr, os.ErrNotExist) {
		return key, fmt.Errorf("open master key: %w", openErr)
	}
	if openErr == nil {
		defer f.Close()
		info, statErr := f.Stat()
		if statErr != nil {
			return key, fmt.Errorf("stat master key: %w", statErr)
		}
		if info.Mode().Perm() != 0400 {
			return key, fmt.Errorf("master key file has insecure permissions %04o (expected 0400)", info.Mode().Perm())
		}
		data, readErr := io.ReadAll(io.LimitReader(f, 33)) // 33 = 32 valid + 1 to detect oversized
		if readErr != nil {
			return key, fmt.Errorf("read master key: %w", readErr)
		}
		if len(data) != 32 {
			return key, fmt.Errorf("master key file corrupt: expected 32 bytes, got %d", len(data))
		}
		copy(key[:], data)
		return key, nil
	}

	// Generate new key. Use O_EXCL to prevent overwriting an existing file
	// (double-init race) and to avoid the brief world-readable window that
	// os.WriteFile creates before chmod (create-with-mode is atomic via O_EXCL).
	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		return key, fmt.Errorf("generate: %w", err)
	}
	kf, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if err != nil {
		return key, fmt.Errorf("create master key: %w", err)
	}
	if _, writeErr := kf.Write(key[:]); writeErr != nil {
		kf.Close()
		os.Remove(path) // clean up partial file
		return key, fmt.Errorf("write master key: %w", writeErr)
	}
	if closeErr := kf.Close(); closeErr != nil {
		os.Remove(path) // clean up on close failure
		return key, fmt.Errorf("close master key: %w", closeErr)
	}
	return key, nil
}

func (s *Store) encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(s.masterKey[:])
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (s *Store) decrypt(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.masterKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
