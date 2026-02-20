// Package gate provides external integration primitives:
// Webhook ingress/egress, cron scheduling, and platform connectors
// (Slack, GitHub).
package gate

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is the payload dispatched through WebhookManager endpoints.
type Event struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Source string          `json:"source,omitempty"`
	Data   json.RawMessage `json:"data"`
}

// Endpoint represents a registered webhook destination.
type Endpoint struct {
	Name   string
	URL    string
	Secret string
	Events []string // subscribed event types; empty = all events
}

// WebhookConfig holds global settings for WebhookManager.
type WebhookConfig struct {
	DefaultTimeout time.Duration
	MaxRetries     int
}

// WebhookManager manages outbound webhook endpoints and dispatches events.
type WebhookManager struct {
	mu         sync.RWMutex
	endpoints  []*Endpoint
	cfg        WebhookConfig
	httpClient *http.Client
}

// NewWebhookManager creates a WebhookManager with the given configuration.
func NewWebhookManager(cfg WebhookConfig) *WebhookManager {
	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &WebhookManager{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// RegisterEndpoint adds a new webhook destination.
// events is the list of event types to subscribe; empty means all events.
// Returns the created Endpoint.
func (m *WebhookManager) RegisterEndpoint(name, url, secret string, events []string) *Endpoint {
	ep := &Endpoint{
		Name:   name,
		URL:    url,
		Secret: secret,
		Events: events,
	}
	m.mu.Lock()
	m.endpoints = append(m.endpoints, ep)
	m.mu.Unlock()
	return ep
}

// Dispatch sends the event to all matching registered endpoints.
// Returns the first error encountered; other endpoints still receive the event.
func (m *WebhookManager) Dispatch(event Event) error {
	m.mu.RLock()
	eps := make([]*Endpoint, len(m.endpoints))
	copy(eps, m.endpoints)
	m.mu.RUnlock()

	var firstErr error
	for _, ep := range eps {
		if !matchesEvents(ep.Events, event.Type) {
			continue
		}
		if err := m.post(ep, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// matchesEvents reports whether eventType matches the subscription list.
// An empty list matches all event types.
func matchesEvents(subscribed []string, eventType string) bool {
	if len(subscribed) == 0 {
		return true
	}
	for _, s := range subscribed {
		if s == eventType {
			return true
		}
	}
	return false
}

// post performs a single HTTP POST to the endpoint.
func (m *WebhookManager) post(ep *Endpoint, event Event) error {
	if event.Source == "" {
		event.Source = "c4.gate"
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if ep.Secret != "" {
		mac := hmac.New(sha256.New, []byte(ep.Secret))
		mac.Write(body)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Gate-Signature", sig)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST to %s: %w", ep.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s returned HTTP %d", ep.Name, resp.StatusCode)
	}
	return nil
}
