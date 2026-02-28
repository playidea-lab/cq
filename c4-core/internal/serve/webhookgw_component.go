//go:build c3_eventbus

package serve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
)

// DoorayInbound is the POST body sent by Dooray Slash Command.
// Field names follow the NHN Cloud Dooray API specification.
type DoorayInbound struct {
	TenantID     string `json:"tenantId"`
	TenantDomain string `json:"tenantDomain"`
	ChannelID    string `json:"channelId"`
	ChannelName  string `json:"channelName"`
	UserID       string `json:"userId"`
	UserNickname string `json:"userNickname"`
	Command      string `json:"command"`
	Text         string `json:"text"`
	ResponseURL  string `json:"responseUrl"`
	AppToken     string `json:"appToken"`
	CmdToken     string `json:"cmdToken"`
	TriggerID    string `json:"triggerId"`
}

// doorayResponse is the JSON response body returned to Dooray after handling the slash command.
type doorayResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"responseType"`
}

// WebhookGatewayComponent is an HTTP server that receives inbound webhooks from
// external services (e.g. Dooray slash commands) and publishes events to the local EventBus.
type WebhookGatewayComponent struct {
	host      string
	port      int
	doorayCfg config.DoorayWebhookConfig
	pub       eventbus.Publisher

	mu  sync.Mutex
	srv *http.Server
}

// NewWebhookGatewayComponent creates a WebhookGateway component.
// pub may be nil; events will be dropped silently.
func NewWebhookGatewayComponent(host string, port int, doorayCfg config.DoorayWebhookConfig, pub eventbus.Publisher) *WebhookGatewayComponent {
	return &WebhookGatewayComponent{
		host:      host,
		port:      port,
		doorayCfg: doorayCfg,
		pub:       pub,
	}
}

func (c *WebhookGatewayComponent) Name() string { return "webhook-gateway" }

func (c *WebhookGatewayComponent) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/webhooks/dooray", c.handleDooray)

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("webhook-gateway listen %s: %w", addr, err)
	}

	go func() { _ = srv.Serve(ln) }()
	c.srv = srv
	return nil
}

func (c *WebhookGatewayComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.srv == nil {
		return nil
	}
	err := c.srv.Shutdown(ctx)
	c.srv = nil
	return err
}

func (c *WebhookGatewayComponent) Health() ComponentHealth {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()

	if srv == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}

	url := fmt.Sprintf("http://%s:%d/v1/webhooks/dooray", c.host, c.port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ComponentHealth{Status: "error", Detail: err.Error()}
	}
	resp.Body.Close()

	// GET returns 405 Method Not Allowed — server is alive.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return ComponentHealth{Status: "ok"}
	}
	return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
}

// handleDooray handles POST /v1/webhooks/dooray.
func (c *WebhookGatewayComponent) handleDooray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload DoorayInbound
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Token verification (skip if no token configured).
	if c.doorayCfg.CmdToken != "" {
		expected := []byte(c.doorayCfg.CmdToken)
		got := []byte(payload.CmdToken)
		if subtle.ConstantTimeCompare(expected, got) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Publish to EventBus (security: omit appToken, cmdToken, responseUrl).
	if c.pub != nil {
		eventData := map[string]any{
			"source":        "dooray",
			"tenant_id":     payload.TenantID,
			"tenant_domain": payload.TenantDomain,
			"channel_id":    payload.ChannelID,
			"channel_name":  payload.ChannelName,
			"user_id":       payload.UserID,
			"user_nickname": payload.UserNickname,
			"text":          payload.Text,
			"command":       payload.Command,
			"trigger_id":    payload.TriggerID,
		}
		data, _ := json.Marshal(eventData)
		c.pub.PublishAsync("webhook.dooray.inbound", "dooray", data, "")
	}

	// Respond to Dooray: ephemeral = visible only to the slash command sender.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doorayResponse{
		Text:         "수신 완료",
		ResponseType: "ephemeral",
	})
}
