//go:build hub

package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestDispatchJob(t *testing.T) {
	reg := mcp.NewRegistry()
	RegisterDispatchHandler(reg)

	t.Run("accepted with valid job_id", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"job_id":  "job-abc-123",
			"command": "echo hello",
			"workdir": "/tmp",
		})
		result, err := handleDispatchJob(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("expected map result, got %T", result)
		}
		if m["status"] != "accepted" {
			t.Errorf("expected status=accepted, got %v", m["status"])
		}
		if m["job_id"] != "job-abc-123" {
			t.Errorf("expected job_id=job-abc-123, got %v", m["job_id"])
		}
	})

	t.Run("error on missing job_id", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{"command": "echo hello"})
		_, err := handleDispatchJob(raw)
		if err == nil {
			t.Fatal("expected error for missing job_id")
		}
	})
}
