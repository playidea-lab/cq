package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestJobExecutor_ExperimentWrapper_Activated verifies that when ExperimentProtocol
// is configured and exp_run_id is present in the payload, WrapOutput is called
// and the MCP checkpoint endpoint receives the metric.
func TestJobExecutor_ExperimentWrapper_Activated(t *testing.T) {
	var called atomic.Int32
	var capturedMetric float64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err == nil {
			if params, ok := req["params"].(map[string]any); ok {
				if args, ok := params["arguments"].(map[string]any); ok {
					if v, ok := args["metric"].(float64); ok {
						capturedMetric = v
					}
				}
			}
		}
		called.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	cfg := &WorkerConfig{
		MCPURL: srv.URL,
		ExperimentProtocol: &ExperimentProtocolConfig{
			MetricKey:      "loss",
			CheckpointTool: "c4_run_checkpoint",
		},
	}
	payload := JobPayload{
		ExpRunID: "run-abc",
		ExpID:    "exp-42",
	}

	src := strings.NewReader("epoch 1\n@loss=48.5 @epoch=1\ndone\n")
	var dst bytes.Buffer

	if err := ExecuteWithExperiment(context.Background(), cfg, payload, src, &dst); err != nil {
		t.Fatalf("ExecuteWithExperiment error: %v", err)
	}

	if n := called.Load(); n != 1 {
		t.Errorf("expected 1 MCP checkpoint call, got %d", n)
	}
	if capturedMetric != 48.5 {
		t.Errorf("expected metric=48.5, got %v", capturedMetric)
	}
	if !strings.Contains(dst.String(), "@loss=48.5") {
		t.Errorf("expected output to contain the matched line, got: %q", dst.String())
	}
}

// TestJobExecutor_ExperimentWrapper_Skipped verifies that when ExperimentProtocol
// is nil, no ExperimentWrapper is created and output is passed through.
func TestJobExecutor_ExperimentWrapper_Skipped(t *testing.T) {
	cfg := &WorkerConfig{
		MCPURL:             "http://unreachable-host",
		ExperimentProtocol: nil, // no protocol — wrapper must NOT be activated
	}
	payload := JobPayload{ExpRunID: "run-xyz"}

	src := strings.NewReader("@loss=0.55\ndone\n")
	var dst bytes.Buffer

	if err := ExecuteWithExperiment(context.Background(), cfg, payload, src, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(dst.String(), "@loss=0.55") {
		t.Errorf("expected pass-through output, got: %q", dst.String())
	}
}

// TestJobExecutor_ExperimentWrapper_MissingRunID verifies that when ExperimentProtocol
// is configured but exp_run_id is empty, the wrapper is skipped with a warning
// and output is still passed through unchanged.
func TestJobExecutor_ExperimentWrapper_MissingRunID(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &WorkerConfig{
		MCPURL: srv.URL,
		ExperimentProtocol: &ExperimentProtocolConfig{
			MetricKey: "loss",
		},
	}
	payload := JobPayload{
		ExpRunID: "", // empty — wrapper must be skipped
	}

	src := strings.NewReader("@loss=0.60\ndone\n")
	var dst bytes.Buffer

	if err := ExecuteWithExperiment(context.Background(), cfg, payload, src, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 0 {
		t.Errorf("expected 0 MCP calls when run_id is empty, got %d", callCount)
	}
	if !strings.Contains(dst.String(), "@loss=0.60") {
		t.Errorf("expected pass-through output, got: %q", dst.String())
	}
}
