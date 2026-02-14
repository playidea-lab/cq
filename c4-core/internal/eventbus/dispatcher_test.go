package eventbus

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDispatchLog(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	s.AddRule("log-all", "*", "", "log", "", true, 0)

	evID, _ := s.StoreEvent("test.event", "test", json.RawMessage(`{"key":"value"}`), "")
	d.DispatchSync(evID, "test.event", json.RawMessage(`{"key":"value"}`))

	// Verify log entry was created
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log WHERE event_id = ?`, evID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 log entry, got %d", count)
	}
}

func TestDispatchWebhook(t *testing.T) {
	var received []byte
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	s := tempStore(t)
	d := NewDispatcher(s)

	cfg := fmt.Sprintf(`{"url":"%s"}`, ts.URL)
	s.AddRule("webhook-test", "drive.uploaded", "", "webhook", cfg, true, 0)

	evData := json.RawMessage(`{"path":"/test.pdf"}`)
	evID, _ := s.StoreEvent("drive.uploaded", "c4.drive", evData, "")
	d.DispatchSync(evID, "drive.uploaded", evData)

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("webhook was not called")
	}

	var payload map[string]any
	json.Unmarshal(received, &payload)
	if payload["type"] != "drive.uploaded" {
		t.Errorf("expected type drive.uploaded, got %v", payload["type"])
	}
}

func TestDispatchRPC(t *testing.T) {
	// Start a fake JSON-RPC server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var rpcReq []byte
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		rpcReq = buf[:n]
		conn.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}` + "\n"))
	}()

	s := tempStore(t)
	d := NewDispatcher(s)
	d.SetRPCAddr(ln.Addr().String())

	cfg := `{"method":"C2ParseDocument","args_template":{"file_path":"{{data.path}}"}}`
	s.AddRule("rpc-test", "drive.uploaded", "", "rpc", cfg, true, 0)

	evData := json.RawMessage(`{"path":"/docs/paper.pdf"}`)
	evID, _ := s.StoreEvent("drive.uploaded", "c4.drive", evData, "")
	d.DispatchSync(evID, "drive.uploaded", evData)

	wg.Wait()

	if len(rpcReq) == 0 {
		t.Fatal("RPC server was not called")
	}

	var req map[string]any
	json.Unmarshal(rpcReq, &req)
	if req["method"] != "C2ParseDocument" {
		t.Errorf("expected method C2ParseDocument, got %v", req["method"])
	}
	params, _ := req["params"].(map[string]any)
	if params["file_path"] != "/docs/paper.pdf" {
		t.Errorf("expected file_path /docs/paper.pdf, got %v", params["file_path"])
	}
}

func TestDispatchFilterMatch(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	s.AddRule("pdf-only", "drive.uploaded", `{"content_type":"application/pdf"}`, "log", "", true, 0)

	// Should match
	evData := json.RawMessage(`{"content_type":"application/pdf","path":"/doc.pdf"}`)
	evID, _ := s.StoreEvent("drive.uploaded", "c4.drive", evData, "")
	d.DispatchSync(evID, "drive.uploaded", evData)

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log WHERE event_id = ?`, evID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 log entry for matching event, got %d", count)
	}

	// Should NOT match
	evData2 := json.RawMessage(`{"content_type":"image/png","path":"/img.png"}`)
	evID2, _ := s.StoreEvent("drive.uploaded", "c4.drive", evData2, "")
	d.DispatchSync(evID2, "drive.uploaded", evData2)

	s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log WHERE event_id = ?`, evID2).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 log entries for non-matching event, got %d", count)
	}
}

func TestDispatchNoRules(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	// Dispatch with no rules should not error
	d.DispatchSync("ev-1", "test.event", json.RawMessage(`{}`))
}

func TestEvaluateFilter(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		{`{"type":"pdf"}`, `{"type":"pdf","size":100}`, true},
		{`{"type":"pdf"}`, `{"type":"doc","size":100}`, false},
		{`{"a":"1","b":"2"}`, `{"a":"1","b":"2","c":"3"}`, true},
		{`{"a":"1","b":"2"}`, `{"a":"1"}`, false},
		{`{}`, `{"any":"data"}`, true},
	}

	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestResolveTemplate(t *testing.T) {
	data := json.RawMessage(`{"path":"/docs/paper.pdf","size":1024}`)
	tpl := map[string]any{
		"file_path": "{{data.path}}",
		"msg":       "File {{data.path}} of size {{data.size}}",
		"static":    "no-template",
	}

	result := resolveTemplate(tpl, data)
	if result["file_path"] != "/docs/paper.pdf" {
		t.Errorf("expected /docs/paper.pdf, got %v", result["file_path"])
	}
	if result["msg"] != "File /docs/paper.pdf of size 1024" {
		t.Errorf("unexpected msg: %v", result["msg"])
	}
	if result["static"] != "no-template" {
		t.Errorf("static value should be unchanged")
	}
}

func TestDispatchWebhookError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()

	s := tempStore(t)
	d := NewDispatcher(s)

	cfg := fmt.Sprintf(`{"url":"%s"}`, ts.URL)
	s.AddRule("fail-hook", "test.*", "", "webhook", cfg, true, 0)

	evID, _ := s.StoreEvent("test.fail", "test", json.RawMessage(`{}`), "")
	d.DispatchSync(evID, "test.fail", json.RawMessage(`{}`))

	// Verify error was logged
	var status string
	s.db.QueryRow(`SELECT status FROM c4_event_log WHERE event_id = ?`, evID).Scan(&status)
	if status != "error" {
		t.Errorf("expected error status, got %s", status)
	}
}

// mockC1Poster records AutoPost calls for testing.
type mockC1Poster struct {
	mu       sync.Mutex
	channels []string
	messages []string
}

func (m *mockC1Poster) AutoPost(channel, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, channel)
	m.messages = append(m.messages, content)
	return nil
}

func TestDispatchC1Post(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	poster := &mockC1Poster{}
	d.SetC1Poster(poster)

	cfg := `{"channel":"#updates","template":"[{{event_type}}] {{task_id}}: {{title}}"}`
	s.AddRule("c1-tasks", "task.*", "", "c1_post", cfg, true, 0)

	evData := json.RawMessage(`{"task_id":"T-001-0","title":"Implement feature X","worker_id":"w-abc"}`)
	evID, _ := s.StoreEvent("task.completed", "c4.core", evData, "")
	d.DispatchSync(evID, "task.completed", evData)

	poster.mu.Lock()
	defer poster.mu.Unlock()

	if len(poster.messages) != 1 {
		t.Fatalf("expected 1 c1_post call, got %d", len(poster.messages))
	}
	if poster.channels[0] != "#updates" {
		t.Errorf("expected channel #updates, got %s", poster.channels[0])
	}
	want := "[completed] T-001-0: Implement feature X"
	if poster.messages[0] != want {
		t.Errorf("expected message %q, got %q", want, poster.messages[0])
	}

	// Verify dispatch log entry was created with success
	var logStatus string
	s.db.QueryRow(`SELECT status FROM c4_event_log WHERE event_id = ?`, evID).Scan(&logStatus)
	if logStatus != "ok" {
		t.Errorf("expected log status ok, got %s", logStatus)
	}
}

func TestDispatchC1PostDefaultTemplate(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	poster := &mockC1Poster{}
	d.SetC1Poster(poster)

	// No template — should use default format
	s.AddRule("c1-default", "task.*", "", "c1_post", `{"channel":"#dev"}`, true, 0)

	evData := json.RawMessage(`{"task_id":"T-002-0","title":"Fix bug"}`)
	evID, _ := s.StoreEvent("task.started", "c4.core", evData, "")
	d.DispatchSync(evID, "task.started", evData)

	poster.mu.Lock()
	defer poster.mu.Unlock()

	if len(poster.messages) != 1 {
		t.Fatalf("expected 1 c1_post call, got %d", len(poster.messages))
	}
	if poster.channels[0] != "#dev" {
		t.Errorf("expected channel #dev, got %s", poster.channels[0])
	}
	want := "[started] T-002-0: Fix bug"
	if poster.messages[0] != want {
		t.Errorf("expected message %q, got %q", want, poster.messages[0])
	}
}

func TestDispatchC1PostNoPoster(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)
	// No poster set

	s.AddRule("c1-no-poster", "task.*", "", "c1_post", `{"channel":"#updates"}`, true, 0)

	evData := json.RawMessage(`{"task_id":"T-003-0","title":"Test"}`)
	evID, _ := s.StoreEvent("task.completed", "c4.core", evData, "")
	d.DispatchSync(evID, "task.completed", evData)

	// Should log error (poster not configured)
	var logStatus string
	s.db.QueryRow(`SELECT status FROM c4_event_log WHERE event_id = ?`, evID).Scan(&logStatus)
	if logStatus != "error" {
		t.Errorf("expected error status when poster not set, got %s", logStatus)
	}
}
