package eventbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestStartEmbedded(t *testing.T) {
	dir := t.TempDir()

	e, err := StartEmbedded(EmbeddedConfig{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Stop()

	if e.SocketPath() == "" {
		t.Error("expected non-empty socket path")
	}
	if e.Store() == nil {
		t.Error("expected non-nil store")
	}
	if e.Dispatcher() == nil {
		t.Error("expected non-nil dispatcher")
	}
}

func TestEmbeddedPublishAndList(t *testing.T) {
	dir := t.TempDir()

	e, err := StartEmbedded(EmbeddedConfig{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Stop()

	// Connect as a client
	conn, err := grpc.NewClient("unix:"+e.SocketPath(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewEventBusClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Publish
	resp, err := client.Publish(ctx, &pb.Event{
		Type:   "test.embedded",
		Source: "unit-test",
		Data:   []byte(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}

	// List
	listResp, err := client.ListEvents(ctx, &pb.ListEventsRequest{
		Type:  "test.embedded",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if listResp.Total != 1 {
		t.Errorf("expected 1 event, got %d", listResp.Total)
	}

	var data map[string]any
	json.Unmarshal(listResp.Events[0].Data, &data)
	if data["key"] != "value" {
		t.Errorf("expected key=value, got %v", data)
	}
}

func TestEmbeddedWithC1Poster(t *testing.T) {
	dir := t.TempDir()

	e, err := StartEmbedded(EmbeddedConfig{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Stop()

	// Wire a mock C1 poster
	poster := &mockC1Poster{}
	e.Dispatcher().SetC1Poster(poster)

	// Add a c1_post rule directly through the store
	e.Store().AddRule("c1-test", "task.*", "", "c1_post",
		`{"channel":"#test","template":"[{{event_type}}] {{task_id}}"}`, true, 0)

	// Publish via the store + dispatch
	evID, _ := e.Store().StoreEvent("task.completed", "c4.core",
		json.RawMessage(`{"task_id":"T-001-0","title":"Test"}`), "")
	e.Dispatcher().DispatchSync(evID, "task.completed",
		json.RawMessage(`{"task_id":"T-001-0","title":"Test"}`))

	poster.mu.Lock()
	defer poster.mu.Unlock()

	if len(poster.messages) != 1 {
		t.Fatalf("expected 1 c1_post, got %d", len(poster.messages))
	}
	if poster.channels[0] != "#test" {
		t.Errorf("expected channel #test, got %s", poster.channels[0])
	}
}
