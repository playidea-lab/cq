package eventbus

import (
	"encoding/json"
)

// Publisher is a lightweight interface for components to publish events
// without depending on the full gRPC client.
type Publisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}

// NoopPublisher is a no-op publisher used when eventbus is disabled.
type NoopPublisher struct{}

func (NoopPublisher) PublishAsync(string, string, json.RawMessage, string) {}

