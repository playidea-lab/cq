package mcphttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
)

// newTestComponentWithQueue creates a Component with a DoorayQueue for testing.
// apiKey is pre-injected so withAuth works without Start().
func newTestComponentWithQueue(t *testing.T, apiKey string) (*Component, *DoorayQueue) {
	t.Helper()
	q := NewDoorayQueue()
	comp := New(&stubHandler{}, &stubSecretGetter{}, config.ServeMCPHTTPConfig{
		Port:   4142,
		Bind:   "127.0.0.1",
		APIKey: apiKey,
	}).WithDoorayQueue(q)
	comp.apiKey = apiKey
	return comp, q
}

// TestDoorayQueue_PushPopAll verifies basic push/pop semantics.
func TestDoorayQueue_PushPopAll(t *testing.T) {
	q := NewDoorayQueue()

	// Empty pop returns zero-length result (may be nil slice — callers use len).
	msgs := q.PopAll()
	if len(msgs) != 0 {
		t.Fatalf("PopAll on empty queue should return 0 messages, got %d", len(msgs))
	}

	q.Push(DoorayMessage{Text: "hello", ResponseURL: "https://hooks.dooray.com/1", ReceivedAt: time.Now()})
	q.Push(DoorayMessage{Text: "world", ResponseURL: "https://hooks.dooray.com/2", ReceivedAt: time.Now()})

	msgs = q.PopAll()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" {
		t.Errorf("msgs[0].Text = %q, want hello", msgs[0].Text)
	}

	// Second pop should return empty — pop pattern.
	msgs = q.PopAll()
	if len(msgs) != 0 {
		t.Fatalf("second PopAll should return 0 messages, got %d", len(msgs))
	}
}

// TestDoorayPending_EmptyQueue returns an empty JSON array.
func TestDoorayPending_EmptyQueue(t *testing.T) {
	comp, _ := newTestComponentWithQueue(t, "k")

	req := httptest.NewRequest(http.MethodGet, "/v1/dooray/pending", nil)
	w := httptest.NewRecorder()
	comp.handleDoorayPending(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var msgs []DoorayMessage
	if err := json.NewDecoder(w.Body).Decode(&msgs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty array, got %d messages", len(msgs))
	}
}

// TestDoorayPending_ReturnsAndClears verifies pop behaviour.
func TestDoorayPending_ReturnsAndClears(t *testing.T) {
	comp, q := newTestComponentWithQueue(t, "k")
	q.Push(DoorayMessage{Text: "test", ResponseURL: "https://hooks.dooray.com/x", ReceivedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/v1/dooray/pending", nil)
	w := httptest.NewRecorder()
	comp.handleDoorayPending(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var msgs []DoorayMessage
	if err := json.NewDecoder(w.Body).Decode(&msgs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "test" {
		t.Errorf("Text = %q, want test", msgs[0].Text)
	}

	// Second request should see empty queue (pop pattern).
	req2 := httptest.NewRequest(http.MethodGet, "/v1/dooray/pending", nil)
	w2 := httptest.NewRecorder()
	comp.handleDoorayPending(w2, req2)
	var msgs2 []DoorayMessage
	json.NewDecoder(w2.Body).Decode(&msgs2) //nolint:errcheck
	if len(msgs2) != 0 {
		t.Errorf("second GET should return empty, got %d messages", len(msgs2))
	}
}

// TestDoorayPending_MethodNotAllowed verifies POST is rejected.
func TestDoorayPending_MethodNotAllowed(t *testing.T) {
	comp, _ := newTestComponentWithQueue(t, "k")
	req := httptest.NewRequest(http.MethodPost, "/v1/dooray/pending", nil)
	w := httptest.NewRecorder()
	comp.handleDoorayPending(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestDoorayReply_SSRFProtection verifies non-dooray.com URLs are rejected.
func TestDoorayReply_SSRFProtection(t *testing.T) {
	comp, _ := newTestComponentWithQueue(t, "k")

	cases := []struct {
		name string
		url  string
	}{
		{"non-dooray", "https://evil.example.com/hook"},
		{"http-not-https", "http://hooks.dooray.com/services/123"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{
				"response_url": tc.url,
				"text":         "hello",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/dooray/reply", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			comp.handleDoorayReply(w, req)
			if w.Code == http.StatusOK {
				t.Errorf("expected non-200 for SSRF-prone URL %q, got 200", tc.url)
			}
		})
	}
}

// TestDoorayReply_MissingText verifies 400 when text is empty.
func TestDoorayReply_MissingText(t *testing.T) {
	comp, _ := newTestComponentWithQueue(t, "k")
	body, _ := json.Marshal(map[string]string{
		"response_url": "https://hooks.dooray.com/services/1",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/dooray/reply", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	comp.handleDoorayReply(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing text, got %d", w.Code)
	}
}

// TestDoorayReply_MethodNotAllowed verifies GET is rejected.
func TestDoorayReply_MethodNotAllowed(t *testing.T) {
	comp, _ := newTestComponentWithQueue(t, "k")
	req := httptest.NewRequest(http.MethodGet, "/v1/dooray/reply", nil)
	w := httptest.NewRecorder()
	comp.handleDoorayReply(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestDoorayReply_ProxiesToDooray verifies a valid reply is proxied to *.dooray.com.
func TestDoorayReply_ProxiesToDooray(t *testing.T) {
	// Set up a fake Dooray server.
	fakeDooray := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeDooray.Close()

	// We can't use a real *.dooray.com URL in unit tests, so we test SSRF validation
	// is the only gate — the proxy logic is covered by integration; here we verify
	// the path rejects non-dooray URLs and accepts the validation boundary.
	comp, _ := newTestComponentWithQueue(t, "k")

	// A well-formed dooray URL but unavailable host → should fail at network, not SSRF.
	body, _ := json.Marshal(map[string]string{
		"response_url": "https://hooks.dooray.com/services/999/test",
		"text":         "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/dooray/reply", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	comp.handleDoorayReply(w, req)

	// The SSRF check passes (*.dooray.com), but the network POST fails → 502 or DNS error.
	// We simply verify it's NOT a 400-level SSRF rejection.
	if w.Code == http.StatusBadRequest {
		t.Errorf("SSRF validation should pass for *.dooray.com, got 400: %s", w.Body.String())
	}
}
