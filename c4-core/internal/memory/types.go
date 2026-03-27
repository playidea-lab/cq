// Package memory provides import of AI conversation sessions into the CQ knowledge store.
package memory

import (
	"io"
	"time"
)

// Turn represents a single message in a conversation session.
type Turn struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// Session represents a parsed conversation session ready for import.
type Session struct {
	ID        string
	Title     string    // conversation title (from export)
	Source    string    // "chatgpt", "claude-code", "codex", "gemini-cli", "claude-web"
	Project   string    // project path or name
	StartedAt time.Time // session start time
	Turns     []Turn
}

// Parser extracts sessions from platform-specific formats.
type Parser interface {
	Parse(r io.Reader) ([]Session, error)
	Source() string
}

// ImportResult holds the outcome of an import batch.
type ImportResult struct {
	Total    int
	Imported int
	Skipped  int // already imported (dedup)
	Errors   []ImportError
}

// ImportError records a per-session failure.
type ImportError struct {
	SessionID string
	Err       error
}
