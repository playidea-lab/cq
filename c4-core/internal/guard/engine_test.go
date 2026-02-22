package guard_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/changmin/c4-core/internal/guard"
)

// mockPublisher counts PublishAsync calls for assertion in tests.
type mockPublisher struct {
	count atomic.Int32
}

func (m *mockPublisher) PublishAsync(_ string, _ string, _ json.RawMessage, _ string) {
	m.count.Add(1)
}

// TestEngine_Deny_PublishesEvent verifies that a Deny decision emits exactly one event.
func TestEngine_Deny_PublishesEvent(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	pub := &mockPublisher{}
	eng.SetPublisher(pub)

	ctx := context.Background()

	// Add a deny policy so Check returns ActionDeny.
	if err := eng.SavePolicy(ctx, guard.PolicyRule{
		Tool:   "c4_secret_delete",
		Action: guard.ActionDeny,
		Reason: "not allowed",
	}); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}

	result := eng.Check(ctx, "alice", "c4_secret_delete", nil)
	if result != guard.ActionDeny {
		t.Fatalf("expected ActionDeny, got %v", result)
	}
	if got := pub.count.Load(); got != 1 {
		t.Errorf("PublishAsync call count: want 1, got %d", got)
	}
}

// TestEngine_Deny_NoPublisher_NoPanic verifies no panic when publisher is nil.
func TestEngine_Deny_NoPublisher_NoPanic(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	if err := eng.SavePolicy(ctx, guard.PolicyRule{
		Tool:   "c4_secret_delete",
		Action: guard.ActionDeny,
		Reason: "not allowed",
	}); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}

	// Must not panic with nil publisher.
	result := eng.Check(ctx, "alice", "c4_secret_delete", nil)
	if result != guard.ActionDeny {
		t.Errorf("expected ActionDeny, got %v", result)
	}
}

// TestEngine_AuditOnly_NoPublish verifies that AuditOnly decisions do not emit a guard.denied event.
func TestEngine_AuditOnly_NoPublish(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	pub := &mockPublisher{}
	eng.SetPublisher(pub)

	ctx := context.Background()

	if err := eng.SavePolicy(ctx, guard.PolicyRule{
		Tool:   "c4_sensitive",
		Action: guard.ActionAuditOnly,
		Reason: "audit only",
	}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	result := eng.Check(ctx, "alice", "c4_sensitive", nil)
	if result != guard.ActionAuditOnly {
		t.Fatalf("expected ActionAuditOnly, got %v", result)
	}
	// AuditOnly is not a denial — must not emit guard.denied event.
	if got := pub.count.Load(); got != 0 {
		t.Errorf("PublishAsync call count: want 0 for AuditOnly, got %d", got)
	}
	// AuditOnly must still write an audit log entry.
	entries, err := eng.AuditEntries(ctx, 5, "c4_sensitive", "alice")
	if err != nil {
		t.Fatalf("AuditEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected audit log entry for AuditOnly, got none")
	}
	if entries[0].Action != guard.ActionAuditOnly {
		t.Errorf("audit entry action = %v, want ActionAuditOnly", entries[0].Action)
	}
}

// TestEngine_Allow_NoPublish verifies that Allow decisions do not call PublishAsync.
func TestEngine_Allow_NoPublish(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	pub := &mockPublisher{}
	eng.SetPublisher(pub)

	ctx := context.Background()

	// No policy → default Allow.
	result := eng.Check(ctx, "alice", "c4_task_list", nil)
	if result != guard.ActionAllow {
		t.Fatalf("expected ActionAllow, got %v", result)
	}
	if got := pub.count.Load(); got != 0 {
		t.Errorf("PublishAsync call count: want 0, got %d", got)
	}
}
