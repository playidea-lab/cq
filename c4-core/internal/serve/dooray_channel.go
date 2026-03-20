package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// DoorayMessage is a pending Dooray slash-command message received via Hub.
// ID is assigned locally when a message arrives (Hub may not provide one).
type DoorayMessage struct {
	ID          string    `json:"id,omitempty"`
	ChannelID   string    `json:"channelId"`
	SenderID    string    `json:"senderId"`
	SenderName  string    `json:"senderName,omitempty"`
	Text        string    `json:"text"`
	ResponseURL string    `json:"response_url"`
	ReceivedAt  time.Time `json:"received_at"`
}

// DoorayChannelConfig holds configuration for the DoorayChannelComponent.
type DoorayChannelConfig struct {
	HubURL     string // Hub API base URL (e.g. http://localhost:8585)
	WebhookURL string // Dooray Incoming Webhook URL for replies (optional)
}

// DoorayChannelComponent consumes Dooray messages from Hub via WebSocket push.
// It replaces DoorayPollerComponent (HTTP polling) with a WS-based approach:
// Hub pushes messages to connected clients on POST /v1/webhooks/dooray.
//
// Messages are stored in a local buffer with peek semantics:
//   - GetMessages() returns all buffered messages without removing them.
//   - AckMessage(id) removes a specific message from the buffer.
//
// Implements the serve.Component interface.
type DoorayChannelComponent struct {
	cfg    DoorayChannelConfig
	cancel context.CancelFunc

	mu       sync.Mutex
	messages []DoorayMessage // local buffer (peek semantics)
}

// NewDoorayChannel creates a new DoorayChannelComponent.
func NewDoorayChannel(cfg DoorayChannelConfig) *DoorayChannelComponent {
	return &DoorayChannelComponent{cfg: cfg}
}

func (d *DoorayChannelComponent) Name() string { return "dooray-channel" }

func (d *DoorayChannelComponent) Start(ctx context.Context) error {
	ctx, d.cancel = context.WithCancel(ctx)
	go d.runLoop(ctx)
	return nil
}

func (d *DoorayChannelComponent) Stop(_ context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

func (d *DoorayChannelComponent) Health() ComponentHealth {
	return ComponentHealth{Status: "ok"}
}

// GetMessages returns all buffered messages (peek — buffer is NOT cleared).
// Callers must call AckMessage(id) after processing each message.
func (d *DoorayChannelComponent) GetMessages() []DoorayMessage {
	d.mu.Lock()
	out := make([]DoorayMessage, len(d.messages))
	copy(out, d.messages)
	d.mu.Unlock()
	return out
}

// AckMessage removes a message from the buffer by ID (processing complete).
func (d *DoorayChannelComponent) AckMessage(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, m := range d.messages {
		if m.ID == id {
			d.messages = append(d.messages[:i], d.messages[i+1:]...)
			return
		}
	}
}

// runLoop maintains a persistent WS connection with exponential backoff reconnect.
// Backoff resets to 1s after a successful connection (dial succeeded + readLoop ran).
func (d *DoorayChannelComponent) runLoop(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := d.connect(ctx)
		if ctx.Err() != nil {
			return // stopped intentionally
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: dooray-channel: disconnected (%v), reconnect in %s\n", err, backoff)
		} else {
			// connect() returned nil: dial succeeded and readLoop ran; reset backoff.
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connect establishes a single WS connection to Hub and reads messages until
// the connection closes or ctx is cancelled. Returns nil on clean close.
func (d *DoorayChannelComponent) connect(ctx context.Context) error {
	wsURL := hubWSURL(d.cfg.HubURL)
	if wsURL == "" {
		return fmt.Errorf("hub URL not configured")
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, _, _, err := ws.Dial(dialCtx, wsURL)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()

	fmt.Fprintf(os.Stderr, "cq serve: dooray-channel: connected to %s\n", wsURL)

	// Reset backoff on successful connect (signal to caller via nil return on clean close).
	done := make(chan error, 1)
	go func() {
		done <- d.readLoop(conn)
	}()

	select {
	case <-ctx.Done():
		conn.Close()
		<-done
		return nil
	case err := <-done:
		return err
	}
}

// readLoop reads JSON frames from the WS connection and appends them to the buffer.
func (d *DoorayChannelComponent) readLoop(conn net.Conn) error {
	for {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second)) //nolint:errcheck
		data, op, err := wsutil.ReadServerData(conn)
		if err != nil {
			return err
		}
		if op != ws.OpText {
			continue
		}

		var msg DoorayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: dooray-channel: decode error: %v\n", err)
			continue
		}

		// Assign a stable ID if the Hub didn't provide one.
		if msg.ID == "" {
			msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		}

		d.mu.Lock()
		d.messages = append(d.messages, msg)
		d.mu.Unlock()

		sender := msg.SenderName
		if sender == "" {
			sender = msg.SenderID
		}
		fmt.Fprintf(os.Stderr, "\n💬 [두레이] %s: %s\n", sender, msg.Text)
	}
}

// hubWSURL converts the Hub HTTP base URL to a WS URL for GET /v1/dooray/ws.
// e.g. http://localhost:8585 → ws://localhost:8585/v1/dooray/ws
func hubWSURL(hubURL string) string {
	if hubURL == "" {
		return ""
	}
	u, err := url.Parse(hubURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = "/v1/dooray/ws"
	return u.String()
}
