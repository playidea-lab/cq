//go:build c8_gate

package gate

import (
	"encoding/json"
)

// EventDispatcher abstracts the target that receives forwarded EventBus events.
// WebhookManager implements this interface via Trigger.
type EventDispatcher interface {
	Trigger(eventType string, data []byte) error
}

// EventBusBridge forwards EventBus events to an EventDispatcher (best-effort).
type EventBusBridge struct {
	dispatcher EventDispatcher
}

// NewEventBusBridge creates a bridge that forwards events to d.
func NewEventBusBridge(d EventDispatcher) *EventBusBridge {
	return &EventBusBridge{dispatcher: d}
}

// Feed forwards the event to the dispatcher, ignoring errors (best-effort).
func (b *EventBusBridge) Feed(evType string, data json.RawMessage) {
	_ = b.dispatcher.Trigger(evType, []byte(data))
}

// Trigger implements EventDispatcher for *WebhookManager.
// It constructs a gate.Event and dispatches it to all matching endpoints.
func (m *WebhookManager) Trigger(eventType string, data []byte) error {
	return m.Dispatch(Event{
		Type: eventType,
		Data: json.RawMessage(data),
	})
}
