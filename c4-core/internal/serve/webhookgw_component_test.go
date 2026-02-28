//go:build c3_eventbus

package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
)

func TestWebhookGatewayComponent_Name(t *testing.T) {
	comp := NewWebhookGatewayComponent("127.0.0.1", 0, config.DoorayWebhookConfig{}, nil)
	if comp.Name() != "webhook-gateway" {
		t.Errorf("Name() = %q, want %q", comp.Name(), "webhook-gateway")
	}
}

func TestWebhookGatewayComponent_HealthBeforeStart(t *testing.T) {
	comp := NewWebhookGatewayComponent("127.0.0.1", 0, config.DoorayWebhookConfig{}, nil)
	h := comp.Health()
	if h.Status != "error" {
		t.Errorf("Health before start = %q, want %q", h.Status, "error")
	}
}

func TestWebhookGatewayComponent_StartStop(t *testing.T) {
	port := freePort(t)
	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, nil)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	h := comp.Health()
	if h.Status != "ok" {
		t.Errorf("Health after start = %q (%s), want %q", h.Status, h.Detail, "ok")
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	h = comp.Health()
	if h.Status != "error" {
		t.Errorf("Health after stop = %q, want %q", h.Status, "error")
	}
}

func TestWebhookGatewayComponent_StopIdempotent(t *testing.T) {
	comp := NewWebhookGatewayComponent("127.0.0.1", 0, config.DoorayWebhookConfig{}, nil)
	if err := comp.Stop(context.Background()); err != nil {
		t.Errorf("Stop without start: %v", err)
	}
}

func TestWebhookGatewayComponent_DoorayHandlerBasic(t *testing.T) {
	port := freePort(t)
	spy := &spyPublisher{}
	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, spy)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	payload := DoorayInbound{
		ChannelName:  "dev",
		UserNickname: "홍길동",
		Text:         "테스트 메시지",
		Command:      "/cq",
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if len(spy.calls) == 0 {
		t.Error("spy publisher received no events")
	}
	if spy.calls[0] != "webhook.dooray.inbound" {
		t.Errorf("event type = %q, want %q", spy.calls[0], "webhook.dooray.inbound")
	}
}

func TestWebhookGatewayComponent_DoorayResponseFormat(t *testing.T) {
	port := freePort(t)
	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, &spyPublisher{})

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(DoorayInbound{Text: "hello"})
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	var result doorayResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ResponseType != "ephemeral" {
		t.Errorf("responseType = %q, want %q", result.ResponseType, "ephemeral")
	}
	if result.Text == "" {
		t.Error("text should not be empty")
	}
}

func TestWebhookGatewayComponent_TokenVerification(t *testing.T) {
	port := freePort(t)
	doorayCfg := config.DoorayWebhookConfig{CmdToken: "secret-token"}
	spy := &spyPublisher{}
	comp := NewWebhookGatewayComponent("127.0.0.1", port, doorayCfg, spy)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)

	t.Run("wrong token rejected", func(t *testing.T) {
		body, _ := json.Marshal(DoorayInbound{CmdToken: "wrong", Text: "hi"})
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		if len(spy.calls) != 0 {
			t.Error("event should not be published on wrong token")
		}
	})

	t.Run("correct token accepted", func(t *testing.T) {
		body, _ := json.Marshal(DoorayInbound{CmdToken: "secret-token", Text: "hi"})
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if len(spy.calls) == 0 {
			t.Error("event should be published on correct token")
		}
	})
}

func TestWebhookGatewayComponent_MethodNotAllowed(t *testing.T) {
	port := freePort(t)
	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, nil)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestWebhookGatewayComponent_NilPublisher(t *testing.T) {
	port := freePort(t)
	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, nil)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	// nil publisher should not panic.
	body, _ := json.Marshal(DoorayInbound{Text: "no-pub"})
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWebhookGatewayComponent_EventDataSecurity(t *testing.T) {
	port := freePort(t)

	// Use a custom spy that captures the raw event data.
	type eventCapture struct {
		evType string
		data   json.RawMessage
	}
	captures := make([]eventCapture, 0)
	type capturingPublisher struct{}

	// We need to capture data, so wire a spyDataPublisher inline.
	var captured []json.RawMessage
	pub := &dataCapturingPublisher{fn: func(data json.RawMessage) { captured = append(captured, data) }}

	comp := NewWebhookGatewayComponent("127.0.0.1", port, config.DoorayWebhookConfig{}, pub)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)
	time.Sleep(50 * time.Millisecond)

	payload := DoorayInbound{
		CmdToken:    "should-not-appear",
		AppToken:    "also-secret",
		ResponseURL: "https://secret-url",
		Text:        "hello",
		UserNickname: "tester",
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/webhooks/dooray", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if len(captured) == 0 {
		t.Fatal("no event captured")
	}

	var eventData map[string]any
	if err := json.Unmarshal(captured[0], &eventData); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	// Sensitive fields must not be in the published event.
	for _, forbidden := range []string{"cmd_token", "cmdToken", "app_token", "appToken", "response_url", "responseUrl"} {
		if _, ok := eventData[forbidden]; ok {
			t.Errorf("sensitive field %q should not be in event data", forbidden)
		}
	}

	// Safe fields must be present.
	if eventData["user_nickname"] != "tester" {
		t.Errorf("user_nickname = %v, want %q", eventData["user_nickname"], "tester")
	}
	if eventData["text"] != "hello" {
		t.Errorf("text = %v, want %q", eventData["text"], "hello")
	}
	_ = captures
}

// dataCapturingPublisher captures event data for test assertions.
type dataCapturingPublisher struct {
	fn func(json.RawMessage)
}

func (p *dataCapturingPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	if p.fn != nil {
		p.fn(data)
	}
}
