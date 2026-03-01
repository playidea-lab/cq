// Package conversation provides platform-agnostic conversation history storage.
// It supports both an in-memory TTL-based store (MemoryStore) and a Supabase-backed
// persistent store (SupabaseStore).
package conversation

import "context"

// Message is a single chat turn with a role and content.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Store is the interface for reading and writing conversation history.
// Implementations must be safe for concurrent use.
type Store interface {
	// Get returns the most recent limit messages for the given channel,
	// ordered oldest-first (suitable for LLM context).
	Get(ctx context.Context, channelID string, limit int) ([]Message, error)

	// Append adds msgs to the history for the given channel.
	// platform and projectID are metadata stored alongside the messages.
	Append(ctx context.Context, channelID, platform, projectID string, msgs []Message) error

	// Cleanup removes expired or stale entries.
	Cleanup()
}
