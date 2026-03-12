// Package worker provides utilities for wrapping experiment process output
// and integrating with the C4 MCP checkpoint protocol.
package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"
)

// atKeyRe matches @key=value tokens in a line, where value is a number.
var atKeyRe = regexp.MustCompile(`@(\w+)=([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)`)

// ExperimentProtocolConfig defines the @key=value protocol and MCP integration
// configuration for an experiment run.
type ExperimentProtocolConfig struct {
	// MetricKey is the key whose value triggers a checkpoint call.
	// If empty, the first @key=value match on the line is used.
	MetricKey string

	// EpochKey is the key whose value is sent as "epoch" in checkpoint args.
	EpochKey string

	// CheckpointTool is the MCP tool name to call when a metric is matched.
	// Defaults to "c4_run_checkpoint".
	CheckpointTool string
}

// ExperimentWrapper wraps an io.Reader (typically stdout of an experiment process)
// and calls MCP checkpoint tools when @key=value tokens are found in output.
type ExperimentWrapper struct {
	mcpURL               string
	expID                string
	runID                string
	protocol             *ExperimentProtocolConfig
	client               *http.Client
	consecutiveFailures  int
}

// NewExperimentWrapper creates an ExperimentWrapper.
func NewExperimentWrapper(mcpURL, expID, runID string, protocol *ExperimentProtocolConfig) (*ExperimentWrapper, error) {
	return &ExperimentWrapper{
		mcpURL:   mcpURL,
		expID:    expID,
		runID:    runID,
		protocol: protocol,
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// parseAtKeyValues extracts all @key=value pairs from a line.
// Returns a map of key→value strings and a slice preserving insertion order.
func parseAtKeyValues(line string) (map[string]string, []string) {
	matches := atKeyRe.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	kv := make(map[string]string, len(matches))
	keys := make([]string, 0, len(matches))
	for _, m := range matches {
		key, val := m[1], m[2]
		if _, seen := kv[key]; !seen {
			keys = append(keys, key)
		}
		kv[key] = val
	}
	return kv, keys
}

// WrapOutput reads from src line by line, writes each line to dst, and
// triggers MCP checkpoint calls when @key=value tokens are found.
// It returns when src is exhausted or ctx is cancelled.
func (w *ExperimentWrapper) WrapOutput(ctx context.Context, src io.Reader, dst io.Writer) error {
	scanner := bufio.NewScanner(src)
	// Increase buffer to 1 MiB to handle long ML training lines (e.g. serialised tensors).
	scanner.Buffer(make([]byte, 256*1024), 1*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if _, err := fmt.Fprintln(dst, line); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		if w.protocol == nil {
			continue
		}

		kv, keys := parseAtKeyValues(line)
		if len(keys) == 0 {
			continue
		}

		// Determine the metric key to use for checkpoint trigger.
		triggerKey := w.protocol.MetricKey
		if triggerKey == "" {
			triggerKey = keys[0]
		}
		metricVal, ok := kv[triggerKey]
		if !ok {
			continue
		}

		tool := "c4_run_checkpoint"
		if w.protocol.CheckpointTool != "" {
			tool = w.protocol.CheckpointTool
		}

		args := map[string]any{
			"exp_id": w.expID,
			"run_id": w.runID,
			"metric": metricVal,
			"key":    triggerKey,
		}
		if w.protocol.EpochKey != "" {
			if epochVal, ok := kv[w.protocol.EpochKey]; ok {
				args["epoch"] = epochVal
			}
		}

		if err := w.callMCP(ctx, tool, args); err != nil {
			w.consecutiveFailures++
			if w.consecutiveFailures == 1 {
				log.Printf("experiment-wrapper: checkpoint call failed: %v", err)
			}
			if w.consecutiveFailures >= 3 {
				log.Printf("experiment-wrapper: 3 consecutive failures, disabling checkpoints for this job")
				w.protocol = nil
			}
		} else {
			w.consecutiveFailures = 0
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan output: %w", err)
	}
	return nil
}

// callMCP sends a JSON-RPC-style POST request to the MCP server invoking the
// given tool with the provided arguments.
func (w *ExperimentWrapper) callMCP(ctx context.Context, tool string, args map[string]any) error {
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params": map[string]any{
			"name":      tool,
			"arguments": args,
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.mcpURL, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("call MCP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("MCP returned %d: %s", resp.StatusCode, body)
	}

	return nil
}
