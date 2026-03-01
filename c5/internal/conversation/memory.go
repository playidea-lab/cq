package conversation

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-process, TTL-based conversation store.
// It keeps the most recent maxMsgs messages per channel and discards entries
// after ttl of inactivity. Use as a fallback when Supabase is not configured.
type MemoryStore struct {
	mu           sync.Mutex
	entries      map[string]*memEntry
	channels     map[string]string // channelKey → synthetic id
	participants map[string]string // participantKey → synthetic id
	counters     [2]int            // [0]=channels count, [1]=participants count
	maxMsgs      int
	ttl          time.Duration
}

type memEntry struct {
	msgs   []Message
	lastAt time.Time
}

// NewMemoryStore creates a MemoryStore with the given capacity and TTL.
func NewMemoryStore(maxMsgs int, ttl time.Duration) *MemoryStore {
	return &MemoryStore{
		entries:      make(map[string]*memEntry),
		channels:     make(map[string]string),
		participants: make(map[string]string),
		maxMsgs:      maxMsgs,
		ttl:          ttl,
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

// EnsureChannel creates or returns an in-memory synthetic ID for the channel.
// IDs are stable within a process lifetime but not persisted across restarts.
func (m *MemoryStore) EnsureChannel(_ context.Context, ch Channel) (string, error) {
	tenant := ch.TenantID
	if tenant == "" {
		tenant = "default"
	}
	key := tenant + "\x00" + ch.Platform + "\x00" + ch.Name
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.channels[key]; ok {
		return id, nil
	}
	id := fmt.Sprintf("mem-ch-%d", m.counters[0])
	m.counters[0]++
	m.channels[key] = id
	return id, nil
}

// ListChannels returns an empty slice; not meaningful for the in-memory fallback.
func (m *MemoryStore) ListChannels(_ context.Context, _, _ string) ([]Channel, error) {
	return nil, nil
}

// EnsureParticipant creates or returns an in-memory synthetic ID for the participant.
// IDs are stable within a process lifetime but not persisted across restarts.
func (m *MemoryStore) EnsureParticipant(_ context.Context, p Participant) (string, error) {
	if p.PlatformID == "" {
		return "", nil // no-op: matches SupabaseStore behaviour
	}
	tenant := p.TenantID
	if tenant == "" {
		tenant = "default"
	}
	key := tenant + "\x00" + p.Platform + "\x00" + p.PlatformID
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.participants[key]; ok {
		return id, nil
	}
	id := fmt.Sprintf("mem-p-%d", m.counters[1])
	m.counters[1]++
	m.participants[key] = id
	return id, nil
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
