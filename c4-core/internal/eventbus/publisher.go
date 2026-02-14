package eventbus

import (
	"encoding/json"
	"fmt"
	"os"
)

// Publisher is a lightweight interface for components to publish events
// without depending on the full gRPC client.
type Publisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}

// NoopPublisher is a no-op publisher used when eventbus is disabled.
type NoopPublisher struct{}

func (NoopPublisher) PublishAsync(string, string, json.RawMessage, string) {}

// LogPublisher logs events to stderr instead of sending to gRPC (for debugging).
type LogPublisher struct{}

func (LogPublisher) PublishAsync(evType, source string, data json.RawMessage, _ string) {
	fmt.Fprintf(os.Stderr, "c4: eventbus: [noop] %s from %s: %s\n", evType, source, string(data))
}
