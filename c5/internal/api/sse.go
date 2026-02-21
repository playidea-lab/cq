package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleSSEStream implements GET /v1/events/stream (Server-Sent Events).
//
// Subscribers receive events whenever a new job is queued or its state
// changes (driven by the existing jobNotify broadcast channel).
// Each event is formatted as:
//
//	data: <json>\n\n
//
// The channel buffer is 16.  When the buffer is full the event is dropped
// (non-blocking) to avoid back-pressure stalls in the server.
func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Register subscriber channel.
	ch := make(chan string, 16)
	s.sseSubs.Store(ch, struct{}{})
	defer func() {
		s.sseSubs.Delete(ch)
		// Drain to unblock any concurrent senders.
		for len(ch) > 0 {
			<-ch
		}
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// broadcastSSEEvent sends a JSON-encoded event to all registered SSE subscribers.
// Full when a subscriber's channel is full, the event is dropped for that subscriber.
func (s *Server) broadcastSSEEvent(eventType string, data any) {
	payload, err := json.Marshal(map[string]any{
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	})
	if err != nil {
		return
	}
	msg := string(payload)

	s.sseSubs.Range(func(key, _ any) bool {
		ch, ok := key.(chan string)
		if !ok {
			return true
		}
		// Non-blocking send: drop if buffer is full.
		select {
		case ch <- msg:
		default:
		}
		return true
	})
}
