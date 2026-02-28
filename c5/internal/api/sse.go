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
//
// Project isolation: the subscriber's project ID (from auth context) is
// stored as the sync.Map value. Master key subscribers store "" and receive
// all events regardless of project.
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

	// Register subscriber channel with caller's project ID.
	// Master key sets pid="" which receives all events.
	ch := make(chan string, 16)
	pid := projectIDFromContext(r)
	s.sseSubs.Store(ch, pid)
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
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

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
		case <-keepalive.C:
			// SSE comment to keep the connection alive through proxies/load balancers.
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// broadcastSSEEvent sends a JSON-encoded event to SSE subscribers whose
// project ID matches projectID, plus all master-key subscribers (value == "").
//
// Pass projectID="" to deliver to all subscribers (worker-level broadcasts
// such as "job.available").
//
// When a subscriber's channel is full the event is dropped for that subscriber.
func (s *Server) broadcastSSEEvent(projectID string, eventType string, data any) {
	payload, err := json.Marshal(map[string]any{
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	})
	if err != nil {
		return
	}
	msg := string(payload)

	s.sseSubs.Range(func(k, v any) bool {
		subPID, ok := v.(string)
		if !ok {
			return true
		}
		// Deliver if: subscriber is master (subPID=="") OR same project OR
		// broadcast (projectID=="") which delivers to everyone.
		if subPID == "" || projectID == "" || subPID == projectID {
			ch, ok := k.(chan string)
			if !ok {
				return true
			}
			// Non-blocking send: drop if buffer is full.
			select {
			case ch <- msg:
			default:
			}
		}
		return true
	})
}
