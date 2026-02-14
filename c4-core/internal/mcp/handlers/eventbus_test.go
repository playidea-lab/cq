package handlers

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/eventbus"
	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/changmin/c4-core/internal/mcp"
	"google.golang.org/grpc"
)

// setupEventBusTest creates a gRPC server on a temp Unix socket, client, and MCP registry.
func setupEventBusTest(t *testing.T) (*mcp.Registry, func()) {
	t.Helper()

	dir := t.TempDir()
	store, err := eventbus.NewStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}

	dispatcher := eventbus.NewDispatcher(store)
	srv := eventbus.NewServer(eventbus.ServerConfig{
		Store:      store,
		Dispatcher: dispatcher,
	})

	sockPath := filepath.Join(dir, "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterEventBusServer(grpcServer, srv)
	go grpcServer.Serve(ln)

	// Create client via UDS
	ebClient, err := eventbus.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}

	reg := mcp.NewRegistry()
	RegisterEventBusHandlers(reg, ebClient)

	cleanup := func() {
		ebClient.Close()
		grpcServer.GracefulStop()
		ln.Close()
		store.Close()
	}

	return reg, cleanup
}

func TestEventBusPublish(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	result, err := reg.Call("c4_event_publish", json.RawMessage(`{
		"type": "test.hello",
		"source": "unit-test",
		"data": {"message": "world"}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]any)
	if m["status"] != "published" {
		t.Errorf("expected status published, got %v", m["status"])
	}
	if m["event_id"] == nil || m["event_id"] == "" {
		t.Error("expected non-empty event_id")
	}
}

func TestEventBusList(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	// Publish some events
	reg.Call("c4_event_publish", json.RawMessage(`{"type":"test.a","source":"test"}`))
	reg.Call("c4_event_publish", json.RawMessage(`{"type":"test.b","source":"test"}`))

	// List all
	result, err := reg.Call("c4_event_list", json.RawMessage(`{"limit": 10}`))
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]any)
	count := m["count"].(int)
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}

	// List filtered
	result2, err := reg.Call("c4_event_list", json.RawMessage(`{"type":"test.a","limit":10}`))
	if err != nil {
		t.Fatal(err)
	}
	m2 := result2.(map[string]any)
	if m2["count"].(int) != 1 {
		t.Errorf("expected 1 filtered event, got %v", m2["count"])
	}
}

func TestEventBusRulesCRUD(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	// Add rule
	result, err := reg.Call("c4_rule_add", json.RawMessage(`{
		"name": "log-test",
		"event_pattern": "test.*",
		"action_type": "log",
		"priority": 100
	}`))
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["status"] != "added" {
		t.Errorf("expected status added, got %v", m["status"])
	}

	// List rules
	listResult, err := reg.Call("c4_rule_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	lm := listResult.(map[string]any)
	if lm["count"].(int) != 1 {
		t.Errorf("expected 1 rule, got %v", lm["count"])
	}

	// Remove rule
	removeResult, err := reg.Call("c4_rule_remove", json.RawMessage(`{"name":"log-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	rm := removeResult.(map[string]any)
	if rm["status"] != "removed" {
		t.Errorf("expected status removed, got %v", rm["status"])
	}

	// Verify removed
	listResult2, _ := reg.Call("c4_rule_list", json.RawMessage(`{}`))
	lm2 := listResult2.(map[string]any)
	if lm2["count"].(int) != 0 {
		t.Errorf("expected 0 rules after remove, got %v", lm2["count"])
	}
}

func TestEventBusRuleToggle(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	// Add rule
	reg.Call("c4_rule_add", json.RawMessage(`{
		"name": "toggle-me",
		"event_pattern": "test.*",
		"action_type": "log",
		"enabled": true
	}`))

	// Toggle off
	result, err := reg.Call("c4_rule_toggle", json.RawMessage(`{"name":"toggle-me","enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["status"] != "disabled" {
		t.Errorf("expected status disabled, got %v", m["status"])
	}

	// Verify via list
	listResult, _ := reg.Call("c4_rule_list", json.RawMessage(`{}`))
	lm := listResult.(map[string]any)
	rules := lm["rules"].([]map[string]any)
	if rules[0]["enabled"] != false {
		t.Error("expected rule to be disabled")
	}
}

func TestEventBusPublishValidation(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	// Missing type
	_, err := reg.Call("c4_event_publish", json.RawMessage(`{"source":"test"}`))
	if err == nil {
		t.Error("expected error for missing type")
	}
}

func TestEventBusRuleAddValidation(t *testing.T) {
	reg, cleanup := setupEventBusTest(t)
	defer cleanup()

	// Missing name
	_, err := reg.Call("c4_rule_add", json.RawMessage(`{"event_pattern":"*","action_type":"log"}`))
	if err == nil {
		t.Error("expected error for missing name")
	}
}
