// Package realtime provides Supabase Realtime subscription support.
//
// Uses WebSocket to receive real-time changes to c4_tasks and c4_state
// tables. Includes automatic reconnection with exponential backoff.
package realtime

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ChangeEvent represents a table change received via Supabase Realtime.
type ChangeEvent struct {
	Table    string         `json:"table"`
	Type     string         `json:"type"` // "INSERT", "UPDATE", "DELETE"
	Record   map[string]any `json:"record"`
	OldRecord map[string]any `json:"old_record,omitempty"`
}

// Callback is called when a table change is received.
type Callback func(event ChangeEvent)

// Subscription represents an active realtime subscription.
type Subscription struct {
	ID       string
	Table    string
	callback Callback
	active   atomic.Bool
}

// IsActive returns true if the subscription is still active.
func (s *Subscription) IsActive() bool {
	return s.active.Load()
}

// Transport abstracts the WebSocket connection for testing.
type Transport interface {
	// Connect establishes the WebSocket connection.
	Connect() error

	// Send sends a message (e.g., subscribe/unsubscribe commands).
	Send(msg []byte) error

	// Receive blocks until a message is received or error occurs.
	Receive() ([]byte, error)

	// Close closes the connection.
	Close() error

	// IsOpen returns true if the connection is established.
	IsOpen() bool
}

// Config holds Realtime client configuration.
type Config struct {
	URL            string        // WebSocket URL (wss://xxx.supabase.co/realtime/v1)
	AnonKey        string        // Supabase anon key
	MaxReconnect   int           // max reconnection attempts (0 = unlimited)
	BaseBackoff    time.Duration // initial backoff (default 1s)
	MaxBackoff     time.Duration // max backoff (default 30s)
	HeartbeatInterval time.Duration // keepalive interval (default 30s)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxReconnect:      0,
		BaseBackoff:       1 * time.Second,
		MaxBackoff:        30 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
}

// Client manages Supabase Realtime subscriptions.
//
// Thread-safe: Subscribe/Unsubscribe can be called concurrently.
type Client struct {
	config        *Config
	transport     Transport
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
	nextID        atomic.Int64
	running       atomic.Bool
	stopChan      chan struct{}
	reconnecting  atomic.Bool
}

// NewClient creates a Realtime client with the given transport.
func NewClient(cfg *Config, transport Transport) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{
		config:        cfg,
		transport:     transport,
		subscriptions: make(map[string]*Subscription),
		stopChan:      make(chan struct{}),
	}
}

// Start connects to the Realtime server and begins receiving events.
// Runs the event loop in a goroutine.
func (c *Client) Start() error {
	if err := c.transport.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	c.running.Store(true)

	// Start the event receive loop
	go c.receiveLoop()

	// Start heartbeat
	go c.heartbeatLoop()

	return nil
}

// Stop disconnects and stops all subscriptions.
func (c *Client) Stop() {
	if !c.running.Load() {
		return
	}
	c.running.Store(false)
	close(c.stopChan)

	c.mu.Lock()
	for _, sub := range c.subscriptions {
		sub.active.Store(false)
	}
	c.subscriptions = make(map[string]*Subscription)
	c.mu.Unlock()

	c.transport.Close()
}

// Subscribe registers a callback for changes on the given table.
//
// Returns a Subscription that can be passed to Unsubscribe.
func (c *Client) Subscribe(table string, callback Callback) (*Subscription, error) {
	if callback == nil {
		return nil, fmt.Errorf("callback must not be nil")
	}

	id := fmt.Sprintf("sub-%d", c.nextID.Add(1))

	sub := &Subscription{
		ID:       id,
		Table:    table,
		callback: callback,
	}
	sub.active.Store(true)

	c.mu.Lock()
	c.subscriptions[id] = sub
	c.mu.Unlock()

	// Send subscribe message to server
	msg := map[string]any{
		"event": "phx_join",
		"topic": fmt.Sprintf("realtime:public:%s", table),
		"ref":   id,
		"payload": map[string]any{
			"config": map[string]any{
				"postgres_changes": []map[string]any{
					{
						"event":  "*",
						"schema": "public",
						"table":  table,
					},
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal subscribe: %w", err)
	}

	if c.transport.IsOpen() {
		if err := c.transport.Send(data); err != nil {
			return nil, fmt.Errorf("send subscribe: %w", err)
		}
	}

	return sub, nil
}

// Unsubscribe removes a subscription.
func (c *Client) Unsubscribe(sub *Subscription) error {
	if sub == nil {
		return nil
	}

	sub.active.Store(false)

	c.mu.Lock()
	delete(c.subscriptions, sub.ID)
	c.mu.Unlock()

	// Send unsubscribe message
	msg := map[string]any{
		"event": "phx_leave",
		"topic": fmt.Sprintf("realtime:public:%s", sub.Table),
		"ref":   sub.ID,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}

	if c.transport.IsOpen() {
		c.transport.Send(data)
	}

	return nil
}

// ActiveSubscriptions returns the count of active subscriptions.
func (c *Client) ActiveSubscriptions() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.subscriptions)
}

// receiveLoop processes incoming WebSocket messages.
func (c *Client) receiveLoop() {
	for c.running.Load() {
		msg, err := c.transport.Receive()
		if err != nil {
			if !c.running.Load() {
				return // intentional shutdown
			}
			// Connection lost - attempt reconnect
			c.reconnect()
			continue
		}

		c.handleMessage(msg)
	}
}

// handleMessage dispatches a received message to matching subscriptions.
func (c *Client) handleMessage(msg []byte) {
	// Parse the Supabase Realtime message format
	var envelope struct {
		Event   string          `json:"event"`
		Topic   string          `json:"topic"`
		Payload json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal(msg, &envelope); err != nil {
		return
	}

	// Only process postgres_changes events
	if envelope.Event != "postgres_changes" {
		return
	}

	// Extract the change data
	var payload struct {
		Data struct {
			Table     string         `json:"table"`
			Type      string         `json:"type"`
			Record    map[string]any `json:"record"`
			OldRecord map[string]any `json:"old_record"`
		} `json:"data"`
	}

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return
	}

	change := ChangeEvent{
		Table:     payload.Data.Table,
		Type:      payload.Data.Type,
		Record:    payload.Data.Record,
		OldRecord: payload.Data.OldRecord,
	}

	// Dispatch to matching subscriptions
	c.mu.RLock()
	for _, sub := range c.subscriptions {
		if sub.active.Load() && sub.Table == change.Table {
			go sub.callback(change) // non-blocking dispatch
		}
	}
	c.mu.RUnlock()
}

// reconnect attempts to re-establish the WebSocket connection
// with exponential backoff.
func (c *Client) reconnect() {
	if c.reconnecting.Load() {
		return // already reconnecting
	}
	c.reconnecting.Store(true)
	defer c.reconnecting.Store(false)

	attempt := 0
	for c.running.Load() {
		attempt++

		if c.config.MaxReconnect > 0 && attempt > c.config.MaxReconnect {
			c.running.Store(false) // stop the receive loop
			return
		}

		// Exponential backoff
		backoff := time.Duration(
			float64(c.config.BaseBackoff) * math.Pow(2, float64(attempt-1)),
		)
		if backoff > c.config.MaxBackoff {
			backoff = c.config.MaxBackoff
		}

		select {
		case <-time.After(backoff):
		case <-c.stopChan:
			return
		}

		if err := c.transport.Connect(); err != nil {
			continue
		}

		// Resubscribe to all active subscriptions
		c.mu.RLock()
		for _, sub := range c.subscriptions {
			if sub.active.Load() {
				msg := map[string]any{
					"event": "phx_join",
					"topic": fmt.Sprintf("realtime:public:%s", sub.Table),
					"ref":   sub.ID,
					"payload": map[string]any{
						"config": map[string]any{
							"postgres_changes": []map[string]any{
								{"event": "*", "schema": "public", "table": sub.Table},
							},
						},
					},
				}
				data, _ := json.Marshal(msg)
				c.transport.Send(data)
			}
		}
		c.mu.RUnlock()

		return // successfully reconnected
	}
}

// heartbeatLoop sends periodic heartbeat messages.
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.transport.IsOpen() {
				msg, _ := json.Marshal(map[string]any{
					"event":   "heartbeat",
					"topic":   "phoenix",
					"payload": map[string]any{},
					"ref":     nil,
				})
				c.transport.Send(msg)
			}
		case <-c.stopChan:
			return
		}
	}
}
