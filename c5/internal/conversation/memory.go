package conversation

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-process, TTL-based conversation store.
// It keeps the most recent maxMsgs messages per channel and discards entries
// after ttl of inactivity. Use as a fallback when Supabase is not configured.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]*memEntry
	maxMsgs int
	ttl     time.Duration
}

type memEntry struct {
	msgs   []Message
	lastAt time.Time
}

// NewMemoryStore creates a MemoryStore with the given capacity and TTL.
func NewMemoryStore(maxMsgs int, ttl time.Duration) *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]*memEntry),
		maxMsgs: maxMsgs,
		ttl:     ttl,
	}
}

// Compile-time interface assertion.
var _ Store = (*MemoryStore)(nil)

// Get returns up to limit messages for channelID, oldest-first.
// Returns nil if the channel is unknown or its entry has expired.
func (m *MemoryStore) Get(_ context.Context, channelID string, limit int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[channelID]
	if !ok || time.Since(e.lastAt) > m.ttl {
		return nil, nil
	}
	msgs := e.msgs
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

// Append adds msgs to the channel history, capping at maxMsgs entries.
func (m *MemoryStore) Append(_ context.Context, channelID, _, _ string, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[channelID]
	if !ok || time.Since(e.lastAt) > m.ttl {
		e = &memEntry{}
		m.entries[channelID] = e
	}
	e.msgs = append(e.msgs, msgs...)
	e.lastAt = time.Now()
	if m.maxMsgs > 0 && len(e.msgs) > m.maxMsgs {
		e.msgs = e.msgs[len(e.msgs)-m.maxMsgs:]
	}
	return nil
}

// Cleanup removes expired entries.
func (m *MemoryStore) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.entries {
		if time.Since(e.lastAt) > m.ttl {
			delete(m.entries, id)
		}
	}
}
