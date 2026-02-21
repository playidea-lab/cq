package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// RealtimeEvent represents a parsed Supabase Realtime postgres_changes event.
type RealtimeEvent struct {
	Table      string          `json:"table"`
	ChangeType string          `json:"change_type"` // INSERT, UPDATE, DELETE
	Record     json.RawMessage `json:"record"`
	OldRecord  json.RawMessage `json:"old_record,omitempty"`
}

// MessageCallback is called when a Realtime event is received.
type MessageCallback func(event RealtimeEvent)

// RealtimeClient connects to Supabase Realtime (Phoenix Channels protocol)
// and delivers postgres_changes events via a callback.
type RealtimeClient struct {
	supabaseURL string
	apiKey      string
	authToken   string

	mu       sync.Mutex
	conn     net.Conn
	tables   []string
	callback MessageCallback
	cancel   context.CancelFunc
	running  bool
}

// NewRealtimeClient creates a new client. Call Connect to establish the connection.
func NewRealtimeClient(supabaseURL, apiKey, authToken string) *RealtimeClient {
	return &RealtimeClient{
		supabaseURL: supabaseURL,
		apiKey:      apiKey,
		authToken:   authToken,
	}
}

// Subscribe adds a table to the subscription list. Must be called before Connect.
func (rc *RealtimeClient) Subscribe(table string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.tables = append(rc.tables, table)
}

// OnMessage sets the callback for received events. Must be called before Connect.
func (rc *RealtimeClient) OnMessage(cb MessageCallback) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.callback = cb
}

// Connect starts the WebSocket connection with auto-reconnect.
// It spawns a background goroutine and returns immediately.
// The connection is closed when ctx is cancelled.
func (rc *RealtimeClient) Connect(ctx context.Context) error {
	rc.mu.Lock()
	if rc.running {
		rc.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	if len(rc.tables) == 0 {
		rc.mu.Unlock()
		return fmt.Errorf("no tables subscribed")
	}
	if rc.callback == nil {
		rc.mu.Unlock()
		return fmt.Errorf("no message callback set")
	}
	rc.running = true
	childCtx, cancel := context.WithCancel(ctx)
	rc.cancel = cancel
	rc.mu.Unlock()

	go rc.connectionLoop(childCtx)
	return nil
}

// Close shuts down the connection.
func (rc *RealtimeClient) Close() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.cancel != nil {
		rc.cancel()
	}
	rc.running = false
}

// buildWSURL converts the Supabase project URL to a Realtime WebSocket URL.
func (rc *RealtimeClient) buildWSURL() string {
	base := rc.supabaseURL
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf("%s/realtime/v1/websocket?apikey=%s&vsn=1.0.0", base, url.QueryEscape(rc.apiKey))
}

// phxMessage is the Phoenix Channels protocol message format.
type phxMessage struct {
	Topic   string      `json:"topic"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
	Ref     *string     `json:"ref"`
}

// makeJoinPayload creates the phx_join payload for a table subscription.
func makeJoinPayload(table, authToken string) map[string]interface{} {
	payload := map[string]interface{}{
		"config": map[string]interface{}{
			"broadcast": map[string]interface{}{"self": false},
			"presence":  map[string]interface{}{"key": ""},
			"postgres_changes": []map[string]interface{}{
				{
					"event":  "*",
					"schema": "public",
					"table":  table,
				},
			},
		},
	}
	if authToken != "" {
		payload["access_token"] = authToken
	}
	return payload
}

// connectionLoop runs the reconnection loop with exponential backoff.
func (rc *RealtimeClient) connectionLoop(ctx context.Context) {
	const (
		maxBackoff         = 60 * time.Second
		maxReconnectFails  = 50
	)
	backoff := 1 * time.Second
	consecutiveFailures := 0

	for {
		if ctx.Err() != nil {
			return
		}
		if consecutiveFailures >= maxReconnectFails {
			fmt.Fprintf(os.Stderr, "cq: [realtime] max reconnect attempts (%d) reached\n", maxReconnectFails)
			return
		}

		err := rc.connectOnce(ctx)
		if ctx.Err() != nil {
			return // intentional shutdown
		}
		if err != nil {
			consecutiveFailures++
			fmt.Fprintf(os.Stderr, "cq: [realtime] connection error (%d/%d): %v\n",
				consecutiveFailures, maxReconnectFails, err)
		} else {
			// Clean disconnect — reset counters
			consecutiveFailures = 0
			backoff = 1 * time.Second
			fmt.Fprintln(os.Stderr, "cq: [realtime] connection closed cleanly, reconnecting")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectOnce establishes a single WebSocket connection, joins channels,
// sends heartbeats, and reads messages until disconnection.
func (rc *RealtimeClient) connectOnce(ctx context.Context) error {
	wsURL := rc.buildWSURL()

	dialer := ws.Dialer{
		Timeout: 10 * time.Second,
	}
	conn, _, _, err := dialer.Dial(ctx, wsURL)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	rc.mu.Lock()
	rc.conn = conn
	tables := make([]string, len(rc.tables))
	copy(tables, rc.tables)
	callback := rc.callback
	rc.mu.Unlock()

	defer func() {
		conn.Close()
		rc.mu.Lock()
		rc.conn = nil
		rc.mu.Unlock()
	}()

	// Join channels for each subscribed table
	for i, table := range tables {
		ref := fmt.Sprintf("%d", i+1)
		msg := phxMessage{
			Topic:   fmt.Sprintf("realtime:public:%s", table),
			Event:   "phx_join",
			Payload: makeJoinPayload(table, rc.authToken),
			Ref:     &ref,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal join for %s: %w", table, err)
		}
		if err := wsutil.WriteClientText(conn, data); err != nil {
			return fmt.Errorf("send join for %s: %w", table, err)
		}
	}

	// Heartbeat + read loop
	heartbeatTicker := time.NewTicker(25 * time.Second)
	defer heartbeatTicker.Stop()

	readCh := make(chan readResult, 1)

	// Spawn a read goroutine
	go func() {
		for {
			if nc, ok := conn.(net.Conn); ok {
				nc.SetReadDeadline(time.Now().Add(35 * time.Second))
			}
			data, op, err := wsutil.ReadServerData(conn)
			select {
			case readCh <- readResult{data: data, op: op, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-heartbeatTicker.C:
			ref := "hb"
			hb := phxMessage{
				Topic:   "phoenix",
				Event:   "heartbeat",
				Payload: map[string]interface{}{},
				Ref:     &ref,
			}
			data, _ := json.Marshal(hb)
			if err := wsutil.WriteClientText(conn, data); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}

		case result := <-readCh:
			if result.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("read: %w", result.err)
			}
			if result.op == ws.OpClose {
				return fmt.Errorf("server closed connection")
			}
			if result.op == ws.OpText {
				rc.handleMessage(result.data, callback)
			}
		}
	}
}

type readResult struct {
	data []byte
	op   ws.OpCode
	err  error
}

// handleMessage parses a Phoenix Channel message and dispatches postgres_changes events.
func (rc *RealtimeClient) handleMessage(data []byte, callback MessageCallback) {
	var msg struct {
		Topic   string          `json:"topic"`
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	if msg.Event != "postgres_changes" {
		return
	}

	// Extract table from topic: "realtime:public:c1_messages" -> "c1_messages"
	table := "unknown"
	if parts := strings.SplitN(msg.Topic, ":", 3); len(parts) == 3 {
		table = parts[2]
	}

	// Parse payload.data for the change details
	var payload struct {
		Data struct {
			Type      string          `json:"type"`
			Record    json.RawMessage `json:"record"`
			OldRecord json.RawMessage `json:"old_record"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return
	}

	event := RealtimeEvent{
		Table:      table,
		ChangeType: payload.Data.Type,
		Record:     payload.Data.Record,
		OldRecord:  payload.Data.OldRecord,
	}
	callback(event)
}
