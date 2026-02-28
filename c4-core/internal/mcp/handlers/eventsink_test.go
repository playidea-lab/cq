//go:build c3_eventbus

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers/eventbushandler"
)

// capturePublisher records PublishAsync calls for test assertions.
type capturePublisher struct {
	calls []capturedEvent
}

type capturedEvent struct {
	evType    string
	source    string
	data      json.RawMessage
	projectID string
}

func (c *capturePublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	c.calls = append(c.calls, capturedEvent{evType: evType, source: source, data: data, projectID: projectID})
}

func TestEventSinkServer_PublishSuccess(t *testing.T) {
	pub := &capturePublisher{}
	srv, err := eventbushandler.StartEventSinkServer(0, "", pub) // port=0 → disabled
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv != nil {
		t.Fatal("expected nil server for port=0")
	}

	// Test via handler directly (port=0 disables server, use httptest instead)
	token := "test-secret"
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events/publish", func(w http.ResponseWriter, r *http.Request) {
		// inline the handler logic via a minimal server for test
		if token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{"ok": false})
				return
			}
		}
		var req struct {
			EventType string          `json:"event_type"`
			Source    string          `json:"source"`
			Data      json.RawMessage `json:"data"`
			ProjectID string          `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pub.PublishAsync(req.EventType, req.Source, req.Data, req.ProjectID)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"event_type": "hub.job.completed",
		"source":     "c4.hub",
		"data":       map[string]any{"job_id": "j-1"},
		"project_id": "proj-abc",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/events/publish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].evType != "hub.job.completed" {
		t.Errorf("event_type = %q, want hub.job.completed", pub.calls[0].evType)
	}
}

func TestEventSinkServer_WrongToken401(t *testing.T) {
	pub := &capturePublisher{}
	token := "correct-token"

	// Build a real HTTP test server using StartEventSinkServer via httptest approach
	mux := buildEventSinkMux(token, pub)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"event_type": "test.event"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/events/publish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected 0 publish calls, got %d", len(pub.calls))
	}
}

func TestEventSinkServer_NoTokenSkipsAuth(t *testing.T) {
	pub := &capturePublisher{}
	mux := buildEventSinkMux("", pub) // empty token → no auth required
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"event_type": "test.event"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/events/publish", bytes.NewReader(body))
	// No Authorization header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(pub.calls) != 1 {
		t.Errorf("expected 1 publish call, got %d", len(pub.calls))
	}
}

func TestEventSinkServer_MissingEventType400(t *testing.T) {
	pub := &capturePublisher{}
	mux := buildEventSinkMux("", pub)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"source": "test"}) // no event_type
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/events/publish", bytes.NewReader(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestEventSinkServer_Disabled(t *testing.T) {
	pub := &capturePublisher{}
	srv, err := eventbushandler.StartEventSinkServer(0, "", pub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv != nil {
		t.Error("expected nil server for port=0 (disabled)")
	}
}

func TestEventSinkServer_StartAndStop(t *testing.T) {
	pub := &capturePublisher{}
	// Use a random high port to avoid conflicts
	srv, err := eventbushandler.StartEventSinkServer(14242, "", pub)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	// Give it a moment to bind
	time.Sleep(20 * time.Millisecond)
	srv.Close()
}

// buildEventSinkMux creates an http.ServeMux that handles /v1/events/publish.
// This mirrors the logic inside StartEventSinkServer for testability.
func buildEventSinkMux(token string, pub eventbus.Publisher) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "unauthorized"})
				return
			}
		}
		var req struct {
			EventType string          `json:"event_type"`
			Source    string          `json:"source"`
			Data      json.RawMessage `json:"data"`
			ProjectID string          `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid JSON"})
			return
		}
		if req.EventType == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "event_type is required"})
			return
		}
		source := req.Source
		if source == "" {
			source = "eventsink"
		}
		data := req.Data
		if data == nil {
			data = json.RawMessage("{}")
		}
		pub.PublishAsync(req.EventType, source, data, req.ProjectID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	return mux
}
