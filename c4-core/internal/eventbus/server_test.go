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
