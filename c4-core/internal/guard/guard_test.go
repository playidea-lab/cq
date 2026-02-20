// Package guard_test exercises the guard engine: RBAC, policy, audit, middleware.
package guard_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp"
)

// helpers

func tempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "guard.db")
}

func defaultConfig() guard.Config {
	return guard.Config{
		Enabled:  true,
		Policies: []guard.PolicyRule{},
	}
}

// ─── TestRBACBasic ────────────────────────────────────────────────────────────

// TestRBACBasic verifies that role → permission mapping works correctly.
// An actor with the "admin" role should be allowed any tool.
// An actor with the "readonly" role should be denied write tools.
func TestRBACBasic(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	// Assign roles via RBAC
	if err := eng.AssignRole(ctx, "alice", "admin"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := eng.AssignRole(ctx, "bob", "readonly"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	// Define permissions
	if err := eng.GrantPermission(ctx, "admin", "c4_task_list", guard.ActionAllow); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}
	if err := eng.GrantPermission(ctx, "admin", "c4_add_todo", guard.ActionAllow); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}
	if err := eng.GrantPermission(ctx, "readonly", "c4_task_list", guard.ActionAllow); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}
	if err := eng.GrantPermission(ctx, "readonly", "c4_add_todo", guard.ActionDeny); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}

	// alice (admin) can call c4_add_todo
	result := eng.Check(ctx, "alice", "c4_add_todo", nil)
	if result != guard.ActionAllow {
		t.Errorf("alice c4_add_todo: want Allow, got %v", result)
	}

	// bob (readonly) is denied c4_add_todo
	result = eng.Check(ctx, "bob", "c4_add_todo", nil)
	if result != guard.ActionDeny {
		t.Errorf("bob c4_add_todo: want Deny, got %v", result)
	}

	// bob (readonly) can call c4_task_list
	result = eng.Check(ctx, "bob", "c4_task_list", nil)
	if result != guard.ActionAllow {
		t.Errorf("bob c4_task_list: want Allow, got %v", result)
	}
}

// ─── TestPolicyDeny ───────────────────────────────────────────────────────────

// TestPolicyDeny verifies that a policy-denied tool call is rejected.
// Even if RBAC allows, a policy Deny wins.
func TestPolicyDeny(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "c4_shell_exec", Action: guard.ActionDeny, Reason: "shell execution blocked"},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	// Even without explicit RBAC, the policy should deny
	result := eng.Check(ctx, "anyone", "c4_shell_exec", nil)
	if result != guard.ActionDeny {
		t.Errorf("c4_shell_exec: want Deny from policy, got %v", result)
	}

	// Unblocked tool falls through to default Allow
	result = eng.Check(ctx, "anyone", "c4_task_list", nil)
	if result != guard.ActionAllow {
		t.Errorf("c4_task_list: want Allow (default), got %v", result)
	}
}

// ─── TestAuditLog ─────────────────────────────────────────────────────────────

// TestAuditLog verifies that every Check call is recorded in the audit log.
func TestAuditLog(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	// Perform a few checks
	eng.Check(ctx, "alice", "c4_task_list", nil)
	eng.Check(ctx, "bob", "c4_add_todo", json.RawMessage(`{"title":"test"}`))

	entries, err := eng.AuditEntries(ctx, 10)
	if err != nil {
		t.Fatalf("AuditEntries: %v", err)
	}

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 audit entries, got %d", len(entries))
	}

	// Verify alice's entry is present
	found := false
	for _, e := range entries {
		if e.Actor == "alice" && e.Tool == "c4_task_list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("audit log missing alice/c4_task_list entry")
	}
}

// ─── TestMiddlewareAllow ──────────────────────────────────────────────────────

// TestMiddlewareAllow verifies that an allowed tool call passes through the middleware.
// Uses MiddlewareForTool to bind the guard check to a specific tool name.
func TestMiddlewareAllow(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	reg := mcp.NewRegistry()
	// MiddlewareForTool binds enforcement to a specific tool name.
	reg.Use(guard.MiddlewareForTool(eng, "test-actor", "allowed_tool"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "allowed_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "ok", nil
	})

	result, err := reg.Call("allowed_tool", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %v, want ok", result)
	}
	if !handlerCalled {
		t.Error("handler was not called for allowed tool")
	}
}

// ─── TestMiddlewareDeny ───────────────────────────────────────────────────────

// TestMiddlewareDeny verifies that a denied tool call is rejected by the middleware.
func TestMiddlewareDeny(t *testing.T) {
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
	reg.Use(guard.MiddlewareForTool(eng, "test-actor", "blocked_tool"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "blocked_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "should-not-reach", nil
	})

	_, err = reg.Call("blocked_tool", nil)
	if err == nil {
		t.Fatal("expected error for blocked tool, got nil")
	}
	if handlerCalled {
		t.Error("handler should not be called for blocked tool")
	}
}

// ─── TestConfigOverride ───────────────────────────────────────────────────────

// TestConfigOverride verifies that config.yaml policy takes priority over DB role permissions.
// Even if RBAC would allow a tool, a config-level policy deny wins.
func TestConfigOverride(t *testing.T) {
	// Config policy explicitly denies "c4_dangerous"
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "c4_dangerous", Action: guard.ActionDeny, Reason: "config override", Priority: 100},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	// Grant RBAC allow for the tool — config policy should override
	if err := eng.AssignRole(ctx, "superuser", "superrole"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := eng.GrantPermission(ctx, "superrole", "c4_dangerous", guard.ActionAllow); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}

	// Config policy priority > RBAC
	result := eng.Check(ctx, "superuser", "c4_dangerous", nil)
	if result != guard.ActionDeny {
		t.Errorf("config override: want Deny, got %v", result)
	}
}

// ─── TestAuditOnly ────────────────────────────────────────────────────────────

// TestAuditOnly verifies that AuditOnly action allows the call but still records it.
func TestAuditOnly(t *testing.T) {
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

	ctx := context.Background()
	result := eng.Check(ctx, "alice", "sensitive_tool", nil)
	if result != guard.ActionAuditOnly {
		t.Errorf("want AuditOnly, got %v", result)
	}

	// In middleware, AuditOnly should allow the handler to run
	reg := mcp.NewRegistry()
	reg.Use(guard.MiddlewareForTool(eng, "alice", "sensitive_tool"))

	handlerCalled := false
	reg.Register(mcp.ToolSchema{Name: "sensitive_tool"}, func(args json.RawMessage) (any, error) {
		handlerCalled = true
		return "audited", nil
	})

	result2, err := reg.Call("sensitive_tool", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if result2 != "audited" {
		t.Errorf("result = %v, want audited", result2)
	}
	if !handlerCalled {
		t.Error("handler must be called for AuditOnly")
	}
}

// ─── TestDisabledEngine ───────────────────────────────────────────────────────

// TestDisabledEngine verifies that a disabled guard engine allows everything.
func TestDisabledEngine(t *testing.T) {
	cfg := guard.Config{
		Enabled: false,
		Policies: []guard.PolicyRule{
			{Tool: "any_tool", Action: guard.ActionDeny},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()
	result := eng.Check(ctx, "bob", "any_tool", nil)
	if result != guard.ActionAllow {
		t.Errorf("disabled guard: want Allow, got %v", result)
	}
}

// ─── TestManualAuditLog ───────────────────────────────────────────────────────

// TestManualAuditLog verifies that engine.AuditLog records a custom entry.
func TestManualAuditLog(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()
	entry := guard.AuditEntry{
		Actor:  "system",
		Tool:   "c4_status",
		Action: guard.ActionAllow,
		Reason: "manual",
	}
	if err := eng.AuditLog(ctx, entry); err != nil {
		t.Fatalf("AuditLog: %v", err)
	}

	entries, err := eng.AuditEntries(ctx, 5)
	if err != nil {
		t.Fatalf("AuditEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry after AuditLog")
	}
	e := entries[0]
	if e.Actor != "system" || e.Tool != "c4_status" {
		t.Errorf("wrong entry: %+v", e)
	}
}

// ─── TestWildcardPolicy ───────────────────────────────────────────────────────

// TestWildcardPolicy verifies wildcard "*" tool pattern matches all tools.
func TestWildcardPolicy(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "*", Action: guard.ActionAuditOnly, Reason: "audit all"},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	result := eng.Check(ctx, "alice", "c4_task_list", nil)
	if result != guard.ActionAuditOnly {
		t.Errorf("wildcard policy: want AuditOnly, got %v", result)
	}
	result = eng.Check(ctx, "alice", "c4_add_todo", nil)
	if result != guard.ActionAuditOnly {
		t.Errorf("wildcard policy: want AuditOnly for c4_add_todo, got %v", result)
	}
}

// ─── TestActorFromContext ─────────────────────────────────────────────────────

// TestActorFromContext verifies that the actor can be stored and retrieved from context.
func TestActorFromContext(t *testing.T) {
	ctx := guard.WithActor(context.Background(), "carol")
	actor := guard.ActorFromContext(ctx)
	if actor != "carol" {
		t.Errorf("actor = %q, want carol", actor)
	}
}

// ─── TestPolicyPersistence ───────────────────────────────────────────────────

// TestPolicyPersistence verifies that policies can be saved to DB and reloaded.
func TestPolicyPersistence(t *testing.T) {
	dbPath := tempDB(t)

	// Create engine and save a policy
	eng1, err := guard.NewEngine(dbPath, defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()
	if err := eng1.SavePolicy(ctx, guard.PolicyRule{
		Tool:   "c4_nuke",
		Action: guard.ActionDeny,
		Reason: "never allow nukes",
	}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}
	eng1.Close()

	// Reopen and verify policy is applied
	eng2, err := guard.NewEngine(dbPath, defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine (reopen): %v", err)
	}
	defer eng2.Close()

	result := eng2.Check(ctx, "alice", "c4_nuke", nil)
	if result != guard.ActionDeny {
		t.Errorf("persisted policy: want Deny, got %v", result)
	}
}

// ─── TestRoleUnassign ─────────────────────────────────────────────────────────

// TestRoleUnassign verifies that an actor whose role is unassigned falls back to default.
func TestRoleUnassign(t *testing.T) {
	eng, err := guard.NewEngine(tempDB(t), defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()
	if err := eng.AssignRole(ctx, "dave", "restrictive"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := eng.GrantPermission(ctx, "restrictive", "c4_task_list", guard.ActionDeny); err != nil {
		t.Fatalf("GrantPermission: %v", err)
	}

	// Before unassign: denied
	result := eng.Check(ctx, "dave", "c4_task_list", nil)
	if result != guard.ActionDeny {
		t.Errorf("before unassign: want Deny, got %v", result)
	}

	// Unassign role
	if err := eng.UnassignRole(ctx, "dave", "restrictive"); err != nil {
		t.Fatalf("UnassignRole: %v", err)
	}

	// After unassign: default Allow
	result = eng.Check(ctx, "dave", "c4_task_list", nil)
	if result != guard.ActionAllow {
		t.Errorf("after unassign: want Allow (default), got %v", result)
	}
}

// ─── TestDBCreation ───────────────────────────────────────────────────────────

// TestDBCreation verifies the engine creates the SQLite DB file on startup.
func TestDBCreation(t *testing.T) {
	dbPath := tempDB(t)
	eng, err := guard.NewEngine(dbPath, defaultConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	eng.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("expected DB file at %s, not found", dbPath)
	}
}

// ─── TestMiddlewareMultiTool ──────────────────────────────────────────────────

// TestMiddlewareMultiTool verifies the generic Middleware works with a tool resolver.
func TestMiddlewareMultiTool(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "restricted", Action: guard.ActionDeny},
		},
	}
	eng, err := guard.NewEngine(tempDB(t), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// Simulate a tool resolver that tracks which tool is being called.
	currentTool := "open_tool"
	resolver := func() string { return currentTool }

	reg := mcp.NewRegistry()
	reg.Use(guard.MiddlewareWithResolver(eng, "actor", resolver))

	calledCount := 0
	reg.Register(mcp.ToolSchema{Name: "open_tool"}, func(args json.RawMessage) (any, error) {
		calledCount++
		return "ok", nil
	})
	reg.Register(mcp.ToolSchema{Name: "restricted"}, func(args json.RawMessage) (any, error) {
		calledCount++
		return "bad", nil
	})

	// open_tool: allowed
	_, err = reg.Call("open_tool", nil)
	if err != nil {
		t.Fatalf("open_tool: %v", err)
	}

	// restricted: denied (update resolver)
	currentTool = "restricted"
	_, err = reg.Call("restricted", nil)
	if err == nil {
		t.Fatal("expected error for restricted tool")
	}

	if calledCount != 1 {
		t.Errorf("calledCount = %d, want 1 (restricted handler must not run)", calledCount)
	}
}
