package conversation

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_GetEmpty(t *testing.T) {
	s := NewMemoryStore(20, 30*time.Minute)
	msgs, err := s.Get(context.Background(), "ch-1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty, got %d messages", len(msgs))
	}
}

func TestMemoryStore_AppendAndGet(t *testing.T) {
	s := NewMemoryStore(20, 30*time.Minute)
	in := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	if err := s.Append(context.Background(), "ch-1", "telegram", "proj-1", in); err != nil {
		t.Fatalf("append error: %v", err)
	}
	msgs, err := s.Get(context.Background(), "ch-1", 10)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("unexpected msg[0]: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "world" {
		t.Errorf("unexpected msg[1]: %+v", msgs[1])
	}
}

func TestMemoryStore_LimitCap(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(4, 30*time.Minute)
	for i := range 5 {
		_ = s.Append(ctx, "ch-1", "", "", []Message{{Role: "user", Content: string(rune('a' + i))}})
	}
	msgs, _ := s.Get(ctx, "ch-1", 100)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (cap), got %d", len(msgs))
	}
	// Oldest messages should have been evicted; last 4 remain.
	if msgs[0].Content != "b" {
		t.Errorf("expected 'b' first, got %q", msgs[0].Content)
	}
}

func TestMemoryStore_LimitCap_BatchOverflow(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(4, 30*time.Minute)
	// Single Append with 6 messages — only last 4 should survive.
	batch := []Message{
		{Role: "user", Content: "a"},
		{Role: "user", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "user", Content: "d"},
		{Role: "user", Content: "e"},
		{Role: "user", Content: "f"},
	}
	_ = s.Append(ctx, "ch-1", "", "", batch)
	msgs, _ := s.Get(ctx, "ch-1", 100)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (cap), got %d", len(msgs))
	}
	if msgs[0].Content != "c" {
		t.Errorf("expected 'c' first after overflow, got %q", msgs[0].Content)
	}
}

func TestMemoryStore_GetLimit(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)
	for i := range 10 {
		_ = s.Append(ctx, "ch-1", "", "", []Message{{Role: "user", Content: string(rune('a' + i))}})
	}
	msgs, _ := s.Get(ctx, "ch-1", 3)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
}

func TestMemoryStore_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 10*time.Millisecond)
	_ = s.Append(ctx, "ch-1", "", "", []Message{{Role: "user", Content: "hi"}})
	time.Sleep(20 * time.Millisecond)
	msgs, _ := s.Get(ctx, "ch-1", 10)
	if len(msgs) != 0 {
		t.Fatalf("expected expired entry, got %d messages", len(msgs))
	}
}

func TestMemoryStore_Cleanup(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 10*time.Millisecond)
	_ = s.Append(ctx, "ch-1", "", "", []Message{{Role: "user", Content: "hi"}})
	_ = s.Append(ctx, "ch-2", "", "", []Message{{Role: "user", Content: "bye"}})
	time.Sleep(20 * time.Millisecond)
	s.Cleanup()
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected all entries cleaned, got %d", n)
	}
}

func TestMemoryStore_ConcurrentSafe(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(100, time.Minute)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch := string(rune('a' + id))
			_ = s.Append(ctx, ch, "", "", []Message{{Role: "user", Content: "msg"}})
			_, _ = s.Get(ctx, ch, 5)
		}(i)
	}
	wg.Wait()
}

func TestMemoryStore_EnsureChannel(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)

	ch := Channel{TenantID: "default", Platform: "telegram", Name: "tg-123", ChannelType: "bot"}
	id1, err := s.EnsureChannel(ctx, ch)
	if err != nil {
		t.Fatalf("ensure channel: %v", err)
	}
	if id1 == "" {
		t.Fatal("expected non-empty id")
	}

	// Same channel → same id (idempotent).
	id2, err := s.EnsureChannel(ctx, ch)
	if err != nil {
		t.Fatalf("ensure channel second call: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected same id on second call, got %q vs %q", id1, id2)
	}

	// Different channel → different id.
	ch2 := Channel{TenantID: "default", Platform: "telegram", Name: "tg-456", ChannelType: "bot"}
	id3, _ := s.EnsureChannel(ctx, ch2)
	if id1 == id3 {
		t.Errorf("expected different ids for different channels, both got %q", id1)
	}
}

func TestMemoryStore_EnsureChannel_DefaultTenant(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)

	// Empty TenantID should default to "default".
	ch1 := Channel{TenantID: "", Platform: "telegram", Name: "tg-999", ChannelType: "bot"}
	ch2 := Channel{TenantID: "default", Platform: "telegram", Name: "tg-999", ChannelType: "bot"}
	id1, _ := s.EnsureChannel(ctx, ch1)
	id2, _ := s.EnsureChannel(ctx, ch2)
	if id1 != id2 {
		t.Errorf("empty tenant should resolve to 'default': got %q vs %q", id1, id2)
	}
}

func TestMemoryStore_EnsureParticipant(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)

	p := Participant{TenantID: "default", Platform: "telegram", PlatformID: "user-1", MemberType: "user"}
	id1, err := s.EnsureParticipant(ctx, p)
	if err != nil {
		t.Fatalf("ensure participant: %v", err)
	}
	if id1 == "" {
		t.Fatal("expected non-empty id")
	}

	// Same participant → same id (idempotent).
	id2, _ := s.EnsureParticipant(ctx, p)
	if id1 != id2 {
		t.Errorf("expected same id on second call, got %q vs %q", id1, id2)
	}

	// Different platform_id → different id.
	p2 := Participant{TenantID: "default", Platform: "telegram", PlatformID: "user-2", MemberType: "user"}
	id3, _ := s.EnsureParticipant(ctx, p2)
	if id1 == id3 {
		t.Errorf("expected different ids for different participants")
	}
}

func TestMemoryStore_ListChannels(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)
	// ListChannels is a no-op for MemoryStore fallback.
	chs, err := s.ListChannels(ctx, "default", "")
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(chs) != 0 {
		t.Errorf("expected empty list from MemoryStore, got %d", len(chs))
	}
}

func TestMemoryStore_EnsureChannelAndAppendGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(20, 30*time.Minute)

	// Create channel and use returned ID for Append/Get.
	ch := Channel{TenantID: "default", Platform: "telegram", Name: "tg-abc", ChannelType: "bot"}
	chID, err := s.EnsureChannel(ctx, ch)
	if err != nil {
		t.Fatalf("ensure channel: %v", err)
	}

	_ = s.Append(ctx, chID, "telegram", "", []Message{
		{Role: "user", Content: "ping"},
		{Role: "assistant", Content: "pong"},
	})

	msgs, err := s.Get(ctx, chID, 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "ping" || msgs[1].Content != "pong" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}
