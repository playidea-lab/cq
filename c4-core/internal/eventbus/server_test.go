package eventbus

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startTestServer creates a gRPC server on a temp Unix socket and returns a client + cleanup.
func startTestServer(t *testing.T) (pb.EventBusClient, func()) {
	t.Helper()

	store := tempStore(t)
	dispatcher := NewDispatcher(store)
	srv := NewServer(ServerConfig{
		Store:      store,
		Dispatcher: dispatcher,
	})

	sockPath := filepath.Join(t.TempDir(), "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterEventBusServer(grpcServer, srv)

	go grpcServer.Serve(ln)

	conn, err := grpc.NewClient("unix:"+sockPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}

	client := pb.NewEventBusClient(conn)
	cleanup := func() {
		conn.Close()
		grpcServer.GracefulStop()
		ln.Close()
	}

	return client, cleanup
}

func TestServerPublishAndList(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Publish an event
	resp, err := client.Publish(ctx, &pb.Event{
		Type:   "test.hello",
		Source: "unit-test",
		Data:   []byte(`{"message":"world"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.EventId == "" {
		t.Error("expected non-empty event ID")
	}

	// List events
	listResp, err := client.ListEvents(ctx, &pb.ListEventsRequest{
		Type:  "test.hello",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if listResp.Total != 1 {
		t.Errorf("expected 1 event, got %d", listResp.Total)
	}
	if listResp.Events[0].Type != "test.hello" {
		t.Errorf("expected type test.hello, got %s", listResp.Events[0].Type)
	}
}

func TestServerPublishValidation(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Missing type should fail
	_, err := client.Publish(ctx, &pb.Event{Source: "test"})
	if err == nil {
		t.Error("expected error for missing event type")
	}
}

func TestServerRulesCRUD(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add rule
	addResp, err := client.AddRule(ctx, &pb.Rule{
		Name:         "test-rule",
		EventPattern: "test.*",
		ActionType:   "log",
		Enabled:      true,
		Priority:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !addResp.Ok {
		t.Error("expected ok=true for AddRule")
	}

	// List rules
	listResp, err := client.ListRules(ctx, &pb.ListRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listResp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(listResp.Rules))
	}
	if listResp.Rules[0].Name != "test-rule" {
		t.Errorf("expected rule name test-rule, got %s", listResp.Rules[0].Name)
	}

	// Remove rule
	removeResp, err := client.RemoveRule(ctx, &pb.RemoveRuleRequest{Name: "test-rule"})
	if err != nil {
		t.Fatal(err)
	}
	if !removeResp.Ok {
		t.Error("expected ok=true for RemoveRule")
	}

	// Verify removed
	listResp2, _ := client.ListRules(ctx, &pb.ListRulesRequest{})
	if len(listResp2.Rules) != 0 {
		t.Errorf("expected 0 rules after remove, got %d", len(listResp2.Rules))
	}
}

func TestServerAddRuleValidation(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Missing name
	_, err := client.AddRule(ctx, &pb.Rule{EventPattern: "test.*", ActionType: "log"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing pattern
	_, err = client.AddRule(ctx, &pb.Rule{Name: "test", ActionType: "log"})
	if err == nil {
		t.Error("expected error for missing pattern")
	}

	// Missing action type
	_, err = client.AddRule(ctx, &pb.Rule{Name: "test", EventPattern: "test.*"})
	if err == nil {
		t.Error("expected error for missing action type")
	}
}

func TestServerSubscribe(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start subscriber
	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{EventPattern: "test.*"})
	if err != nil {
		t.Fatal(err)
	}

	// Give subscriber time to register
	time.Sleep(50 * time.Millisecond)

	// Publish event
	_, err = client.Publish(ctx, &pb.Event{
		Type:   "test.hello",
		Source: "unit-test",
		Data:   []byte(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Receive from stream
	ev, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "test.hello" {
		t.Errorf("expected type test.hello, got %s", ev.Type)
	}

	var data map[string]any
	json.Unmarshal(ev.Data, &data)
	if data["key"] != "value" {
		t.Errorf("expected data key=value, got %v", data)
	}
}

func TestServerToggleRule(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add a rule
	_, err := client.AddRule(ctx, &pb.Rule{
		Name: "toggle-me", EventPattern: "test.*", ActionType: "log", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Toggle off
	resp, err := client.ToggleRule(ctx, &pb.ToggleRuleRequest{Name: "toggle-me", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}

	// Verify disabled
	rules, _ := client.ListRules(ctx, &pb.ListRulesRequest{})
	if len(rules.Rules) != 1 || rules.Rules[0].Enabled {
		t.Error("expected rule to be disabled")
	}

	// Toggle on
	_, err = client.ToggleRule(ctx, &pb.ToggleRuleRequest{Name: "toggle-me", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	rules, _ = client.ListRules(ctx, &pb.ListRulesRequest{})
	if !rules.Rules[0].Enabled {
		t.Error("expected rule to be enabled")
	}
}

func TestServerToggleRuleNotFound(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	_, err := client.ToggleRule(context.Background(), &pb.ToggleRuleRequest{Name: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestServerListLogs(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add a log rule
	_, _ = client.AddRule(ctx, &pb.Rule{
		Name: "log-for-audit", EventPattern: "*", ActionType: "log", Enabled: true,
	})

	// Publish an event (triggers dispatch)
	_, _ = client.Publish(ctx, &pb.Event{
		Type: "test.audit", Source: "unit-test", Data: []byte(`{}`),
	})

	// Wait for async dispatch
	time.Sleep(100 * time.Millisecond)

	// List logs
	logsResp, err := client.ListLogs(ctx, &pb.ListLogsRequest{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if logsResp.Total < 1 {
		t.Errorf("expected at least 1 log entry, got %d", logsResp.Total)
	}
}

func TestServerGetStats(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Publish some events
	client.Publish(ctx, &pb.Event{Type: "test.a", Source: "test", Data: []byte(`{}`)})
	client.Publish(ctx, &pb.Event{Type: "test.b", Source: "test", Data: []byte(`{}`)})
	client.AddRule(ctx, &pb.Rule{Name: "stat-rule", EventPattern: "*", ActionType: "log", Enabled: true})

	stats, err := client.GetStats(ctx, &pb.GetStatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventCount < 2 {
		t.Errorf("expected event_count >= 2, got %d", stats.EventCount)
	}
	if stats.RuleCount < 1 {
		t.Errorf("expected rule_count >= 1, got %d", stats.RuleCount)
	}
}

func TestServerReplayEvents(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Publish events
	client.Publish(ctx, &pb.Event{Type: "test.replay", Source: "test", Data: []byte(`{"n":1}`)})
	client.Publish(ctx, &pb.Event{Type: "test.replay", Source: "test", Data: []byte(`{"n":2}`)})
	client.Publish(ctx, &pb.Event{Type: "test.other", Source: "test", Data: []byte(`{"n":3}`)})

	// Replay with type filter + dry_run
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	stream, err := client.ReplayEvents(rctx, &pb.ReplayRequest{
		EventType: "test.replay",
		DryRun:    true,
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}

	var replayed []*pb.Event
	for {
		ev, err := stream.Recv()
		if err != nil {
			break
		}
		replayed = append(replayed, ev)
	}

	if len(replayed) != 2 {
		t.Errorf("expected 2 replayed events, got %d", len(replayed))
	}
}

func TestServerReplayDryRun(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add a log rule to detect dispatch
	client.AddRule(ctx, &pb.Rule{
		Name: "replay-check", EventPattern: "test.*", ActionType: "log", Enabled: true,
	})

	// Publish one event
	client.Publish(ctx, &pb.Event{Type: "test.dry", Source: "test", Data: []byte(`{}`)})
	time.Sleep(100 * time.Millisecond)

	// Check initial log count
	stats1, _ := client.GetStats(ctx, &pb.GetStatsRequest{})
	initialLogs := stats1.LogCount

	// Replay with dry_run=true — should NOT create additional dispatch logs
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	stream, _ := client.ReplayEvents(rctx, &pb.ReplayRequest{
		EventType: "test.dry",
		DryRun:    true,
		Limit:     10,
	})

	for {
		_, err := stream.Recv()
		if err != nil {
			break
		}
	}

	time.Sleep(100 * time.Millisecond)
	stats2, _ := client.GetStats(ctx, &pb.GetStatsRequest{})

	if stats2.LogCount != initialLogs {
		t.Errorf("dry_run should not create new logs: before=%d, after=%d", initialLogs, stats2.LogCount)
	}
}

func TestServerPublishWithRule(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add a log rule
	_, err := client.AddRule(ctx, &pb.Rule{
		Name:         "log-test",
		EventPattern: "test.*",
		ActionType:   "log",
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish - should trigger the log rule
	_, err = client.Publish(ctx, &pb.Event{
		Type:   "test.event",
		Source: "unit-test",
		Data:   []byte(`{"msg":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Give async dispatch time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify event was stored
	listResp, _ := client.ListEvents(ctx, &pb.ListEventsRequest{Type: "test.event", Limit: 10})
	if listResp.Total != 1 {
		t.Errorf("expected 1 event, got %d", listResp.Total)
	}
}

func TestServerListDLQEmpty(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.ListDLQ(context.Background(), &pb.ListDLQRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 DLQ entries, got %d", resp.Total)
	}
}

func TestServerCorrID(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Publish with correlation_id
	_, err := client.Publish(ctx, &pb.Event{
		Type:          "test.corr",
		Source:        "unit-test",
		Data:          []byte(`{}`),
		CorrelationId: "corr-abc-123",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify correlation_id is stored and returned
	listResp, err := client.ListEvents(ctx, &pb.ListEventsRequest{Type: "test.corr", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if listResp.Total != 1 {
		t.Fatalf("expected 1 event, got %d", listResp.Total)
	}
	if listResp.Events[0].CorrelationId != "corr-abc-123" {
		t.Errorf("expected correlation_id corr-abc-123, got %q", listResp.Events[0].CorrelationId)
	}
}

func TestServerRemoveRuleValidation(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	// Empty id and name should fail
	_, err := client.RemoveRule(context.Background(), &pb.RemoveRuleRequest{})
	if err == nil {
		t.Error("expected error for empty id and name")
	}
}

func TestServerSubProjFilter(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Subscribe with project_id filter
	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
		EventPattern: "test.*",
		ProjectId:    "proj-A",
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	// Publish event with different project_id (should NOT be received)
	client.Publish(ctx, &pb.Event{
		Type: "test.filtered", Source: "test", Data: []byte(`{}`),
		ProjectId: "proj-B",
	})

	// Publish event with matching project_id (should be received)
	client.Publish(ctx, &pb.Event{
		Type: "test.matched", Source: "test", Data: []byte(`{}`),
		ProjectId: "proj-A",
	})

	ev, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "test.matched" {
		t.Errorf("expected type test.matched, got %s", ev.Type)
	}
	if ev.ProjectId != "proj-A" {
		t.Errorf("expected project_id proj-A, got %s", ev.ProjectId)
	}
}
