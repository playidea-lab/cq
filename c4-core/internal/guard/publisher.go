package guard

import "encoding/json"

// EventPublisher is a local interface for publishing guard events to the event bus.
// Defined here to avoid a direct dependency on the eventbus package (dependency inversion).
type EventPublisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}
