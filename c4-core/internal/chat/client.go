// Package chat provides a Supabase Realtime WebSocket client for the c1_messages table,
// plus a thin PostgREST client for sending messages.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// Message represents a row from the c1_messages table.
type Message struct {
	ID         string `json:"id"`
	ChannelID  string `json:"channel_id"`
	SenderName string `json:"sender_name"`
	SenderType string `json:"sender_type"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// MessageHandler is called when a new c1_messages INSERT is received.
type MessageHandler func(msg Message)

// Client is a Supabase Realtime WebSocket client scoped to c1_messages.
// It subscribes to real-time INSERT events and sends messages via PostgREST.
type Client struct {
	supabaseURL string
	anonKey     string
	accessToken string

	mu       sync.Mutex
	conn     net.Conn
	filter   string // PostgREST filter, e.g. "channel_id=eq.uuid"
	handler  MessageHandler
	cancel   context.CancelFunc
	running  bool

	httpClient *http.Client
}

// New creates a Client. supabaseURL is the Supabase project URL (e.g. https://xxx.supabase.co).
// accessToken is the authenticated user's JWT (may be empty for anon access).
func New(supabaseURL, anonKey, accessToken string) *Client {
	return &Client{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		anonKey:     anonKey,
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Subscribe registers a handler and starts receiving c1_messages INSERT events.
// filter is a PostgREST column filter, e.g. "channel_id=eq.<uuid>".
// Pass an empty filter to receive all inserts.
// Connect must be called after Subscribe.
func (c *Client) Subscribe(filter string, handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filter = filter
	c.handler = handler
}

// Connect starts the WebSocket connection to Supabase Realtime with auto-reconnect.
// It spawns a background goroutine and returns immediately.
// The connection is closed when ctx is cancelled.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("chat: already connected")
	}
	if c.handler == nil {
		c.mu.Unlock()
		return fmt.Errorf("chat: call Subscribe before Connect")
	}
	c.running = true
	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	go c.connectionLoop(childCtx)
	return nil
}

// Close shuts down the Realtime connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	c.running = false
}

// SendMessage inserts a message into c1_messages via PostgREST.
// channelID is the UUID of the target channel.
// senderType is typically "user", "agent", or "system".
func (c *Client) SendMessage(ctx context.Context, channelID, content, senderType string) error {
	if channelID == "" {
		return fmt.Errorf("chat: channelID is required")
	}
	row := map[string]string{
		"channel_id":  channelID,
		"content":     content,
		"sender_type": senderType,
	}
	body, err := json.Marshal([]map[string]string{row})
	if err != nil {
		return fmt.Errorf("chat: send: marshal: %w", err)
	}

	endpoint := c.supabaseURL + "/rest/v1/c1_messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("chat: send: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chat: send: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("chat: send: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// setHeaders adds authentication headers to r.
func (c *Client) setHeaders(r *http.Request) {
	r.Header.Set("apikey", c.anonKey)
	token := c.anonKey
	if c.accessToken != "" {
		token = c.accessToken
	}
	r.Header.Set("Authorization", "Bearer "+token)
}

// --- Realtime internals ---

// buildWSURL converts the Supabase project URL to a Realtime WebSocket endpoint.
func (c *Client) buildWSURL() string {
	base := c.supabaseURL
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	return fmt.Sprintf("%s/realtime/v1/websocket?apikey=%s&vsn=1.0.0", base, url.QueryEscape(c.anonKey))
}

// connectionLoop runs reconnection with exponential backoff.
func (c *Client) connectionLoop(ctx context.Context) {
	const maxBackoff = 60 * time.Second
	backoff := 1 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		err := c.connectOnce(ctx)
		if ctx.Err() != nil {
			return // intentional shutdown
		}
		if err != nil {
			fmt.Printf("chat: [realtime] error: %v — retry in %s\n", err, backoff)
			backoff = min(backoff*2, maxBackoff)
		} else {
			backoff = 1 * time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// phxMessage is the Phoenix Channels wire format.
type phxMessage struct {
	Topic   string      `json:"topic"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
	Ref     *string     `json:"ref"`
}

// connectOnce dials, subscribes, heartbeats, and reads until disconnected.
func (c *Client) connectOnce(ctx context.Context) error {
	dialer := ws.Dialer{Timeout: 10 * time.Second}
	conn, _, _, err := dialer.Dial(ctx, c.buildWSURL())
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	filter := c.filter
	handler := c.handler
	accessToken := c.accessToken
	c.mu.Unlock()

	defer func() {
		conn.Close()
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

	// Send phx_join for c1_messages
	if err := c.sendJoin(conn, filter, accessToken); err != nil {
		return err
	}

	// Heartbeat + read loop
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	type readResult struct {
		data []byte
		op   ws.OpCode
		err  error
	}
	readCh := make(chan readResult, 1)
	readDone := make(chan struct{})
	defer close(readDone)

	go func() {
		for {
			if nc, ok := conn.(net.Conn); ok {
				_ = nc.SetReadDeadline(time.Now().Add(35 * time.Second))
			}
			data, op, err := wsutil.ReadServerData(conn)
			select {
			case readCh <- readResult{data: data, op: op, err: err}:
			case <-readDone:
				return
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
		case <-ticker.C:
			if err := c.sendHeartbeat(conn); err != nil {
				return err
			}
		case r := <-readCh:
			if r.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("read: %w", r.err)
			}
			if r.op == ws.OpClose {
				return nil
			}
			if r.op == ws.OpText {
				c.handleMessage(r.data, handler)
			}
		}
	}
}

// sendJoin sends a phx_join for the realtime:public:c1_messages channel.
func (c *Client) sendJoin(conn net.Conn, filter, accessToken string) error {
	ref := "1"
	joinPayload := map[string]interface{}{
		"config": map[string]interface{}{
			"broadcast": map[string]interface{}{"self": false},
			"presence":  map[string]interface{}{"key": ""},
			"postgres_changes": []map[string]interface{}{
				{
					"event":  "INSERT",
					"schema": "public",
					"table":  "c1_messages",
					"filter": filter,
				},
			},
		},
	}
	if accessToken != "" {
		joinPayload["access_token"] = accessToken
	}
	msg := phxMessage{
		Topic:   "realtime:public:c1_messages",
		Event:   "phx_join",
		Payload: joinPayload,
		Ref:     &ref,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("chat: join marshal: %w", err)
	}
	if err := wsutil.WriteClientText(conn, data); err != nil {
		return fmt.Errorf("chat: join send: %w", err)
	}
	return nil
}

// sendHeartbeat sends a Phoenix Channel heartbeat.
func (c *Client) sendHeartbeat(conn net.Conn) error {
	ref := "hb"
	msg := phxMessage{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: map[string]interface{}{},
		Ref:     &ref,
	}
	data, _ := json.Marshal(msg)
	if err := wsutil.WriteClientText(conn, data); err != nil {
		return fmt.Errorf("chat: heartbeat: %w", err)
	}
	return nil
}

// handleMessage parses a Phoenix Channel message and dispatches c1_messages INSERT events.
func (c *Client) handleMessage(data []byte, handler MessageHandler) {
	var raw struct {
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if raw.Event != "postgres_changes" {
		return
	}

	var payload struct {
		Data struct {
			Type   string  `json:"type"`
			Record Message `json:"record"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return
	}
	if payload.Data.Type != "INSERT" {
		return
	}
	handler(payload.Data.Record)
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
