package llm

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// mockAlertPublisher captures published events for test assertions.
type mockAlertPublisher struct {
	mu     sync.Mutex
	events []struct {
		evType string
		data   json.RawMessage
	}
}

func (m *mockAlertPublisher) PublishAsync(evType, _ string, data json.RawMessage, _ string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, struct {
		evType string
		data   json.RawMessage
	}{evType: evType, data: data})
}

func (m *mockAlertPublisher) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockAlertPublisher) lastPayload() *cacheAlertPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	last := m.events[len(m.events)-1]
	var p cacheAlertPayload
	if err := json.Unmarshal(last.data, &p); err != nil {
		return nil
	}
	return &p
}

// makeGatewayWithCacheUsage sets up a gateway with a mock provider that returns
// the given cache token counts, simulating a specific cache hit rate.
// cacheRead / (cacheRead + cacheWrite) = hit rate.
func makeGatewayWithCacheUsage(t *testing.T, cacheRead, cacheWrite int) *Gateway {
	t.Helper()
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Response = &ChatResponse{
		Content:      "ok",
		Model:        "mock-model",
		FinishReason: "stop",
		Usage: TokenUsage{
			InputTokens:      10,
			OutputTokens:     5,
			CacheReadTokens:  cacheRead,
			CacheWriteTokens: cacheWrite,
		},
	}
	gw.Register(mock)
	return gw
}

// TestCacheAlertFired_WhenBelowThreshold verifies that a cache miss alert is
// published when GlobalCacheHitRate is below the configured threshold.
// With cacheRead=10, cacheWrite=90: hit_rate = 10/100 = 0.10, threshold=0.30 → alert fires.
func TestCacheAlertFired_WhenBelowThreshold(t *testing.T) {
	gw := makeGatewayWithCacheUsage(t, 10, 90) // hit rate = 0.10
	pub := &mockAlertPublisher{}
	gw.SetCacheAlert(0.30, pub, "test-project")

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if pub.count() != 1 {
		t.Errorf("expected 1 alert event, got %d", pub.count())
	}

	payload := pub.lastPayload()
	if payload == nil {
		t.Fatal("could not decode alert payload")
	}
	if payload.Threshold != 0.30 {
		t.Errorf("payload.Threshold = %f, want 0.30", payload.Threshold)
	}
	if payload.GlobalCacheHitRate >= 0.30 {
		t.Errorf("payload.GlobalCacheHitRate = %f, expected < 0.30", payload.GlobalCacheHitRate)
	}
	if payload.Provider == "" {
		t.Error("payload.Provider should not be empty")
	}
}

// TestCacheAlertNotFired_WhenAboveThreshold verifies no alert when hit rate is above threshold.
// cacheRead=80, cacheWrite=20: hit_rate = 80/100 = 0.80, threshold=0.30 → no alert.
func TestCacheAlertNotFired_WhenAboveThreshold(t *testing.T) {
	gw := makeGatewayWithCacheUsage(t, 80, 20) // hit rate = 0.80
	pub := &mockAlertPublisher{}
	gw.SetCacheAlert(0.30, pub, "test-project")

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if pub.count() != 0 {
		t.Errorf("expected 0 alert events, got %d", pub.count())
	}
}

// TestCacheAlertDisabled_WhenThresholdZero verifies that setting threshold=0.0
// disables alerting entirely.
func TestCacheAlertDisabled_WhenThresholdZero(t *testing.T) {
	gw := makeGatewayWithCacheUsage(t, 0, 100) // hit rate = 0.0 (worst case)
	pub := &mockAlertPublisher{}
	gw.SetCacheAlert(0.0, pub, "test-project") // disabled

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if pub.count() != 0 {
		t.Errorf("expected 0 alert events when threshold=0.0, got %d", pub.count())
	}
}

// TestCacheAlertFiredOnce_StateTransition verifies that the alert fires only once
// when rate stays below threshold (state transition model: no repeated alerts).
func TestCacheAlertFiredOnce_StateTransition(t *testing.T) {
	gw := makeGatewayWithCacheUsage(t, 5, 95) // hit rate = 0.05
	pub := &mockAlertPublisher{}
	gw.SetCacheAlert(0.30, pub, "test-project")

	// Call Chat multiple times; alert should fire only once.
	for i := 0; i < 5; i++ {
		_, err := gw.Chat(context.Background(), "", &ChatRequest{
			Messages: []Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("Chat() error on call %d: %v", i, err)
		}
	}

	if pub.count() != 1 {
		t.Errorf("expected exactly 1 alert event (state transition), got %d", pub.count())
	}
}

// TestCacheAlertNotFired_WhenNoCacheAttempts verifies no alert when no cache tokens
// are used (cacheAttempts == 0 means GlobalCacheHitRate is not meaningful).
func TestCacheAlertNotFired_WhenNoCacheAttempts(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Response = &ChatResponse{
		Content:      "ok",
		Model:        "mock-model",
		FinishReason: "stop",
		// No cache tokens — cacheAttempts == 0.
		Usage: TokenUsage{InputTokens: 100, OutputTokens: 50},
	}
	gw.Register(mock)

	pub := &mockAlertPublisher{}
	gw.SetCacheAlert(0.30, pub, "test-project")

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if pub.count() != 0 {
		t.Errorf("expected 0 alerts when no cache attempts, got %d", pub.count())
	}
}
