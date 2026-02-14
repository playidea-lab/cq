package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

// mockRestarter implements Restarter for testing.
type mockRestarter struct {
	called  int
	newAddr string
	err     error
}

func (m *mockRestarter) Restart() (string, error) {
	m.called++
	if m.err != nil {
		return "", m.err
	}
	return m.newAddr, nil
}

// startMockSidecar starts a minimal JSON-RPC server that responds to any method.
func startMockSidecar(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				resp := map[string]any{
					"result": map[string]any{"status": "ok"},
					"error":  nil,
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

// TestProxyCallSuccess verifies normal proxy call works.
func TestProxyCallSuccess(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	result, err := proxy.Call("Ping", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected ok, got %v", result["status"])
	}
}

// TestProxyCallEmptyAddr verifies empty addr fails immediately.
func TestProxyCallEmptyAddr(t *testing.T) {
	proxy := NewBridgeProxy("")
	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty addr")
	}
}

// TestProxyAutoRestartSuccess verifies: conn fail → restart → retry succeeds.
func TestProxyAutoRestartSuccess(t *testing.T) {
	// Start a mock sidecar that the restarter will "switch to"
	goodAddr, cleanup := startMockSidecar(t)
	defer cleanup()

	// Start proxy pointing to a dead address
	proxy := NewBridgeProxy("127.0.0.1:1") // dead

	restarter := &mockRestarter{newAddr: goodAddr}
	proxy.SetRestarter(restarter)

	result, err := proxy.Call("Ping", map[string]any{})
	if err != nil {
		t.Fatalf("Call should succeed after restart, got: %v", err)
	}
	if restarter.called != 1 {
		t.Fatalf("expected 1 restart, got %d", restarter.called)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected ok, got %v", result["status"])
	}
}

// TestProxyAutoRestartFails verifies: conn fail → restart fails → original error returned.
func TestProxyAutoRestartFails(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:1") // dead

	restarter := &mockRestarter{err: fmt.Errorf("restart failed")}
	proxy.SetRestarter(restarter)

	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error when restart fails")
	}
	if restarter.called != 1 {
		t.Fatalf("expected 1 restart attempt, got %d", restarter.called)
	}
}

// TestProxyNoRestartOnBridgeError verifies: bridge-level error (not conn) doesn't trigger restart.
func TestProxyNoRestartOnBridgeError(t *testing.T) {
	// Start a mock sidecar that returns an error response
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				if n == 0 {
					return
				}
				errMsg := "method not found"
				resp := map[string]any{
					"result": nil,
					"error":  &errMsg,
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	proxy := NewBridgeProxy(ln.Addr().String())
	restarter := &mockRestarter{newAddr: ln.Addr().String()}
	proxy.SetRestarter(restarter)

	_, err = proxy.Call("Unknown", map[string]any{})
	if err == nil {
		t.Fatal("expected bridge error")
	}
	// Bridge error (not conn error) should NOT trigger restart
	if restarter.called != 0 {
		t.Fatalf("restart should not be called for bridge errors, got %d calls", restarter.called)
	}
}

// TestProxyIsAvailable verifies IsAvailable checks.
func TestProxyIsAvailable(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	if !proxy.IsAvailable() {
		t.Fatal("expected available")
	}

	emptyProxy := NewBridgeProxy("")
	if emptyProxy.IsAvailable() {
		t.Fatal("expected unavailable for empty addr")
	}
}

// TestProxyUpdateAddr verifies addr can be updated.
func TestProxyUpdateAddr(t *testing.T) {
	proxy := NewBridgeProxy("old:1234")
	proxy.UpdateAddr("new:5678")

	proxy.mu.Lock()
	got := proxy.addr
	proxy.mu.Unlock()

	if got != "new:5678" {
		t.Fatalf("expected new:5678, got %s", got)
	}
}

// TestProxyNoRestarterConfigured verifies graceful handling when no restarter set.
func TestProxyNoRestarterConfigured(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:1") // dead, no restarter
	_, err := proxy.Call("Ping", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should fail without panic (no restarter = no retry)
}

// =========================================================================
// mockKnowledgeSyncer for pull handler tests
// =========================================================================

type mockKnowledgeSyncer struct {
	listDocs  []map[string]any
	listErr   error
	getDocs   map[string]map[string]any // docID → full doc
	getErr    error
	syncCalls int
}

func (m *mockKnowledgeSyncer) SyncDocument(params map[string]any, docID string) error {
	m.syncCalls++
	return nil
}

func (m *mockKnowledgeSyncer) SearchDocuments(query string, docType string, limit int) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockKnowledgeSyncer) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listDocs, nil
}

func (m *mockKnowledgeSyncer) GetDocument(docID string) (map[string]any, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if doc, ok := m.getDocs[docID]; ok {
		return doc, nil
	}
	return nil, fmt.Errorf("not found: %s", docID)
}

// startMockKnowledgeSidecar starts a mock sidecar that handles KnowledgeGet and KnowledgeRecord.
// getResponses maps doc_id → response (nil means doc not found locally).
// recordSuccess controls whether KnowledgeRecord returns success.
func startMockKnowledgeSidecar(t *testing.T, getResponses map[string]map[string]any, recordSuccess bool) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 65536)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}

				var req struct {
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				json.Unmarshal(buf[:n], &req)

				var result map[string]any
				switch req.Method {
				case "KnowledgeGet":
					docID, _ := req.Params["doc_id"].(string)
					if resp, ok := getResponses[docID]; ok && resp != nil {
						result = resp
					} else {
						result = map[string]any{"error": "not found"}
					}
				case "KnowledgeRecord":
					result = map[string]any{"success": recordSuccess, "doc_id": req.Params["id"]}
				default:
					result = map[string]any{"status": "ok"}
				}

				resp := map[string]any{"result": result, "error": nil}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

// TestKnowledgePull_NewDocs verifies pulling new documents from cloud.
func TestKnowledgePull_NewDocs(t *testing.T) {
	// Mock sidecar: no local docs exist
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{
			{"doc_id": "exp-001", "version": float64(1)},
			{"doc_id": "exp-002", "version": float64(1)},
		},
		getDocs: map[string]map[string]any{
			"exp-001": {"doc_id": "exp-001", "doc_type": "experiment", "title": "Exp 1", "body": "Body 1", "domain": "ml", "tags": `["a"]`},
			"exp-002": {"doc_id": "exp-002", "doc_type": "experiment", "title": "Exp 2", "body": "Body 2", "domain": "ml", "tags": `["b"]`},
		},
	}

	handler := knowledgePullHandler(proxy, kc)
	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("pull failed: %v", err)
	}

	m := result.(map[string]any)
	if m["pulled"] != 2 {
		t.Errorf("pulled = %v, want 2", m["pulled"])
	}
	if m["skipped"] != 0 {
		t.Errorf("skipped = %v, want 0", m["skipped"])
	}
}

// TestKnowledgePull_SkipsExistingWithSameVersion verifies version-based skip.
func TestKnowledgePull_SkipsExistingWithSameVersion(t *testing.T) {
	// Mock sidecar: exp-001 exists locally with version 1
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{
		"exp-001": {"version": float64(1), "doc_id": "exp-001"},
	}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{
			{"doc_id": "exp-001", "version": float64(1)}, // same version
		},
		getDocs: map[string]map[string]any{
			"exp-001": {"doc_id": "exp-001", "doc_type": "experiment", "title": "Exp 1", "body": "Body 1"},
		},
	}

	handler := knowledgePullHandler(proxy, kc)
	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("pull failed: %v", err)
	}

	m := result.(map[string]any)
	if m["pulled"] != 0 {
		t.Errorf("pulled = %v, want 0", m["pulled"])
	}
	if m["skipped"] != 1 {
		t.Errorf("skipped = %v, want 1", m["skipped"])
	}
}

// TestKnowledgePull_UpdatesNewerVersion verifies cloud-newer docs get updated.
func TestKnowledgePull_UpdatesNewerVersion(t *testing.T) {
	// Mock sidecar: exp-001 exists locally with version 1
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{
		"exp-001": {"version": float64(1), "doc_id": "exp-001"},
	}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{
			{"doc_id": "exp-001", "version": float64(2)}, // newer version
		},
		getDocs: map[string]map[string]any{
			"exp-001": {"doc_id": "exp-001", "doc_type": "experiment", "title": "Updated", "body": "New body"},
		},
	}

	handler := knowledgePullHandler(proxy, kc)
	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("pull failed: %v", err)
	}

	m := result.(map[string]any)
	if m["updated"] != 1 {
		t.Errorf("updated = %v, want 1", m["updated"])
	}
}

// TestKnowledgePull_ForceOverwrite verifies force mode overwrites existing docs.
func TestKnowledgePull_ForceOverwrite(t *testing.T) {
	// Mock sidecar: exp-001 exists locally with version 1
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{
		"exp-001": {"version": float64(1), "doc_id": "exp-001"},
	}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{
			{"doc_id": "exp-001", "version": float64(1)}, // same version, but force
		},
		getDocs: map[string]map[string]any{
			"exp-001": {"doc_id": "exp-001", "doc_type": "experiment", "title": "Forced", "body": "Forced body"},
		},
	}

	handler := knowledgePullHandler(proxy, kc)
	result, err := handler(json.RawMessage(`{"force": true}`))
	if err != nil {
		t.Fatalf("pull failed: %v", err)
	}

	m := result.(map[string]any)
	if m["updated"] != 1 {
		t.Errorf("updated = %v, want 1 (force overwrite)", m["updated"])
	}
	if m["skipped"] != 0 {
		t.Errorf("skipped = %v, want 0 (force mode)", m["skipped"])
	}
}

// TestKnowledgePull_CloudNotConfigured verifies error when cloud is nil.
func TestKnowledgePull_CloudNotConfigured(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:0")
	handler := knowledgePullHandler(proxy, nil)
	_, err := handler(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for nil cloud")
	}
}

// TestKnowledgePull_CloudListError verifies error propagation from cloud list.
func TestKnowledgePull_CloudListError(t *testing.T) {
	proxy := NewBridgeProxy("127.0.0.1:0")
	kc := &mockKnowledgeSyncer{
		listErr: fmt.Errorf("network timeout"),
	}

	handler := knowledgePullHandler(proxy, kc)
	_, err := handler(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error on cloud list failure")
	}
}

// TestKnowledgePull_GetDocumentError verifies partial failure handling.
func TestKnowledgePull_GetDocumentError(t *testing.T) {
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{
			{"doc_id": "exp-001", "version": float64(1)},
			{"doc_id": "exp-fail", "version": float64(1)},
		},
		getDocs: map[string]map[string]any{
			"exp-001": {"doc_id": "exp-001", "doc_type": "experiment", "title": "OK", "body": "Body"},
			// exp-fail not in getDocs → GetDocument returns error
		},
	}

	handler := knowledgePullHandler(proxy, kc)
	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("pull should not fail completely: %v", err)
	}

	m := result.(map[string]any)
	if m["pulled"] != 1 {
		t.Errorf("pulled = %v, want 1", m["pulled"])
	}
	errors := m["errors"].([]string)
	if len(errors) != 1 {
		t.Errorf("errors = %v, want 1 error", errors)
	}
}

// TestKnowledgePull_Registration verifies the tool is registered properly.
func TestKnowledgePull_Registration(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	kc := &mockKnowledgeSyncer{}
	reg := mcp.NewRegistry()

	RegisterProxyHandlers(reg, proxy, kc)

	tools := reg.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "c4_knowledge_pull" {
			found = true
			break
		}
	}
	if !found {
		t.Error("c4_knowledge_pull not found in registered tools")
	}
}

// TestKnowledgePull_DocTypeFilter verifies doc_type parameter is passed to cloud.
func TestKnowledgePull_DocTypeFilter(t *testing.T) {
	addr, cleanup := startMockKnowledgeSidecar(t, map[string]map[string]any{}, true)
	defer cleanup()

	proxy := NewBridgeProxy(addr)

	var capturedDocType string
	kc := &mockKnowledgeSyncer{
		listDocs: []map[string]any{},
	}
	// Override ListDocuments to capture docType
	origList := kc.listDocs
	_ = origList

	// Use a custom syncer to verify filtering
	customKC := &filterCaptureSyncer{docType: &capturedDocType}

	handler := knowledgePullHandler(proxy, customKC)
	handler(json.RawMessage(`{"doc_type": "pattern"}`))

	if capturedDocType != "pattern" {
		t.Errorf("doc_type = %q, want %q", capturedDocType, "pattern")
	}
}

// filterCaptureSyncer captures the doc_type passed to ListDocuments.
type filterCaptureSyncer struct {
	docType *string
}

func (f *filterCaptureSyncer) SyncDocument(params map[string]any, docID string) error { return nil }
func (f *filterCaptureSyncer) SearchDocuments(query string, docType string, limit int) ([]map[string]any, error) {
	return nil, nil
}
func (f *filterCaptureSyncer) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	*f.docType = docType
	return []map[string]any{}, nil
}
func (f *filterCaptureSyncer) GetDocument(docID string) (map[string]any, error) {
	return nil, fmt.Errorf("not found")
}

// =========================================================================
// Event extraction tests (publishSidecarEvents / SetEventBus)
// =========================================================================

// mockPublisher records PublishAsync calls for assertions.
type mockPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	evType    string
	source    string
	data      json.RawMessage
	projectID string
}

func (m *mockPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{evType, source, data, projectID})
}

func (m *mockPublisher) getEvents() []publishedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]publishedEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

// startMockSidecarWithEvents starts a mock sidecar that returns _events in the response.
func startMockSidecarWithEvents(t *testing.T, events []map[string]any) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				result := map[string]any{
					"status":  "ok",
					"_events": events,
				}
				resp := map[string]any{
					"result": result,
					"error":  nil,
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

// TestProxyEventExtraction verifies _events are extracted and published.
func TestProxyEventExtraction(t *testing.T) {
	events := []map[string]any{
		{
			"type":       "c2.document.parsed",
			"source":     "c4.c2",
			"data":       map[string]any{"file_path": "/a/b.pdf", "block_count": float64(5)},
			"project_id": "proj-1",
		},
	}
	addr, cleanup := startMockSidecarWithEvents(t, events)
	defer cleanup()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(addr)
	proxy.SetEventBus(pub)

	result, err := proxy.Call("C2ParseDocument", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// _events should be removed from result
	if _, exists := result["_events"]; exists {
		t.Error("_events should be stripped from result")
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}

	// Publisher should have received the event
	published := pub.getEvents()
	if len(published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(published))
	}
	if published[0].evType != "c2.document.parsed" {
		t.Errorf("evType = %q, want c2.document.parsed", published[0].evType)
	}
	if published[0].source != "c4.c2" {
		t.Errorf("source = %q, want c4.c2", published[0].source)
	}
	if published[0].projectID != "proj-1" {
		t.Errorf("projectID = %q, want proj-1", published[0].projectID)
	}
}

// TestProxyEventExtractionMultiple verifies multiple events are all published.
func TestProxyEventExtractionMultiple(t *testing.T) {
	events := []map[string]any{
		{"type": "ev.a", "source": "src-a", "data": map[string]any{}},
		{"type": "ev.b", "source": "src-b", "data": map[string]any{"k": "v"}},
	}
	addr, cleanup := startMockSidecarWithEvents(t, events)
	defer cleanup()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(addr)
	proxy.SetEventBus(pub)

	_, err := proxy.Call("Test", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	published := pub.getEvents()
	if len(published) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(published))
	}
	if published[0].evType != "ev.a" {
		t.Errorf("first event type = %q, want ev.a", published[0].evType)
	}
	if published[1].evType != "ev.b" {
		t.Errorf("second event type = %q, want ev.b", published[1].evType)
	}
}

// TestProxyNoEventBus verifies _events are still stripped even without a publisher.
func TestProxyNoEventBus(t *testing.T) {
	events := []map[string]any{
		{"type": "ev.x", "source": "src", "data": map[string]any{}},
	}
	addr, cleanup := startMockSidecarWithEvents(t, events)
	defer cleanup()

	proxy := NewBridgeProxy(addr) // no SetEventBus call

	result, err := proxy.Call("Test", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// _events should still be removed
	if _, exists := result["_events"]; exists {
		t.Error("_events should be stripped even without publisher")
	}
}

// TestProxyNoEvents verifies normal responses without _events work fine.
func TestProxyNoEvents(t *testing.T) {
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(addr)
	proxy.SetEventBus(pub)

	result, err := proxy.Call("Ping", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}

	// No events should be published
	if len(pub.getEvents()) != 0 {
		t.Error("expected no published events")
	}
}

// TestProxyEventSkipsEmptyType verifies events without a type are skipped.
func TestProxyEventSkipsEmptyType(t *testing.T) {
	events := []map[string]any{
		{"type": "", "source": "src", "data": map[string]any{}},
		{"type": "valid.event", "source": "src", "data": map[string]any{}},
	}
	addr, cleanup := startMockSidecarWithEvents(t, events)
	defer cleanup()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(addr)
	proxy.SetEventBus(pub)

	_, err := proxy.Call("Test", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	published := pub.getEvents()
	if len(published) != 1 {
		t.Fatalf("expected 1 published event (empty type skipped), got %d", len(published))
	}
	if published[0].evType != "valid.event" {
		t.Errorf("evType = %q, want valid.event", published[0].evType)
	}
}

// TestProxyEventNonArrayEvents verifies _events as non-array (string, object) is ignored gracefully.
func TestProxyEventNonArrayEvents(t *testing.T) {
	// _events is a string instead of array — should be stripped but not crash
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				if n == 0 {
					return
				}
				// _events is a string, not an array
				result := map[string]any{
					"status":  "ok",
					"_events": "not-an-array",
				}
				resp := map[string]any{"result": result, "error": nil}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				c.Write(data)
			}(conn)
		}
	}()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(ln.Addr().String())
	proxy.SetEventBus(pub)

	result, err := proxy.Call("Test", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// _events should be stripped
	if _, exists := result["_events"]; exists {
		t.Error("_events should be stripped even when non-array")
	}
	// No events should be published (type assertion to []any fails)
	if len(pub.getEvents()) != 0 {
		t.Errorf("expected 0 published events for non-array _events, got %d", len(pub.getEvents()))
	}
}

// TestProxyEventNilData verifies events with missing/nil data produce valid JSON ("null").
func TestProxyEventNilData(t *testing.T) {
	events := []map[string]any{
		{"type": "test.nil_data", "source": "src", "project_id": ""},
		// note: no "data" key at all
	}
	addr, cleanup := startMockSidecarWithEvents(t, events)
	defer cleanup()

	pub := &mockPublisher{}
	proxy := NewBridgeProxy(addr)
	proxy.SetEventBus(pub)

	_, err := proxy.Call("Test", map[string]any{})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	published := pub.getEvents()
	if len(published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(published))
	}
	// json.Marshal(nil) → "null" — verify it doesn't crash and produces valid JSON
	if published[0].evType != "test.nil_data" {
		t.Errorf("evType = %q, want test.nil_data", published[0].evType)
	}
	if string(published[0].data) != "null" {
		t.Errorf("data = %q, want \"null\" for nil data", string(published[0].data))
	}
}
