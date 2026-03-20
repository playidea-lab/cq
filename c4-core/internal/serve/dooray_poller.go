package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// DoorayPollerConfig holds configuration for the Dooray message poller.
type DoorayPollerConfig struct {
	HubURL       string        // Hub API base URL
	PollInterval time.Duration // polling interval (default: 3s)
	WebhookURL   string        // Incoming Webhook URL for replies
}

// DoorayMessage is a pending message from the Hub.
type DoorayMessage struct {
	ChannelID   string    `json:"channelId"`
	SenderID    string    `json:"senderId"`
	SenderName  string    `json:"senderName,omitempty"`
	Text        string    `json:"text"`
	ResponseURL string    `json:"response_url"`
	ReceivedAt  time.Time `json:"received_at"`
}

// DoorayPollerComponent polls Hub /v1/dooray/pending and stores messages
// for MCP tool access. Implements the Component interface.
type DoorayPollerComponent struct {
	cfg    DoorayPollerConfig
	cancel context.CancelFunc

	mu       sync.Mutex
	messages []DoorayMessage
}

// NewDoorayPoller creates a new Dooray poller component.
func NewDoorayPoller(cfg DoorayPollerConfig) *DoorayPollerComponent {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 3 * time.Second
	}
	return &DoorayPollerComponent{cfg: cfg}
}

func (d *DoorayPollerComponent) Name() string { return "dooray-poller" }

func (d *DoorayPollerComponent) Start(ctx context.Context) error {
	ctx, d.cancel = context.WithCancel(ctx)
	go d.loop(ctx)
	return nil
}

func (d *DoorayPollerComponent) Stop(_ context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

func (d *DoorayPollerComponent) Health() ComponentHealth {
	return ComponentHealth{Status: "ok"}
}

// PopAll returns and clears all pending messages (for MCP tool).
func (d *DoorayPollerComponent) PopAll() []DoorayMessage {
	d.mu.Lock()
	msgs := d.messages
	d.messages = nil
	d.mu.Unlock()
	return msgs
}

// Reply sends a message to the configured Dooray Incoming Webhook.
func (d *DoorayPollerComponent) Reply(ctx context.Context, text string) error {
	if d.cfg.WebhookURL == "" {
		return fmt.Errorf("dooray webhook URL not configured")
	}
	body, _ := json.Marshal(map[string]string{
		"botName": "CQ",
		"text":    text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
	return nil
}

func (d *DoorayPollerComponent) loop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}
	url := d.cfg.HubURL + "/v1/dooray/pending"

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.poll(ctx, client, url)
		}
	}
}

func (d *DoorayPollerComponent) poll(ctx context.Context, client *http.Client, url string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
		return
	}

	var msgs []DoorayMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil || len(msgs) == 0 {
		return
	}

	d.mu.Lock()
	d.messages = append(d.messages, msgs...)
	d.mu.Unlock()

	// Print to stderr so it appears in Claude Code terminal
	for _, m := range msgs {
		sender := m.SenderName
		if sender == "" {
			sender = m.SenderID
		}
		fmt.Fprintf(os.Stderr, "\n💬 [두레이] %s: %s\n", sender, m.Text)
	}
}

