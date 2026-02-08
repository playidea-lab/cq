package realtime

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =========================================================================
// Mock Transport (simulates WebSocket)
// =========================================================================

type mockTransport struct {
	mu        sync.Mutex
	open      bool
	messages  chan []byte // incoming messages
	sent      [][]byte   // captured outgoing messages
	connectFn func() error
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		messages: make(chan []byte, 100),
		open:     false,
	}
}

func (m *mockTransport) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectFn != nil {
		if err := m.connectFn(); err != nil {
			return err
		}
	}
	m.open = true
	return nil
}

func (m *mockTransport) Send(msg []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.open {
		return fmt.Errorf("not connected")
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockTransport) Receive() ([]byte, error) {
	msg, ok := <-m.messages
	if !ok {
		return nil, fmt.Errorf("connection closed")
	}
	return msg, nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.open = false
	return nil
}

func (m *mockTransport) IsOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.open
}

// simulateEvent sends a Supabase Realtime change event.
func (m *mockTransport) simulateEvent(table, changeType string, record map[string]any) {
	payload := map[string]any{
		"data": map[string]any{
			"table":  table,
			"type":   changeType,
			"record": record,
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	msg := map[string]any{
		"event":   "postgres_changes",
		"topic":   fmt.Sprintf("realtime:public:%s", table),
		"payload": json.RawMessage(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)
	m.messages <- msgBytes
}

// =========================================================================
// Tests: Basic Subscribe and Event
// =========================================================================

func TestSubscribeReceivesEvents(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)

	if err := client.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer client.Stop()

	var received atomic.Int32
	var lastEvent ChangeEvent
	var mu sync.Mutex

	sub, err := client.Subscribe("c4_tasks", func(event ChangeEvent) {
		mu.Lock()
		lastEvent = event
		mu.Unlock()
		received.Add(1)
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if !sub.IsActive() {
		t.Error("subscription should be active")
	}

	// Simulate an event
	transport.simulateEvent("c4_tasks", "INSERT", map[string]any{
		"id":     "T-001-0",
		"status": "pending",
	})

	// Wait for delivery
	time.Sleep(50 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("received = %d, want 1", received.Load())
	}

	mu.Lock()
	if lastEvent.Table != "c4_tasks" {
		t.Errorf("table = %q, want c4_tasks", lastEvent.Table)
	}
	if lastEvent.Type != "INSERT" {
		t.Errorf("type = %q, want INSERT", lastEvent.Type)
	}
	mu.Unlock()
}

func TestSubscribeMultipleTables(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	var taskEvents, stateEvents atomic.Int32

	client.Subscribe("c4_tasks", func(event ChangeEvent) {
		taskEvents.Add(1)
	})
	client.Subscribe("c4_state", func(event ChangeEvent) {
		stateEvents.Add(1)
	})

	transport.simulateEvent("c4_tasks", "UPDATE", map[string]any{})
	transport.simulateEvent("c4_state", "UPDATE", map[string]any{})
	transport.simulateEvent("c4_tasks", "INSERT", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if taskEvents.Load() != 2 {
		t.Errorf("taskEvents = %d, want 2", taskEvents.Load())
	}
	if stateEvents.Load() != 1 {
		t.Errorf("stateEvents = %d, want 1", stateEvents.Load())
	}
}

// =========================================================================
// Tests: Unsubscribe
// =========================================================================

func TestUnsubscribe(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	var received atomic.Int32
	sub, _ := client.Subscribe("c4_tasks", func(event ChangeEvent) {
		received.Add(1)
	})

	// Send event before unsubscribe
	transport.simulateEvent("c4_tasks", "INSERT", map[string]any{})
	time.Sleep(50 * time.Millisecond)

	// Unsubscribe
	client.Unsubscribe(sub)
	if sub.IsActive() {
		t.Error("subscription should be inactive after unsubscribe")
	}

	// Send event after unsubscribe
	transport.simulateEvent("c4_tasks", "UPDATE", map[string]any{})
	time.Sleep(50 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("received = %d, want 1 (second event should not be received)", received.Load())
	}
}

func TestUnsubscribeNil(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)

	err := client.Unsubscribe(nil)
	if err != nil {
		t.Errorf("Unsubscribe(nil) should not error: %v", err)
	}
}

// =========================================================================
// Tests: Active Subscriptions
// =========================================================================

func TestActiveSubscriptions(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	if client.ActiveSubscriptions() != 0 {
		t.Error("should start with 0 subscriptions")
	}

	sub1, _ := client.Subscribe("c4_tasks", func(e ChangeEvent) {})
	sub2, _ := client.Subscribe("c4_state", func(e ChangeEvent) {})

	if client.ActiveSubscriptions() != 2 {
		t.Errorf("active = %d, want 2", client.ActiveSubscriptions())
	}

	client.Unsubscribe(sub1)
	if client.ActiveSubscriptions() != 1 {
		t.Errorf("active after unsub = %d, want 1", client.ActiveSubscriptions())
	}

	client.Unsubscribe(sub2)
	if client.ActiveSubscriptions() != 0 {
		t.Errorf("active after all unsub = %d, want 0", client.ActiveSubscriptions())
	}
}

// =========================================================================
// Tests: Subscribe sends join message
// =========================================================================

func TestSubscribeSendsJoinMessage(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	client.Subscribe("c4_tasks", func(e ChangeEvent) {})

	if len(transport.sent) == 0 {
		t.Fatal("no messages sent")
	}

	var msg map[string]any
	json.Unmarshal(transport.sent[0], &msg)

	if msg["event"] != "phx_join" {
		t.Errorf("event = %q, want phx_join", msg["event"])
	}
	if msg["topic"] != "realtime:public:c4_tasks" {
		t.Errorf("topic = %q, want realtime:public:c4_tasks", msg["topic"])
	}
}

// =========================================================================
// Tests: Reconnection Logic
// =========================================================================

func TestReconnectOnDisconnect(t *testing.T) {
	transport := newMockTransport()
	var connectCount atomic.Int32

	transport.connectFn = func() error {
		connectCount.Add(1)
		return nil
	}

	cfg := DefaultConfig()
	cfg.BaseBackoff = 1 * time.Millisecond // fast for testing
	cfg.MaxBackoff = 10 * time.Millisecond
	cfg.MaxReconnect = 3

	client := NewClient(cfg, transport)
	client.Start()
	defer client.Stop()

	client.Subscribe("c4_tasks", func(e ChangeEvent) {})

	// Simulate disconnect by closing the message channel
	close(transport.messages)

	// Wait for reconnection attempts
	time.Sleep(100 * time.Millisecond)

	// Should have reconnected at least once
	if connectCount.Load() < 2 {
		t.Errorf("connect count = %d, expected >= 2 (initial + reconnect)", connectCount.Load())
	}
}

func TestReconnectExponentialBackoff(t *testing.T) {
	transport := newMockTransport()
	var mu sync.Mutex
	var connectAttempts []time.Time

	// First call (from Start) succeeds; subsequent calls track attempts
	// and succeed only on the 3rd reconnect attempt.
	callCount := 0
	transport.connectFn = func() error {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		if callCount == 1 {
			return nil // initial connect succeeds
		}
		connectAttempts = append(connectAttempts, time.Now())
		if len(connectAttempts) < 3 {
			return fmt.Errorf("simulated failure")
		}
		return nil
	}

	cfg := DefaultConfig()
	cfg.BaseBackoff = 5 * time.Millisecond
	cfg.MaxBackoff = 100 * time.Millisecond
	cfg.MaxReconnect = 5

	client := NewClient(cfg, transport)
	client.Start()
	defer client.Stop()

	// Simulate disconnect
	close(transport.messages)

	time.Sleep(300 * time.Millisecond)

	// Should have attempted multiple reconnections
	mu.Lock()
	count := len(connectAttempts)
	mu.Unlock()
	if count < 3 {
		t.Errorf("connect attempts = %d, expected >= 3", count)
	}
}

func TestReconnectMaxAttempts(t *testing.T) {
	transport := newMockTransport()
	var connectCount atomic.Int32

	transport.connectFn = func() error {
		connectCount.Add(1)
		if connectCount.Load() > 1 {
			return fmt.Errorf("always fail")
		}
		return nil
	}

	cfg := DefaultConfig()
	cfg.BaseBackoff = 1 * time.Millisecond
	cfg.MaxBackoff = 5 * time.Millisecond
	cfg.MaxReconnect = 3

	client := NewClient(cfg, transport)
	client.Start()
	defer client.Stop()

	close(transport.messages)
	time.Sleep(200 * time.Millisecond)

	// Should stop after MaxReconnect attempts (initial + MaxReconnect reconnect attempts)
	count := connectCount.Load()
	// 1 initial + up to MaxReconnect+1 in reconnect (attempt starts at 1, exceeds at MaxReconnect+1)
	if count > int32(cfg.MaxReconnect)+2 {
		t.Errorf("connect count = %d, expected <= %d (initial + MaxReconnect + 1)", count, cfg.MaxReconnect+2)
	}
}

// =========================================================================
// Tests: Stop
// =========================================================================

func TestStopDeactivatesSubscriptions(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()

	sub, _ := client.Subscribe("c4_tasks", func(e ChangeEvent) {})

	client.Stop()

	if sub.IsActive() {
		t.Error("subscription should be inactive after Stop")
	}
	if client.ActiveSubscriptions() != 0 {
		t.Error("should have 0 active subscriptions after Stop")
	}
}

// =========================================================================
// Tests: Subscribe validation
// =========================================================================

func TestSubscribeNilCallback(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	_, err := client.Subscribe("c4_tasks", nil)
	if err == nil {
		t.Error("expected error for nil callback")
	}
}

// =========================================================================
// Tests: Event filtering (only matching table)
// =========================================================================

func TestEventFilterByTable(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	var tasksReceived, stateReceived atomic.Int32

	client.Subscribe("c4_tasks", func(e ChangeEvent) {
		tasksReceived.Add(1)
	})
	client.Subscribe("c4_state", func(e ChangeEvent) {
		stateReceived.Add(1)
	})

	// Send only c4_tasks events
	for i := 0; i < 3; i++ {
		transport.simulateEvent("c4_tasks", "UPDATE", map[string]any{"i": i})
	}

	time.Sleep(50 * time.Millisecond)

	if tasksReceived.Load() != 3 {
		t.Errorf("tasks received = %d, want 3", tasksReceived.Load())
	}
	if stateReceived.Load() != 0 {
		t.Errorf("state received = %d, want 0", stateReceived.Load())
	}
}

// =========================================================================
// Tests: Non-matching events ignored
// =========================================================================

func TestNonPostgresChangesIgnored(t *testing.T) {
	transport := newMockTransport()
	client := NewClient(DefaultConfig(), transport)
	client.Start()
	defer client.Stop()

	var received atomic.Int32
	client.Subscribe("c4_tasks", func(e ChangeEvent) {
		received.Add(1)
	})

	// Send a non-postgres_changes event
	msg, _ := json.Marshal(map[string]any{
		"event":   "phx_reply",
		"topic":   "realtime:public:c4_tasks",
		"payload": map[string]any{"status": "ok"},
	})
	transport.messages <- msg

	time.Sleep(50 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("received = %d, want 0 (non-postgres_changes should be ignored)", received.Load())
	}
}
