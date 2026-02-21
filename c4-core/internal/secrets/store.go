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

	var keys []string
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

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func initDB(db *sql.DB) error {
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

	// CI override via env var (64 hex chars = 32 bytes)
	if envKey := os.Getenv("C4_MASTER_KEY"); envKey != "" {
		b, err := hex.DecodeString(envKey)
		if err != nil || len(b) != 32 {
			return key, fmt.Errorf("C4_MASTER_KEY must be 64 hex chars (32 bytes)")
		}
		copy(key[:], b)
		return key, nil
	}

	// Load existing key file
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return key, fmt.Errorf("master key file corrupt: expected 32 bytes, got %d", len(data))
		}
		copy(key[:], data)
		return key, nil
	}

	// Generate new key
	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		return key, fmt.Errorf("generate: %w", err)
	}
	if err := os.WriteFile(path, key[:], 0400); err != nil {
		return key, fmt.Errorf("write master key: %w", err)
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
