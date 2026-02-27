package guard_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp"
)

// TestContextualMW_Deny verifies that a deny policy blocks the call and does not invoke the handler.
func TestContextualMW_Deny(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "blocked_tool", Action: guard.ActionDeny, Reason: "blocked by policy"},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "mcp-session"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "blocked_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "should-not-reach", nil
	})

	_, err = reg.CallWithContext(context.Background(), "blocked_tool", nil)
	if err == nil {
		t.Fatal("expected error for denied tool, got nil")
	}
	if handlerCalled {
		t.Error("handler must not be called for denied tool")
	}
}

// TestContextualMW_Allow verifies that an allowed tool call passes through to the handler.
func TestContextualMW_Allow(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "mcp-session"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "allowed_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "ok", nil
	})

	result, err := reg.CallWithContext(context.Background(), "allowed_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %v, want ok", result)
	}
	if !handlerCalled {
		t.Error("handler was not called for allowed tool")
	}
}

// TestContextualMW_AuditOnly verifies that audit_only passes through to the handler.
func TestContextualMW_AuditOnly(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "sensitive_tool", Action: guard.ActionAuditOnly, Reason: "audit only"},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "mcp-session"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "sensitive_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "audited", nil
	})

	result, err := reg.CallWithContext(context.Background(), "sensitive_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error for AuditOnly: %v", err)
	}
	if result != "audited" {
		t.Errorf("result = %v, want audited", result)
	}
	if !handlerCalled {
		t.Error("handler must be called for AuditOnly")
	}
}

// TestContextualMW_ActorFromContext verifies that the actor injected via WithActor is used for Check.
func TestContextualMW_ActorFromContext(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "default-actor"))

	reg.Register(mcp.ToolSchema{Name: "my_tool"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	ctx := guard.WithActor(context.Background(), "carol")
	_, err = reg.CallWithContext(ctx, "my_tool", nil)
	if err != nil {
		t.Fatalf("CallWithContext: %v", err)
	}

	// carol must appear in the audit log (not default-actor).
	entries, err := eng.AuditEntries(context.Background(), 10, "my_tool", "carol")
	if err != nil {
		t.Fatalf("AuditEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected audit entry for actor=carol, got none")
	}
}

// TestContextualMW_DefaultActor verifies that defaultActor is used when ctx carries no actor.
func TestContextualMW_DefaultActor(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "default-actor"))

	reg.Register(mcp.ToolSchema{Name: "my_tool"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	// No actor in context → defaultActor must be used.
	_, err = reg.CallWithContext(context.Background(), "my_tool", nil)
	if err != nil {
		t.Fatalf("CallWithContext: %v", err)
	}

	entries, err := eng.AuditEntries(context.Background(), 10, "my_tool", "default-actor")
	if err != nil {
		t.Fatalf("AuditEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected audit entry for actor=default-actor, got none")
	}
}
