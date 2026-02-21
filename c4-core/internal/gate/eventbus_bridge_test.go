//go:build c8_gate

package gate_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/changmin/c4-core/internal/gate"
)

// mockDispatcher records calls to Trigger.
type mockDispatcher struct {
	calls  []dispatchCall
	retErr error
}

type dispatchCall struct {
	evType string
	data   []byte
}

func (m *mockDispatcher) Trigger(eventType string, data []byte) error {
	m.calls = append(m.calls, dispatchCall{evType: eventType, data: data})
	return m.retErr
}

func TestBridge_Feed_Dispatch(t *testing.T) {
	d := &mockDispatcher{}
	b := gate.NewEventBusBridge(d)

	data := json.RawMessage(`{"task_id":"T-001"}`)
	b.Feed("task.completed", data)

	if len(d.calls) != 1 {
		t.Fatalf("expected 1 Trigger call, got %d", len(d.calls))
	}
	if d.calls[0].evType != "task.completed" {
		t.Errorf("expected evType %q, got %q", "task.completed", d.calls[0].evType)
	}
	if string(d.calls[0].data) != string(data) {
		t.Errorf("expected data %q, got %q", data, d.calls[0].data)
	}
}

func TestBridge_Feed_DispatchError(t *testing.T) {
	d := &mockDispatcher{retErr: errors.New("dispatch failed")}
	b := gate.NewEventBusBridge(d)

	// Must not panic; errors are best-effort ignored.
	b.Feed("hub.job.completed", json.RawMessage(`{}`))

	if len(d.calls) != 1 {
		t.Fatalf("expected 1 Trigger call, got %d", len(d.calls))
	}
}
