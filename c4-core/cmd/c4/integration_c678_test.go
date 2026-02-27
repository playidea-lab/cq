//go:build c6_guard && c7_observe && c8_gate

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/gate"
	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/observe"
)

// testGuardPublisher bridges guard deny events directly to a WebhookManager.
// Replaces EventBus: when guard.denied fires, it dispatches a gate.Event synchronously.
type testGuardPublisher struct {
	wm *gate.WebhookManager
}

func (p *testGuardPublisher) PublishAsync(evType, _ string, data json.RawMessage, _ string) {
	if evType == "guard.denied" {
		_ = p.wm.Dispatch(gate.Event{Type: evType, Data: data})
	}
}

// newTestRegistry builds a Registry with observe (outer) then guard (inner) middlewares.
// Execution order: observe → guard → handler, so denied calls are still metered.
func newTestRegistry(eng *guard.Engine, metrics *observe.Metrics) *mcp.Registry {
	reg := mcp.NewRegistry()
	// observe first → ctxMws[0]; guard second → ctxMws[1].
	// CallWithContext applies in reverse (last = innermost), so:
	//   result: observe(outer) → guard(inner) → handler.
	logger := observe.NewLogger(observe.LoggerOpts{Output: io.Discard})
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))
	reg.UseContextual(guard.ContextualMiddlewareFunc(eng, "mcp-session"))
	return reg
}

// ─── TestC678_DenyFlowsToWebhook ──────────────────────────────────────────────

// TestC678_DenyFlowsToWebhook verifies the end-to-end deny → observe → webhook path:
//  1. guard.Engine denies the tool call.
//  2. guard emits "guard.denied" via publisher → WebhookManager.Dispatch.
//  3. httptest server receives the HTTP POST with the correct event type.
func TestC678_DenyFlowsToWebhook(t *testing.T) {
	var (
		received []byte
		mu       sync.Mutex
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Gate: WebhookManager subscribed to "guard.denied" events.
	wm := gate.NewWebhookManager(gate.WebhookConfig{DefaultTimeout: 5 * time.Second})
	wm.RegisterEndpoint("test-ep", srv.URL, "", []string{"guard.denied"})

	// Guard: deny policy for "blocked_tool".
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "blocked_tool", Action: guard.ActionDeny, Reason: "test deny"},
		},
	}
	dir := t.TempDir()
	eng, err := guard.NewEngine(dir+"/guard.db", cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// Wire publisher: guard deny → wm.Dispatch (no real EventBus needed).
	eng.SetPublisher(&testGuardPublisher{wm: wm})

	// Registry: observe(outer) → guard(inner).
	metrics := observe.NewMetrics()
	reg := newTestRegistry(eng, metrics)
	reg.Register(mcp.ToolSchema{Name: "blocked_tool"}, func(args json.RawMessage) (any, error) {
		return "should-not-reach", nil
	})

	// Invoke — expect denial.
	_, err = reg.CallWithContext(context.Background(), "blocked_tool", nil)
	if err == nil {
		t.Fatal("expected error for denied tool, got nil")
	}

	// Webhook must have been POSTed synchronously by testGuardPublisher.
	mu.Lock()
	body := received
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("expected webhook POST body, got empty")
	}

	// Verify the posted body contains a "guard.denied" event envelope.
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("webhook body is not valid JSON: %v — body: %s", err, body)
	}
	if got, _ := payload["type"].(string); got != "guard.denied" {
		t.Errorf("webhook event type = %q, want guard.denied", got)
	}
}

// ─── TestC678_AllowNoWebhook ──────────────────────────────────────────────────

// TestC678_AllowNoWebhook verifies that an allowed call does NOT trigger a webhook POST.
func TestC678_AllowNoWebhook(t *testing.T) {
	postCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		postCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wm := gate.NewWebhookManager(gate.WebhookConfig{DefaultTimeout: 5 * time.Second})
	wm.RegisterEndpoint("test-ep", srv.URL, "", []string{"guard.denied"})

	// Guard with no deny policy → default allow.
	cfg := guard.Config{Enabled: true, Policies: nil}
	dir := t.TempDir()
	eng, err := guard.NewEngine(dir+"/guard.db", cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()
	eng.SetPublisher(&testGuardPublisher{wm: wm})

	metrics := observe.NewMetrics()
	reg := newTestRegistry(eng, metrics)
	reg.Register(mcp.ToolSchema{Name: "open_tool"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	_, err = reg.CallWithContext(context.Background(), "open_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error for allowed tool: %v", err)
	}

	mu.Lock()
	n := postCount
	mu.Unlock()
	if n != 0 {
		t.Errorf("expected 0 webhook POSTs for allowed call, got %d", n)
	}
}

// ─── TestC678_ObserveRecordsDenied ───────────────────────────────────────────

// TestC678_ObserveRecordsDenied verifies that observe metrics record denied calls as errors.
// Even when guard blocks the call, the outer observe middleware still counts the error.
func TestC678_ObserveRecordsDenied(t *testing.T) {
	cfg := guard.Config{
		Enabled: true,
		Policies: []guard.PolicyRule{
			{Tool: "deny_me", Action: guard.ActionDeny, Reason: "test"},
		},
	}
	dir := t.TempDir()
	eng, err := guard.NewEngine(dir+"/guard.db", cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	metrics := observe.NewMetrics()
	reg := newTestRegistry(eng, metrics)
	reg.Register(mcp.ToolSchema{Name: "deny_me"}, func(args json.RawMessage) (any, error) {
		return "should-not-reach", nil
	})

	_, _ = reg.CallWithContext(context.Background(), "deny_me", nil)

	snap := metrics.Snapshot()
	toolMetrics, ok := snap.Tools["deny_me"]
	if !ok {
		t.Fatal("expected metrics entry for deny_me, got none")
	}
	if toolMetrics.Calls != 1 {
		t.Errorf("calls = %d, want 1", toolMetrics.Calls)
	}
	if toolMetrics.Errors != 1 {
		t.Errorf("errors = %d, want 1 (denied call must be counted as error)", toolMetrics.Errors)
	}
}
