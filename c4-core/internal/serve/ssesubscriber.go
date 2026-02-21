//go:build c5_hub && c3_eventbus

package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
)

const (
	sseBackoffBase    = 1 * time.Second
	sseBackoffMax     = 30 * time.Second
	sseDegradedAfter  = 3
	sseBackoffJitter  = 0.2 // ±20%
)

// SSESubscriberConfig holds configuration for SSESubscriberComponent.
type SSESubscriberConfig struct {
	// URL is the C5 Hub base URL (e.g. "http://localhost:8585").
	URL string
	// APIKey is the Bearer token for authentication.
	APIKey string
	// ProjectID is forwarded to EventBus events.
	ProjectID string
}

// SSESubscriberComponent connects to C5 /v1/events/stream via SSE and
// forwards received events to the local EventBus. It implements Component.
type SSESubscriberComponent struct {
	cfg        SSESubscriberConfig
	pub        eventbus.Publisher
	httpClient *http.Client // reused across connect() calls

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	failCount int
	running   bool
}

// NewSSESubscriberComponent creates a new SSESubscriberComponent.
func NewSSESubscriberComponent(cfg SSESubscriberConfig, pub eventbus.Publisher) *SSESubscriberComponent {
	return &SSESubscriberComponent{
		cfg: cfg,
		pub: pub,
		httpClient: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
	}
}

func (c *SSESubscriberComponent) Name() string { return "ssesubscriber" }

// Start launches the background reconnect loop.
func (c *SSESubscriberComponent) Start(ctx context.Context) error {
	if c.cfg.URL == "" {
		return fmt.Errorf("ssesubscriber: hub URL not configured")
	}

	innerCtx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})

	c.mu.Lock()
	c.cancel = cancel
	c.done = done
	c.failCount = 0
	c.running = true
	c.mu.Unlock()

	go c.reconnectLoop(innerCtx, done)

	return nil
}

// Stop cancels the context and waits for the background goroutine to exit.
func (c *SSESubscriberComponent) Stop(_ context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	done := c.done
	c.running = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

// Health returns the component's current health.
func (c *SSESubscriberComponent) Health() ComponentHealth {
	c.mu.Lock()
	running := c.running
	fails := c.failCount
	c.mu.Unlock()

	if !running {
		return ComponentHealth{Status: "stopped", Detail: "not started"}
	}
	if fails >= sseDegradedAfter {
		return ComponentHealth{
			Status: "degraded",
			Detail: fmt.Sprintf("reconnect failed %d consecutive times", fails),
		}
	}
	return ComponentHealth{Status: "ok"}
}

// reconnectLoop tries to connect to the SSE endpoint, reads events, and
// reconnects with exponential backoff on failure.
func (c *SSESubscriberComponent) reconnectLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}

		err := c.connect(ctx)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			attempt++
			c.mu.Lock()
			c.failCount = attempt
			c.mu.Unlock()
		} else {
			// Clean disconnect — reset failure count.
			attempt = 0
			c.mu.Lock()
			c.failCount = 0
			c.mu.Unlock()
			continue
		}

		// Exponential backoff with ±20% jitter.
		// Cap the exponent at 30 to prevent int64 overflow on large attempt counts.
		exp := attempt - 1
		if exp > 30 {
			exp = 30
		}
		backoff := sseBackoffBase * (1 << uint(exp))
		if backoff > sseBackoffMax {
			backoff = sseBackoffMax
		}
		jitter := float64(backoff) * sseBackoffJitter * (rand.Float64()*2 - 1)
		delay := backoff + time.Duration(jitter)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// connect opens one SSE connection and reads events until it disconnects or errors.
func (c *SSESubscriberComponent) connect(ctx context.Context) error {
	url := strings.TrimRight(c.cfg.URL, "/") + "/v1/events/stream"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.APIKey)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Reset fail count on successful connection.
	c.mu.Lock()
	c.failCount = 0
	c.mu.Unlock()

	scanner := bufio.NewScanner(resp.Body)
	// Raise token buffer to 1 MiB to handle large SSE event payloads.
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimPrefix(line, "data:")
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}

		// Forward to EventBus.
		c.publishEvent(payload)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	// EOF — clean disconnect.
	return nil
}

// publishEvent forwards a raw SSE data payload to the EventBus.
func (c *SSESubscriberComponent) publishEvent(payload string) {
	if c.pub == nil {
		return
	}
	c.pub.PublishAsync(
		"hub.sse.event",
		"ssesubscriber",
		json.RawMessage(payload),
		c.cfg.ProjectID,
	)
}
