package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func startTestWSBridge(t *testing.T) (*WSBridge, *Server, *Store, int) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	dispatcher := NewDispatcher(store)
	srv := NewServer(ServerConfig{Store: store, Dispatcher: dispatcher})

	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	bridge := NewWSBridge(srv, port)

	go func() {
		if err := bridge.Start(); err != nil {
			// Ignore — test may stop the bridge
		}
	}()

	// Wait for the bridge to be ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Cleanup(func() { bridge.Stop() })

	return bridge, srv, store, port
}

func TestWSBridgeConnect(t *testing.T) {
	_, _, _, port := startTestWSBridge(t)

	// Connect via WebSocket
	conn, _, _, err := ws.Dial(context.Background(), fmt.Sprintf("ws://127.0.0.1:%d/ws/events?pattern=*", port))
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close()

	// Connection should succeed without errors
}

func TestWSBridgeReceiveEvent(t *testing.T) {
	_, srv, _, port := startTestWSBridge(t)

	// Connect via WebSocket
	conn, _, _, err := ws.Dial(context.Background(), fmt.Sprintf("ws://127.0.0.1:%d/ws/events?pattern=*", port))
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close()

	// Give the bridge time to register the subscriber
	time.Sleep(100 * time.Millisecond)

	// Publish an event via the server's internal notify
	srv.notifySubscribers("task.completed", &pb.Event{
		Id:            "ev-test-1",
		Type:          "task.completed",
		Source:        "c4.test",
		Data:          []byte(`{"task_id":"T-001"}`),
		CorrelationId: "corr-xyz",
		TimestampMs:   time.Now().UnixMilli(),
	})

	// Read the WebSocket message with timeout
	done := make(chan []byte, 1)
	go func() {
		data, _, err := wsutil.ReadServerData(conn)
		if err != nil {
			return
		}
		done <- data
	}()

	select {
	case data := <-done:
		var wsEv WSEvent
		if err := json.Unmarshal(data, &wsEv); err != nil {
			t.Fatalf("unmarshal ws event: %v", err)
		}
		if wsEv.ID != "ev-test-1" {
			t.Errorf("expected id ev-test-1, got %s", wsEv.ID)
		}
		if wsEv.Type != "task.completed" {
			t.Errorf("expected type task.completed, got %s", wsEv.Type)
		}
		if wsEv.CorrelationID != "corr-xyz" {
			t.Errorf("expected correlation_id corr-xyz, got %s", wsEv.CorrelationID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for WebSocket event")
	}
}

func TestWSBridgePatternFilter(t *testing.T) {
	_, srv, _, port := startTestWSBridge(t)

	// Connect with pattern "task.*"
	conn, _, _, err := ws.Dial(context.Background(), fmt.Sprintf("ws://127.0.0.1:%d/ws/events?pattern=task.*", port))
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Send a "drive.uploaded" event — should NOT be received by this client
	srv.notifySubscribers("drive.uploaded", &pb.Event{
		Id:          "ev-drive",
		Type:        "drive.uploaded",
		Source:      "c4.drive",
		Data:        []byte(`{}`),
		TimestampMs: time.Now().UnixMilli(),
	})

	// Send a "task.completed" event — should be received
	srv.notifySubscribers("task.completed", &pb.Event{
		Id:          "ev-task",
		Type:        "task.completed",
		Source:      "c4.core",
		Data:        []byte(`{"task_id":"T-002"}`),
		TimestampMs: time.Now().UnixMilli(),
	})

	done := make(chan WSEvent, 2)
	go func() {
		for {
			data, _, err := wsutil.ReadServerData(conn)
			if err != nil {
				return
			}
			var wsEv WSEvent
			if err := json.Unmarshal(data, &wsEv); err != nil {
				continue
			}
			done <- wsEv
		}
	}()

	select {
	case ev := <-done:
		if ev.Type != "task.completed" {
			t.Errorf("expected first event task.completed, got %s", ev.Type)
		}
		if ev.ID != "ev-task" {
			t.Errorf("expected id ev-task, got %s", ev.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for filtered WebSocket event")
	}

	// Verify no more events (drive.uploaded should NOT appear)
	select {
	case ev := <-done:
		t.Errorf("unexpected extra event: %s", ev.Type)
	case <-time.After(200 * time.Millisecond):
		// Good — no extra events
	}
}
