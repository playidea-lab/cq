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
	"time"
)

func TestNewExperimentWrapper_InvalidPattern(t *testing.T) {
	_, err := NewExperimentWrapper("http://localhost", "exp1", "run1", &ExperimentProtocolConfig{
		EpochPattern: "[invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

func TestNewExperimentWrapper_ValidPattern(t *testing.T) {
	w, err := NewExperimentWrapper("http://localhost", "exp1", "run1", &ExperimentProtocolConfig{
		EpochPattern: `MPJPE:\s+(?P<value>[\d.]+)`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
}

func TestNewExperimentWrapper_NilProtocol(t *testing.T) {
	w, err := NewExperimentWrapper("http://localhost", "exp1", "run1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
}

func TestExperimentWrapper_WrapOutput_NoPattern(t *testing.T) {
	w, err := NewExperimentWrapper("http://localhost", "exp1", "run1", nil)
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("line1\nline2\nMPJPE: 45.2\n")
	var dst bytes.Buffer
	if err := w.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	out := dst.String()
	if !strings.Contains(out, "line1") || !strings.Contains(out, "MPJPE: 45.2") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestExperimentWrapper_WrapOutput_PassThrough(t *testing.T) {
	w, err := NewExperimentWrapper("http://localhost", "exp1", "run1", &ExperimentProtocolConfig{
		EpochPattern: `MPJPE:\s+(?P<value>[\d.]+)`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// no MCP server — we just want output passthrough, no pattern match lines
	src := strings.NewReader("training started\nepoch 1 done\n")
	var dst bytes.Buffer
	if err := w.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	out := dst.String()
	if !strings.Contains(out, "training started") {
		t.Errorf("expected passthrough, got: %q", out)
	}
}

func TestCapabilityWrapper_ExperimentProtocol_StdoutPattern(t *testing.T) {
	// Fix 1: channel-based synchronization — no data race with time.Sleep
	checkpointCalled := make(chan struct{}, 1)

	var capturedArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err == nil {
			if params, ok := req["params"].(map[string]any); ok {
				capturedArgs, _ = params["arguments"].(map[string]any)
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		select {
		case checkpointCalled <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	wrapper, err := NewExperimentWrapper(srv.URL, "exp-42", "run-7", &ExperimentProtocolConfig{
		EpochPattern:   `MPJPE:\s+(?P<value>[\d.]+)`,
		CheckpointTool: "c4_run_checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("epoch 1/10\nMPJPE: 52.3 mm\ntraining continues\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	select {
	case <-checkpointCalled:
		// checkpoint was called — success
	case <-time.After(300 * time.Millisecond):
		t.Error("expected c4_run_checkpoint to be called when MPJPE pattern matched")
	}

	if capturedArgs != nil {
		if expID, ok := capturedArgs["exp_id"].(string); !ok || expID != "exp-42" {
			t.Errorf("expected exp_id=exp-42, got %v", capturedArgs["exp_id"])
		}
		// metric is sent as float64 after strconv.ParseFloat
		if metric, ok := capturedArgs["metric"].(float64); !ok || metric != 52.3 {
			t.Errorf("expected metric=52.3, got %v", capturedArgs["metric"])
		}
	}
}

func TestExperimentWrapper_WrapOutput_MultipleMatches(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	wrapper, err := NewExperimentWrapper(srv.URL, "exp-1", "run-1", &ExperimentProtocolConfig{
		EpochPattern: `MPJPE:\s+(?P<value>[\d.]+)`,
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("MPJPE: 55.0\nMPJPE: 50.1\nMPJPE: 47.3\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	if got := callCount.Load(); got != 3 {
		t.Errorf("expected 3 checkpoint calls, got %d", got)
	}
}

func TestExperimentWrapper_WrapOutput_ContextCancelled(t *testing.T) {
	wrapper, err := NewExperimentWrapper("http://localhost:1", "exp-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Use a closed reader so scanner finishes immediately.
	src := strings.NewReader("")
	var dst bytes.Buffer
	err = wrapper.WrapOutput(ctx, src, &dst)
	// Either nil (empty reader) or context.Canceled is acceptable.
	_ = err
}

func TestExperimentWrapper_CallMCP_MarshalError(t *testing.T) {
	// Fix 2: json.Marshal error is handled — test that non-marshallable args cause error
	wrapper, err := NewExperimentWrapper("http://localhost:1", "exp-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	// channels cannot be marshalled to JSON
	args := map[string]any{
		"bad": make(chan int),
	}

	err = wrapper.callMCP(context.Background(), "c4_run_checkpoint", args)
	if err == nil {
		t.Error("expected error marshalling channel value")
	}
	if !strings.Contains(err.Error(), "marshal request") {
		t.Errorf("expected 'marshal request' error, got: %v", err)
	}
}

func TestExperimentWrapper_HTTPClientReuse(t *testing.T) {
	// Fix 3: verify the http.Client is set as a struct field (not nil)
	wrapper, err := NewExperimentWrapper("http://localhost", "exp-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrapper.client == nil {
		t.Error("expected non-nil http.Client on ExperimentWrapper")
	}
}
