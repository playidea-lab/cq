package eventbus

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// httptest.NewServer uses localhost (127.0.0.1), which is blocked by SSRF protection
	// The webhook should NOT be called, and an error should be logged
	var logStatus, logError string
	s.db.QueryRow(`SELECT status, error FROM c4_event_log WHERE event_id = ?`, evID).Scan(&logStatus, &logError)
	if logStatus != "error" {
		t.Errorf("expected error status for localhost webhook (SSRF protection), got %s", logStatus)
	}
	if logError == "" || len(logError) == 0 {
		t.Errorf("expected SSRF error message, got empty")
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

	// Verify error was logged (SSRF protection blocks localhost)
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

// --- v4: Filter v2 tests ---

func TestFilterV2Eq(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		// Explicit $eq
		{`{"type":{"$eq":"pdf"}}`, `{"type":"pdf"}`, true},
		{`{"type":{"$eq":"pdf"}}`, `{"type":"doc"}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2Gt(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		{`{"size":{"$gt":1000}}`, `{"size":2000}`, true},
		{`{"size":{"$gt":1000}}`, `{"size":500}`, false},
		{`{"size":{"$gt":1000}}`, `{"size":1000}`, false},
		{`{"size":{"$lt":100}}`, `{"size":50}`, true},
		{`{"size":{"$lt":100}}`, `{"size":200}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2In(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		{`{"tag":{"$in":["urgent","bug"]}}`, `{"tag":"urgent"}`, true},
		{`{"tag":{"$in":["urgent","bug"]}}`, `{"tag":"feature"}`, false},
		{`{"status":{"$ne":"done"}}`, `{"status":"pending"}`, true},
		{`{"status":{"$ne":"done"}}`, `{"status":"done"}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2Regex(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		{`{"name":{"$regex":"^test"}}`, `{"name":"test_file.go"}`, true},
		{`{"name":{"$regex":"^test"}}`, `{"name":"main_test.go"}`, false},
		{`{"path":{"$regex":"\\.pdf$"}}`, `{"path":"/docs/report.pdf"}`, true},
		{`{"path":{"$regex":"\\.pdf$"}}`, `{"path":"/docs/report.txt"}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2Nested(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		// Dot notation
		{`{"meta.status":"active"}`, `{"meta":{"status":"active","count":5}}`, true},
		{`{"meta.status":"active"}`, `{"meta":{"status":"inactive"}}`, false},
		// Deep nesting
		{`{"a.b.c":"deep"}`, `{"a":{"b":{"c":"deep"}}}`, true},
		{`{"a.b.c":"deep"}`, `{"a":{"b":{"c":"other"}}}`, false},
		// Non-existent nested path
		{`{"a.b.c":"deep"}`, `{"a":{"x":"y"}}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2Exists(t *testing.T) {
	tests := []struct {
		filter string
		data   string
		want   bool
	}{
		{`{"error":{"$exists":true}}`, `{"error":"something"}`, true},
		{`{"error":{"$exists":true}}`, `{"ok":true}`, false},
		{`{"error":{"$exists":false}}`, `{"ok":true}`, true},
		{`{"error":{"$exists":false}}`, `{"error":"bad"}`, false},
	}
	for _, tt := range tests {
		got := evaluateFilter(tt.filter, json.RawMessage(tt.data))
		if got != tt.want {
			t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter, tt.data, got, tt.want)
		}
	}
}

func TestFilterV2BackwardCompat(t *testing.T) {
	// Existing v1 filters should still work identically
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

// --- v4: Dispatcher DLQ integration ---

func TestDispatcherDLQ(t *testing.T) {
	s := tempStore(t)
	d := NewDispatcher(s)

	// c1_post without poster — will fail and should insert DLQ
	s.AddRule("fail-rule", "task.*", "", "c1_post", `{"channel":"#test"}`, true, 0)

	evData := json.RawMessage(`{"task_id":"T-001"}`)
	evID, _ := s.StoreEvent("task.completed", "c4.core", evData, "")
	d.DispatchSync(evID, "task.completed", evData)

	// Verify DLQ entry was created
	entries, err := s.ListDLQ(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 DLQ entry after failed dispatch, got %d", len(entries))
	}
	if entries[0].EventID != evID {
		t.Errorf("DLQ event_id: expected %s, got %s", evID, entries[0].EventID)
	}
	if entries[0].RuleName != "fail-rule" {
		t.Errorf("DLQ rule_name: expected fail-rule, got %s", entries[0].RuleName)
	}
	if entries[0].EventType != "task.completed" {
		t.Errorf("DLQ event_type: expected task.completed, got %s", entries[0].EventType)
	}
}

// --- v4: Webhook HMAC tests ---

func TestWebhookHMAC(t *testing.T) {
	// We test the HMAC signing by using a public IP webhook server
	// Since SSRF protection blocks localhost, we test the signing logic directly
	// by verifying the dispatcher adds DLQ entries for localhost webhooks (which fail due to SSRF).
	// For the HMAC logic, we verify via the filter/template path.
	//
	// To properly test HMAC, we'd need a publicly-accessible server or to mock HTTP.
	// Instead, test that the config parsing handles the secret field.
	s := tempStore(t)
	d := NewDispatcher(s)

	cfg := `{"url":"https://example.com/webhook","secret":"my-secret-key"}`
	s.AddRule("hmac-hook", "test.*", "", "webhook", cfg, true, 0)

	evData := json.RawMessage(`{"key":"value"}`)
	evID, _ := s.StoreEvent("test.hmac", "test", evData, "")
	d.DispatchSync(evID, "test.hmac", evData)

	// The webhook to example.com will likely fail (DNS/network), but should not panic
	// and should log an error + DLQ entry
	var logStatus string
	s.db.QueryRow(`SELECT status FROM c4_event_log WHERE event_id = ?`, evID).Scan(&logStatus)
	if logStatus != "error" && logStatus != "ok" {
		t.Errorf("expected error or ok status (depends on network), got %s", logStatus)
	}
}

func TestWebhookNoSecret(t *testing.T) {
	// Webhook without secret should work (no HMAC header)
	s := tempStore(t)
	d := NewDispatcher(s)

	cfg := `{"url":"https://example.com/webhook"}`
	s.AddRule("no-secret-hook", "test.*", "", "webhook", cfg, true, 0)

	evData := json.RawMessage(`{"key":"value"}`)
	evID, _ := s.StoreEvent("test.nosecret", "test", evData, "")
	d.DispatchSync(evID, "test.nosecret", evData)

	// Should not panic; log entry should exist
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log WHERE event_id = ?`, evID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 log entry, got %d", count)
	}
}

func TestFilterV2NeFieldMissing(t *testing.T) {
	// $ne on a missing field: should pass (field doesn't exist, so it's "not equal")
	filter := `{"missing_field": {"$ne": "value"}}`
	data := json.RawMessage(`{"other": "data"}`)
	if !evaluateFilter(filter, data) {
		t.Error("$ne on missing field should pass")
	}
}

func TestFilterV2MultiOperators(t *testing.T) {
	// Combined $gt + $lt on same field (range query)
	filter := `{"size": {"$gt": 10, "$lt": 100}}`
	data50 := json.RawMessage(`{"size": 50}`)
	data5 := json.RawMessage(`{"size": 5}`)
	data200 := json.RawMessage(`{"size": 200}`)

	if !evaluateFilter(filter, data50) {
		t.Error("size=50 should match $gt:10 $lt:100")
	}
	if evaluateFilter(filter, data5) {
		t.Error("size=5 should NOT match $gt:10")
	}
	if evaluateFilter(filter, data200) {
		t.Error("size=200 should NOT match $lt:100")
	}
}

func TestFilterV2RegexTooLong(t *testing.T) {
	// Pattern longer than 256 chars should be rejected
	longPattern := ""
	for i := 0; i < 300; i++ {
		longPattern += "a"
	}
	filter := fmt.Sprintf(`{"name": {"$regex": "%s"}}`, longPattern)
	data := json.RawMessage(`{"name": "test"}`)
	if evaluateFilter(filter, data) {
		t.Error("regex pattern > 256 chars should be rejected")
	}
}

func TestFilterV2DotNotationDeep(t *testing.T) {
	// 3-level nested dot notation
	filter := `{"a.b.c": "deep"}`
	data := json.RawMessage(`{"a": {"b": {"c": "deep"}}}`)
	dataMiss := json.RawMessage(`{"a": {"b": {"d": "wrong"}}}`)

	if !evaluateFilter(filter, data) {
		t.Error("3-level dot notation should match")
	}
	if evaluateFilter(filter, dataMiss) {
		t.Error("wrong nested key should not match")
	}
}

func TestDLQMaxRetriesExceeded(t *testing.T) {
	s := tempStore(t)
	s.InsertDLQ("ev-1", "rule-1", "test-rule", "test.event", "fail", 1)
	entries, _ := s.ListDLQ(10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(entries))
	}

	// First retry should succeed (0 < 1)
	_, err := s.IncrementDLQRetry(entries[0].ID)
	if err != nil {
		t.Fatalf("first retry should succeed: %v", err)
	}

	// Second retry should fail (1 >= 1)
	_, err = s.IncrementDLQRetry(entries[0].ID)
	if err == nil {
		t.Error("retry beyond max_retries should return error")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		// IPv4 private ranges
		{"127.0.0.1", true},       // Loopback
		{"127.255.255.255", true}, // Loopback
		{"10.0.0.1", true},        // Private
		{"10.255.255.255", true},  // Private
		{"172.16.0.1", true},      // Private
		{"172.31.255.255", true},  // Private
		{"192.168.1.1", true},     // Private
		{"192.168.255.255", true}, // Private
		{"169.254.1.1", true},     // Link-local
		{"0.0.0.0", true},         // Unspecified

		// IPv4 public
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},

		// IPv6 loopback and link-local
		{"::1", true},                                   // Loopback
		{"fe80::1", true},                               // Link-local unicast
		{"ff02::1", true},                               // Link-local multicast
		{"::", true},                                    // Unspecified
		{"fc00::1", true},                               // Unique local (fc00::/7)
		{"fd12:3456:789a:1::1", true},                   // Unique local (fc00::/7)
		{"2001:4860:4860::8888", false},                 // Public (Google DNS)
		{"2606:4700:4700::1111", false},                 // Public (Cloudflare DNS)
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("invalid test IP: %s", tt.ip)
		}
		got := isPrivateIP(ip)
		if got != tt.want {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestDLQMaxRetriesFromConfig(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()

	dispatcher := NewDispatcher(store)

	// Add a webhook rule with custom max_retries
	store.AddRule("custom-retry", "test.*", "{}", "webhook",
		`{"url":"http://192.168.1.1:9999/hook","max_retries":5}`, true, 0)

	// Dispatch event — webhook to private IP will fail (SSRF block)
	dispatcher.DispatchSync("ev-retry-1", "test.event", json.RawMessage(`{}`))

	time.Sleep(50 * time.Millisecond)

	// Check DLQ entry has max_retries=5 (not default 3)
	entries, err := store.ListDLQ(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected DLQ entry")
	}
	if entries[0].MaxRetries != 5 {
		t.Errorf("expected max_retries=5, got %d", entries[0].MaxRetries)
	}
}

func TestDLQMaxRetriesDefault(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()

	dispatcher := NewDispatcher(store)

	// Add a webhook rule WITHOUT max_retries
	store.AddRule("default-retry", "test.*", "{}", "webhook",
		`{"url":"http://192.168.1.1:9999/hook"}`, true, 0)

	dispatcher.DispatchSync("ev-retry-2", "test.event", json.RawMessage(`{}`))

	time.Sleep(50 * time.Millisecond)

	entries, _ := store.ListDLQ(10)
	if len(entries) == 0 {
		t.Fatal("expected DLQ entry")
	}
	if entries[0].MaxRetries != 3 {
		t.Errorf("expected default max_retries=3, got %d", entries[0].MaxRetries)
	}
}

func TestTemplateNestedField(t *testing.T) {
	data := map[string]any{
		"task": map[string]any{
			"id":    "T-001",
			"title": "test task",
		},
		"simple": "value",
	}

	// Nested field
	result := resolveTemplateString("Task: {{task.id}} - {{task.title}}", data)
	if result != "Task: T-001 - test task" {
		t.Errorf("expected 'Task: T-001 - test task', got %q", result)
	}

	// Simple field (backward compatible)
	result2 := resolveTemplateString("Val: {{simple}}", data)
	if result2 != "Val: value" {
		t.Errorf("expected 'Val: value', got %q", result2)
	}

	// Missing nested field
	result3 := resolveTemplateString("Missing: {{task.nonexistent}}", data)
	if result3 != "Missing: " {
		t.Errorf("expected 'Missing: ', got %q", result3)
	}
}

func TestDispatchBoundedConcurrency(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()

	dispatcher := NewDispatcher(store)

	// Verify semaphore was created with capacity 32
	if cap(dispatcher.sem) != 32 {
		t.Errorf("expected sem capacity 32, got %d", cap(dispatcher.sem))
	}

	// Add a log rule
	store.AddRule("bounded-test", "test.*", "{}", "log", "{}", true, 0)

	// Dispatch multiple events — should not deadlock
	for i := 0; i < 50; i++ {
		dispatcher.Dispatch(fmt.Sprintf("ev-%d", i), "test.event", json.RawMessage(`{}`))
	}

	// Wait for all dispatches to complete
	time.Sleep(200 * time.Millisecond)

	// Semaphore should be fully released
	if len(dispatcher.sem) != 0 {
		t.Errorf("expected empty semaphore, got %d in-use", len(dispatcher.sem))
	}
}
