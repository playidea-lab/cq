//go:build c3_eventbus

package eventbushandler

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
)

// StartEventSinkServer starts an HTTP server that accepts incoming events and publishes them
// to the EventBus. Returns nil server if port is 0 (disabled).
// Port and token are provided by the caller (from config.EventSink).
func StartEventSinkServer(port int, token string, pub eventbus.Publisher) (*http.Server, error) {
	if port == 0 {
		return nil, nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Auth check (skip if token is empty)
		if token != "" {
			auth := r.Header.Get("Authorization")
			expected := "Bearer " + token
			if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "unauthorized"})
				return
			}
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
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

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "cq: eventsink server error: %v\n", err)
		}
	}()

	return srv, nil
}

