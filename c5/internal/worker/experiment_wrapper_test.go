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

// --- parseAtKeyValues unit tests ---

func TestParseAtKeyValues_Single(t *testing.T) {
	kv, keys := parseAtKeyValues("training @loss=0.452 done")
	if len(keys) != 1 || keys[0] != "loss" || kv["loss"] != "0.452" {
		t.Errorf("unexpected result: keys=%v kv=%v", keys, kv)
	}
}

func TestParseAtKeyValues_Multi(t *testing.T) {
	kv, keys := parseAtKeyValues("@epoch=5 @loss=0.312 @acc=0.97")
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}
	if kv["epoch"] != "5" || kv["loss"] != "0.312" || kv["acc"] != "0.97" {
		t.Errorf("unexpected kv: %v", kv)
	}
}

func TestParseAtKeyValues_NoMatch(t *testing.T) {
	kv, keys := parseAtKeyValues("plain log line without markers")
	if kv != nil || keys != nil {
		t.Errorf("expected nil results, got keys=%v kv=%v", keys, kv)
	}
}

func TestParseAtKeyValues_Integer(t *testing.T) {
	kv, keys := parseAtKeyValues("@step=100")
	if len(keys) != 1 || kv["step"] != "100" {
		t.Errorf("unexpected result: keys=%v kv=%v", keys, kv)
	}
}

func TestParseAtKeyValues_Negative(t *testing.T) {
	kv, keys := parseAtKeyValues("@delta=-0.05")
	if len(keys) != 1 || kv["delta"] != "-0.05" {
		t.Errorf("unexpected result: keys=%v kv=%v", keys, kv)
	}
}

func TestParseAtKeyValues_Scientific(t *testing.T) {
	kv, keys := parseAtKeyValues("@lr=1.5e-4")
	if len(keys) != 1 || kv["lr"] != "1.5e-4" {
		t.Errorf("unexpected result: keys=%v kv=%v", keys, kv)
	}
}

// --- @key=value protocol integration tests ---

func TestAtKeyProtocol_CallsCheckpoint(t *testing.T) {
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
		MetricKey:      "loss",
		CheckpointTool: "c4_run_checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("epoch 1/10\n@loss=0.452 @epoch=1\ntraining continues\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	select {
	case <-checkpointCalled:
		// success
	case <-time.After(300 * time.Millisecond):
		t.Error("expected c4_run_checkpoint to be called when @loss matched")
	}

	if capturedArgs != nil {
		if v, ok := capturedArgs["metric"].(string); !ok || v != "0.452" {
			t.Errorf("expected metric=0.452, got %v", capturedArgs["metric"])
		}
		if k, ok := capturedArgs["key"].(string); !ok || k != "loss" {
			t.Errorf("expected key=loss, got %v", capturedArgs["key"])
		}
	}
}

func TestAtKeyProtocol_FirstKeyFallback(t *testing.T) {
	// MetricKey is empty → first key on line triggers checkpoint.
	var capturedKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err == nil {
			if params, ok := req["params"].(map[string]any); ok {
				if args, ok := params["arguments"].(map[string]any); ok {
					capturedKey, _ = args["key"].(string)
				}
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	wrapper, err := NewExperimentWrapper(srv.URL, "exp-1", "run-1", &ExperimentProtocolConfig{
		MetricKey: "", // empty → use first key
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("@acc=0.97 @loss=0.1\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	if capturedKey != "acc" {
		t.Errorf("expected first key 'acc', got %q", capturedKey)
	}
}

func TestAtKeyProtocol_PassThrough(t *testing.T) {
	// Lines without @key=value must still appear in dst.
	wrapper, err := NewExperimentWrapper("http://localhost:1", "exp-1", "run-1", &ExperimentProtocolConfig{
		MetricKey: "loss",
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("training started\nepoch 1 done\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	if !strings.Contains(dst.String(), "training started") {
		t.Errorf("expected passthrough, got: %q", dst.String())
	}
}

func TestAtKeyProtocol_NonFatal(t *testing.T) {
	// MCP server returns 500 → WrapOutput must NOT return an error (non-fatal).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wrapper, err := NewExperimentWrapper(srv.URL, "exp-1", "run-1", &ExperimentProtocolConfig{
		MetricKey: "loss",
	})
	if err != nil {
		t.Fatal(err)
	}

	src := strings.NewReader("@loss=0.5\n")
	var dst bytes.Buffer
	if err := wrapper.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Errorf("expected non-fatal MCP error, got: %v", err)
	}
}

// --- retained non-EpochPattern tests ---

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

	src := strings.NewReader("line1\nline2\n@loss=0.3\n")
	var dst bytes.Buffer
	if err := w.WrapOutput(context.Background(), src, &dst); err != nil {
		t.Fatalf("WrapOutput error: %v", err)
	}

	out := dst.String()
	if !strings.Contains(out, "line1") || !strings.Contains(out, "@loss=0.3") {
		t.Errorf("unexpected output: %q", out)
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
		MetricKey: "loss",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3 lines each with @loss — expect 3 checkpoint calls.
	src := strings.NewReader("@loss=0.55\n@loss=0.50\n@loss=0.47\n")
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

	src := strings.NewReader("")
	var dst bytes.Buffer
	err = wrapper.WrapOutput(ctx, src, &dst)
	_ = err
}

func TestExperimentWrapper_CallMCP_MarshalError(t *testing.T) {
	wrapper, err := NewExperimentWrapper("http://localhost:1", "exp-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}

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
	wrapper, err := NewExperimentWrapper("http://localhost", "exp-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrapper.client == nil {
		t.Error("expected non-nil http.Client on ExperimentWrapper")
	}
}
