//go:build c3_eventbus

package serve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

	mu         sync.Mutex
	srv        *http.Server
	httpClient *http.Client // reused for health probes
}

// NewWebhookGatewayComponent creates a WebhookGateway component.
// pub may be nil; events will be dropped silently.
func NewWebhookGatewayComponent(host string, port int, doorayCfg config.DoorayWebhookConfig, pub eventbus.Publisher) *WebhookGatewayComponent {
	return &WebhookGatewayComponent{
		host:       host,
		port:       port,
		doorayCfg:  doorayCfg,
		pub:        pub,
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *WebhookGatewayComponent) Name() string { return "webhook-gateway" }

func (c *WebhookGatewayComponent) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.srv != nil {
		return fmt.Errorf("webhook-gateway: already started")
	}

	if c.doorayCfg.CmdToken == "" {
		slog.Warn("webhook-gateway: no cmd_token configured — all inbound requests accepted without authentication")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/webhooks/dooray", c.handleDooray)
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Second,
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
	srv := c.srv
	c.srv = nil // mark stopped before Shutdown so Health() returns "error" immediately
	c.mu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

func (c *WebhookGatewayComponent) Health() ComponentHealth {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()

	if srv == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}

	url := fmt.Sprintf("http://%s:%d/v1/health", c.host, c.port)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return ComponentHealth{Status: "error", Detail: err.Error()}
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return ComponentHealth{Status: "ok"}
	}
	return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
}

// handleDooray handles POST /v1/webhooks/dooray.
// GET requests return 200 OK to satisfy Dooray's URL verification check.
func (c *WebhookGatewayComponent) handleDooray(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}
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
	// Dooray sends appToken (app-level) in every request; cmdToken is per-command and may differ.
	// We accept a match on either field so that operators can configure either token.
	// Security model: accepting appToken match means the configured token grants access to ALL
	// slash commands registered under the same Dooray app. If per-command isolation is required,
	// deploy a separate WebhookGatewayComponent per command with a distinct CmdToken.
	// Note: subtle.ConstantTimeCompare returns 0 immediately when lengths differ (length oracle).
	// This is acceptable for static webhook tokens; HMAC-based dynamic tokens would need padding.
	if c.doorayCfg.CmdToken != "" {
		expected := []byte(c.doorayCfg.CmdToken)
		appMatch := subtle.ConstantTimeCompare(expected, []byte(payload.AppToken))
		cmdMatch := subtle.ConstantTimeCompare(expected, []byte(payload.CmdToken))
		if appMatch != 1 && cmdMatch != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Publish to EventBus (security: omit appToken, cmdToken; include responseUrl for reply routing).
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
			"response_url":  payload.ResponseURL,
		}
		data, _ := json.Marshal(eventData)
		go c.pub.PublishAsync("webhook.dooray.inbound", "dooray", data, "")
	}

	// Respond to Dooray: ephemeral = visible only to the slash command sender.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doorayResponse{
		Text:         "수신 완료",
		ResponseType: "ephemeral",
	})
}
