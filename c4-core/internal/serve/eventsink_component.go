//go:build c3_eventbus

package serve

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers/eventbushandler"
)

// EventSinkComponent wraps eventbushandler.StartEventSinkServer as a Component.
type EventSinkComponent struct {
	port  int
	token string
	pub   eventbus.Publisher

	mu  sync.Mutex
	srv *http.Server
}

// NewEventSinkComponent creates an EventSink component.
// pub may be nil; StartEventSinkServer handles that case.
func NewEventSinkComponent(port int, token string, pub eventbus.Publisher) *EventSinkComponent {
	return &EventSinkComponent{
		port:  port,
		token: token,
		pub:   pub,
	}
}

func (c *EventSinkComponent) Name() string { return "eventsink" }

func (c *EventSinkComponent) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	srv, err := eventbushandler.StartEventSinkServer(c.port, c.token, c.pub)
	if err != nil {
		return fmt.Errorf("eventsink start: %w", err)
	}
	c.srv = srv
	return nil
}

func (c *EventSinkComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.srv == nil {
		return nil
	}
	err := c.srv.Shutdown(ctx)
	c.srv = nil
	return err
}

func (c *EventSinkComponent) Health() ComponentHealth {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()

	if srv == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}

	url := fmt.Sprintf("http://localhost:%d/v1/events/publish", c.port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ComponentHealth{Status: "error", Detail: err.Error()}
	}
	resp.Body.Close()

	// GET returns 405 Method Not Allowed — that means the server is alive.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return ComponentHealth{Status: "ok"}
	}
	return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
}
