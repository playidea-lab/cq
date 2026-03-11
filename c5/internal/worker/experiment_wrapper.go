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
	"strconv"
	"strings"
	"time"
)

// ExperimentProtocolConfig defines the pattern matching and MCP integration
// configuration for an experiment run.
type ExperimentProtocolConfig struct {
	// EpochPattern is a regex with a named group "value" that matches metric output lines.
	// Example: `MPJPE:\s+(?P<value>[\d.]+)`
	EpochPattern string

	// CheckpointTool is the MCP tool name to call when a metric is matched.
	// Defaults to "c4_run_checkpoint".
	CheckpointTool string
}

// ExperimentWrapper wraps an io.Reader (typically stdout of an experiment process)
// and calls MCP checkpoint tools when configured metric patterns are matched.
type ExperimentWrapper struct {
	mcpURL   string
	expID    string
	runID    string
	protocol *ExperimentProtocolConfig
	epochRe  *regexp.Regexp
	client   *http.Client
}

// NewExperimentWrapper creates an ExperimentWrapper. Returns an error if the
// epoch pattern is set but cannot be compiled.
func NewExperimentWrapper(mcpURL, expID, runID string, protocol *ExperimentProtocolConfig) (*ExperimentWrapper, error) {
	w := &ExperimentWrapper{
		mcpURL:   mcpURL,
		expID:    expID,
		runID:    runID,
		protocol: protocol,
		client:   &http.Client{Timeout: 10 * time.Second},
	}

	if protocol != nil && protocol.EpochPattern != "" {
		re, err := regexp.Compile(protocol.EpochPattern)
		if err != nil {
			return nil, fmt.Errorf("compile epoch pattern: %w", err)
		}
		w.epochRe = re
	}

	return w, nil
}

// WrapOutput reads from src line by line, writes each line to dst, and
// triggers MCP checkpoint calls when a metric pattern is matched.
// It returns when src is exhausted or ctx is cancelled.
func (w *ExperimentWrapper) WrapOutput(ctx context.Context, src io.Reader, dst io.Writer) error {
	scanner := bufio.NewScanner(src)
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

		if w.epochRe != nil {
			if value, ok := w.extractMetric(line); ok {
				tool := "c4_run_checkpoint"
				if w.protocol.CheckpointTool != "" {
					tool = w.protocol.CheckpointTool
				}
				metric, err := strconv.ParseFloat(value, 64)
				if err != nil {
					log.Printf("experiment-wrapper: cannot parse metric %q: %v", value, err)
				} else if err := w.callMCP(ctx, tool, map[string]any{
					"exp_id": w.expID,
					"run_id": w.runID,
					"metric": metric,
				}); err != nil {
					log.Printf("experiment-wrapper: checkpoint call failed: %v", err)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan output: %w", err)
	}
	return nil
}

// extractMetric returns the value captured by the named group "value" in the epoch pattern.
func (w *ExperimentWrapper) extractMetric(line string) (string, bool) {
	match := w.epochRe.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	idx := w.epochRe.SubexpIndex("value")
	if idx < 0 || idx >= len(match) {
		return "", false
	}
	v := strings.TrimSpace(match[idx])
	return v, v != ""
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
