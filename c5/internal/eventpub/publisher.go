// Package eventpub provides a lightweight HTTP client for publishing events
// to the C3 EventBus /v1/events/publish endpoint.
package eventpub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Publisher sends job lifecycle events to the C3 EventBus.
// When url is empty the publisher is disabled (noop).
type Publisher struct {
	url    string
	token  string
	client *http.Client
}

// New creates a Publisher. If url is empty, the publisher is disabled and all
// Publish calls are silently ignored.
func New(url, token string) *Publisher {
	return &Publisher{
		url:   url,
		token: token,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsEnabled reports whether the publisher has a configured endpoint.
func (p *Publisher) IsEnabled() bool {
	return p.url != ""
}

// Publish sends an event to the EventBus. It returns an error on network or
// non-2xx response so the caller can log and ignore it.
func (p *Publisher) Publish(evType, source string, data map[string]any) error {
	if !p.IsEnabled() {
		return nil
	}

	payload := map[string]any{
		"type":   evType,
		"source": source,
		"data":   data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("eventpub: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.url+"/v1/events/publish", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("eventpub: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("eventpub: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("eventpub: unexpected status %d", resp.StatusCode)
	}

	return nil
}
