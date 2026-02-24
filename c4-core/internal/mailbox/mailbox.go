// Package mailbox provides a simple session-to-session mail store backed by SQLite.
// The database lives at ~/.c4/mailbox.db (global — shared across projects).
package mailbox

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested mail message does not exist.
var ErrNotFound = errors.New("mail: not found")

// MailMessage represents a single mail message.
type MailMessage struct {
	ID        int64
	From      string // "" = anonymous (allowed when CQ_SESSION_NAME is not set)
	To        string // "*" = broadcast
	Subject   string
	Body      string
	ProjectID string
	CreatedAt string // RFC3339
	ReadAt    string // "" = unread
}

// MailStore is a SQLite-backed mail store.
type MailStore struct {
	db *sql.DB
}

// NewMailStore opens (or creates) the mailbox database at dbPath, runs the
// migration, and returns a ready-to-use *MailStore.
func NewMailStore(dbPath string) (*MailStore, error) {
	// Ensure parent directory exists (e.g. ~/.c4/ on a fresh machine).
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("mailbox: create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("mailbox: open db: %w", err)
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("mailbox: set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("mailbox: set busy_timeout: %w", err)
	}

	s := &MailStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mailbox: migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *MailStore) Close() error {
	return s.db.Close()
}

// migrate creates the c4_mail table and index if they do not yet exist (idempotent).
func (s *MailStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS c4_mail (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  from_sess  TEXT NOT NULL DEFAULT '',
  to_sess    TEXT NOT NULL,
  subject    TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  project_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  read_at    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mail_to ON c4_mail(to_sess);
`)
	return err
}

// Send inserts a new mail message and returns the new row ID and the stored
// created_at timestamp (RFC3339). Both values come from a single clock read so
// the response returned to the caller always matches what is persisted in the DB.
// from="*" is rejected (reserved word). to="" is rejected (must specify a recipient).
func (s *MailStore) Send(from, to, subject, body, projectID string) (int64, string, error) {
	if from == "*" {
		return 0, "", fmt.Errorf("mailbox: from \"*\" is reserved")
	}
	if to == "" {
		return 0, "", fmt.Errorf("mailbox: to must not be empty")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO c4_mail (from_sess, to_sess, subject, body, project_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		from, to, subject, body, projectID, now,
	)
	if err != nil {
		return 0, "", fmt.Errorf("mailbox: send: %w", err)
	}
	id, err := res.LastInsertId()
	return id, now, err
}

// UnreadCount returns the number of unread messages for toSession.
// It counts messages addressed directly to toSession or to "*" (broadcast).
func (s *MailStore) UnreadCount(toSession string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM c4_mail
		 WHERE (to_sess = ? OR to_sess = '*') AND read_at = ''`,
		toSession,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("mailbox: unread count: %w", err)
	}
	return count, nil
}

// List returns messages for toSession (direct + broadcast), capped at 500 rows.
// If toSession is empty, all messages are returned (admin view).
// If unreadOnly is true, only messages with read_at='' are returned.
// Results are ordered by created_at DESC.
func (s *MailStore) List(toSession string, unreadOnly bool) ([]MailMessage, error) {
	var query string
	var args []any

	if toSession == "" {
		if unreadOnly {
			query = `SELECT id, from_sess, to_sess, subject, body, project_id, created_at, read_at
			         FROM c4_mail WHERE read_at = '' ORDER BY created_at DESC LIMIT 500`
		} else {
			query = `SELECT id, from_sess, to_sess, subject, body, project_id, created_at, read_at
			         FROM c4_mail ORDER BY created_at DESC LIMIT 500`
		}
	} else {
		if unreadOnly {
			query = `SELECT id, from_sess, to_sess, subject, body, project_id, created_at, read_at
			         FROM c4_mail WHERE (to_sess = ? OR to_sess = '*') AND read_at = ''
			         ORDER BY created_at DESC LIMIT 500`
		} else {
			query = `SELECT id, from_sess, to_sess, subject, body, project_id, created_at, read_at
			         FROM c4_mail WHERE (to_sess = ? OR to_sess = '*')
			         ORDER BY created_at DESC LIMIT 500`
		}
		args = append(args, toSession)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("mailbox: list: %w", err)
	}
	defer rows.Close()

	var msgs []MailMessage
	for rows.Next() {
		var m MailMessage
		if err := rows.Scan(&m.ID, &m.From, &m.To, &m.Subject, &m.Body, &m.ProjectID, &m.CreatedAt, &m.ReadAt); err != nil {
			return nil, fmt.Errorf("mailbox: list scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mailbox: list rows: %w", err)
	}
	return msgs, nil
}

// Read marks the message with the given id as read (idempotent) and returns it.
// Returns (nil, ErrNotFound) if the id does not exist.
// If already read, the existing read_at timestamp is preserved.
func (s *MailStore) Read(id int64) (*MailMessage, error) {
	// Mark as read only if not already read (idempotent).
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE c4_mail SET read_at = ? WHERE id = ? AND read_at = ''`,
		now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("mailbox: read update: %w", err)
	}

	// Fetch the message (regardless of whether the UPDATE changed anything).
	var m MailMessage
	err = s.db.QueryRow(
		`SELECT id, from_sess, to_sess, subject, body, project_id, created_at, read_at
		 FROM c4_mail WHERE id = ?`,
		id,
	).Scan(&m.ID, &m.From, &m.To, &m.Subject, &m.Body, &m.ProjectID, &m.CreatedAt, &m.ReadAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mailbox: read fetch: %w", err)
	}
	return &m, nil
}

// Delete removes the message with the given id.
// Returns ErrNotFound if the id does not exist.
func (s *MailStore) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM c4_mail WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mailbox: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mailbox: delete rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DefaultDBPath returns the default path for the mailbox database (~/.c4/mailbox.db).
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("mailbox: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "mailbox.db"), nil
}
