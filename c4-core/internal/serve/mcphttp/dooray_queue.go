package mcphttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
)

// DoorayMessage is a single pending Dooray slash-command message.
type DoorayMessage struct {
	// Text is the slash-command body sent by the user.
	Text string `json:"text"`
	// ResponseURL is the Dooray callback URL for replying.
	ResponseURL string `json:"response_url"`
	// ReceivedAt is set when the message was enqueued.
	ReceivedAt time.Time `json:"received_at"`
	// Raw holds the full original request body for callers that need it.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// DoorayQueue is a thread-safe in-memory pending message queue for Dooray
// slash commands. Messages are popped (removed) on the first GET, so each
// message is delivered exactly once. The queue is intentionally simple:
// no persistence, no capacity limit, restart clears all messages.
type DoorayQueue struct {
	mu   sync.Mutex
	msgs []DoorayMessage
}

// NewDoorayQueue creates an empty DoorayQueue.
func NewDoorayQueue() *DoorayQueue {
	return &DoorayQueue{}
}

// Push appends a message to the tail of the queue.
func (q *DoorayQueue) Push(msg DoorayMessage) {
	q.mu.Lock()
	q.msgs = append(q.msgs, msg)
	q.mu.Unlock()
}

// PopAll removes and returns all pending messages (pop pattern).
func (q *DoorayQueue) PopAll() []DoorayMessage {
	q.mu.Lock()
	msgs := q.msgs
	q.msgs = nil
	q.mu.Unlock()
	return msgs
}

// handleDoorayPending handles GET /v1/dooray/pending.
// It pops all pending messages and returns them as JSON.
// No auth: the endpoint is protected by the server's bind address (localhost by default).
// If auth is required in the future, wrap with withAuth.
func (c *Component) handleDoorayPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	msgs := c.doorayQueue.PopAll()
	if msgs == nil {
		msgs = []DoorayMessage{} // return [] not null
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs) //nolint:errcheck
}

// handleDoorayReply handles POST /v1/dooray/reply.
// It proxies the request to the given response_url after SSRF validation.
//
// Expected JSON body:
//
//	{
//	  "response_url": "https://hooks.dooray.com/…",
//	  "text": "reply text",
//	  "response_type": "ephemeral" | "inChannel"  // optional
//	}
func (c *Component) handleDoorayReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes))
	if err != nil {
		http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
		return
	}

	var args struct {
		ResponseURL  string `json:"response_url"`
		Text         string `json:"text"`
		ResponseType string `json:"response_type"`
	}
	if err := json.Unmarshal(body, &args); err != nil {
		writeJSONError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if args.ResponseURL == "" {
		writeJSONError(w, "response_url is required", http.StatusBadRequest)
		return
	}
	if args.Text == "" {
		writeJSONError(w, "text is required", http.StatusBadRequest)
		return
	}

	// SSRF protection: only *.dooray.com HTTPS URLs are allowed.
	if err := eventbus.ValidateDoorayResponseURL(args.ResponseURL); err != nil {
		writeJSONError(w, "invalid response_url: "+err.Error(), http.StatusBadRequest)
		return
	}

	responseType := args.ResponseType
	if responseType == "" {
		responseType = "ephemeral"
	}

	payload := map[string]any{
		"text":         args.Text,
		"responseType": responseType,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		writeJSONError(w, "marshal error", http.StatusInternalServerError)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, args.ResponseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		writeJSONError(w, "create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		writeJSONError(w, "POST failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		writeJSONError(w, "Dooray returned HTTP "+http.StatusText(resp.StatusCode), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
}

// writeJSONError writes a simple JSON error response.
func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}
