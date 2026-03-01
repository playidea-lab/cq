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
	if err := s.Append(context.Background(), "ch-1", "dooray", "proj-1", in); err != nil {
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
