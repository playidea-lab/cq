// Package relayhandler provides MCP tools for interacting with remote workers
// through the relay server. It enables local agents to discover connected workers
// and call any MCP tool on a remote worker transparently.
package relayhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

const (
	healthTimeout  = 10 * time.Second
	relayTimeout   = 30 * time.Second
	maxResponseLen = 1 << 20 // 1 MiB
)

// HubListerResult holds a single hub worker's info for the unified view.
type HubListerResult struct {
	ID       string
	Hostname string
	Status   string
	Tags     []string
	GPUModel string
}

// HubWorkerLister lists hub-registered workers. Implemented via adapter in mcp_init.go.
type HubWorkerLister interface {
	ListWorkersBasic() ([]HubListerResult, int, error) // workers, pending job count, error
}

// Deps holds dependencies for relay handler tools.
type Deps struct {
	RelayURL  string            // e.g. "wss://cq-relay.fly.dev" (converted to https for HTTP calls)
	AnonKey   string            // Supabase anon key for apikey header
	TokenFunc func() string     // returns fresh JWT
	HubLister HubWorkerLister   // optional: merges hub worker status into cq_workers response
}

// httpBase converts a WebSocket relay URL to an HTTPS base URL.
func (d *Deps) httpBase() string {
	base := d.RelayURL
	base = strings.Replace(base, "wss://", "https://", 1)
	base = strings.Replace(base, "ws://", "http://", 1)
	return strings.TrimRight(base, "/")
}

// Register registers cq_workers and cq_relay_call MCP tools.
func Register(reg *mcp.Registry, deps *Deps) {
	registerWorkers(reg, deps)
	registerRelayCall(reg, deps)
}

// registerWorkers registers the cq_workers tool.
func registerWorkers(reg *mcp.Registry, deps *Deps) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_workers",
		Description: "List remote workers connected to the relay server. Returns worker IDs and connection status. Use this to discover available workers before calling cq_relay_call.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWorkers(deps)
	})
}

// registerRelayCall registers the cq_relay_call tool.
func registerRelayCall(reg *mcp.Registry, deps *Deps) {
	reg.Register(mcp.ToolSchema{
		Name: "cq_relay_call",
		Description: `Call any MCP tool on a remote worker via the relay server.
Use cq_workers first to discover available workers.
Supports all worker tools: cq_read_file, cq_create_text_file, cq_find_file, cq_list_dir, cq_execute, cq_search_for_pattern, cq_replace_content, etc.
For large files (>1MB), use 'cq drive dataset' instead. For long-running commands (>30s), use 'cq hub submit'.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Target worker ID (from cq_workers output)",
				},
				"tool": map[string]any{
					"type":        "string",
					"description": "MCP tool name to call on the remote worker (e.g. cq_read_file, cq_execute)",
				},
				"args": map[string]any{
					"type":        "object",
					"description": "Arguments to pass to the remote tool",
				},
			},
			"required": []string{"worker_id", "tool"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleRelayCall(deps, raw)
	})
}

func handleWorkers(deps *Deps) (any, error) {
	if deps.RelayURL == "" {
		return map[string]any{
			"error": "relay not configured",
			"hint":  "Run 'cq auth login' to configure relay, then 'cq serve' to connect.",
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", deps.httpBase()+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{
			"error":   "relay unreachable",
			"details": err.Error(),
			"hint":    "Check if the relay server is running at " + deps.httpBase(),
		}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var health struct {
		Status      string   `json:"status"`
		Workers     int      `json:"workers"`
		WorkerNames []string `json:"worker_names"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("parse health response: %w", err)
	}

	// Build worker map from relay (connected workers).
	workerMap := make(map[string]map[string]any, len(health.WorkerNames))
	for _, name := range health.WorkerNames {
		workerMap[name] = map[string]any{
			"id":    name,
			"relay": "connected",
			"jobs":  nil, // unknown until hub info merged
		}
	}

	// Merge hub worker info if available.
	var pendingJobs int
	if deps.HubLister != nil {
		hubWorkers, pending, err := deps.HubLister.ListWorkersBasic()
		if err == nil {
			pendingJobs = pending
			for _, hw := range hubWorkers {
				key := hw.Hostname
				if key == "" {
					key = hw.ID
				}
				if w, ok := workerMap[key]; ok {
					// Worker is both relay-connected and hub-registered.
					w["jobs"] = hw.Status
					if len(hw.Tags) > 0 {
						w["tags"] = hw.Tags
					}
					if hw.GPUModel != "" {
						w["gpu"] = hw.GPUModel
					}
				} else {
					// Hub-registered but not relay-connected (offline relay).
					workerMap[key] = map[string]any{
						"id":    key,
						"relay": "disconnected",
						"jobs":  hw.Status,
						"tags":  hw.Tags,
					}
				}
			}
		}
	}

	workers := make([]map[string]any, 0, len(workerMap))
	for _, w := range workerMap {
		workers = append(workers, w)
	}

	result := map[string]any{
		"relay":   deps.httpBase(),
		"status":  health.Status,
		"workers": workers,
		"count":   len(workers),
	}
	if pendingJobs > 0 {
		result["pending_jobs"] = pendingJobs
	}
	return result, nil
}

func handleRelayCall(deps *Deps, raw json.RawMessage) (any, error) {
	if deps.RelayURL == "" {
		return map[string]any{
			"error": "relay not configured",
			"hint":  "Run 'cq auth login' to configure relay, then 'cq serve' to connect.",
		}, nil
	}

	var params struct {
		WorkerID string          `json:"worker_id"`
		Tool     string          `json:"tool"`
		Args     json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if params.WorkerID == "" || params.Tool == "" {
		return nil, fmt.Errorf("worker_id and tool are required")
	}
	if params.Args == nil {
		params.Args = json.RawMessage(`{}`)
	}

	// Build JSON-RPC request for the remote worker.
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      params.Tool,
			"arguments": json.RawMessage(params.Args),
		},
	}
	reqBody, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	// POST to relay.
	url := fmt.Sprintf("%s/w/%s/mcp", deps.httpBase(), params.WorkerID)
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if deps.TokenFunc != nil {
		httpReq.Header.Set("Authorization", "Bearer "+deps.TokenFunc())
	}
	if deps.AnonKey != "" {
		httpReq.Header.Set("apikey", deps.AnonKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return map[string]any{
				"error": fmt.Sprintf("timeout calling %s on worker %s (30s limit)", params.Tool, params.WorkerID),
				"hint":  "For long-running commands, use 'cq hub submit' instead.",
			}, nil
		}
		return nil, fmt.Errorf("relay request: %w", err)
	}
	defer resp.Body.Close()

	// Worker offline.
	if resp.StatusCode == http.StatusServiceUnavailable {
		return map[string]any{
			"error":     fmt.Sprintf("worker %q is offline", params.WorkerID),
			"hint":      "Use cq_workers to check available workers. Ensure 'cq serve' is running on the target machine.",
			"worker_id": params.WorkerID,
		}, nil
	}

	// Timeout from relay side.
	if resp.StatusCode == http.StatusGatewayTimeout {
		return map[string]any{
			"error": fmt.Sprintf("timeout calling %s on worker %s", params.Tool, params.WorkerID),
			"hint":  "For long-running commands, use 'cq hub submit' instead.",
		}, nil
	}

	// Read response with size limit.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseLen+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(body) > maxResponseLen {
		return map[string]any{
			"error":     "response exceeds 1MB limit",
			"hint":      "For large files, use 'cq drive dataset upload' on the worker, then 'cq drive dataset pull' locally.",
			"truncated": true,
			"size":      len(body),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return map[string]any{
			"error":       fmt.Sprintf("relay returned status %d", resp.StatusCode),
			"response":    string(body),
			"worker_id":   params.WorkerID,
			"tool":        params.Tool,
		}, nil
	}

	// Parse JSON-RPC response from worker.
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		// Not valid JSON-RPC — return raw body.
		return map[string]any{
			"raw_response": string(body),
			"worker_id":    params.WorkerID,
			"tool":         params.Tool,
		}, nil
	}

	if rpcResp.Error != nil && string(rpcResp.Error) != "null" {
		return map[string]any{
			"error":     string(rpcResp.Error),
			"worker_id": params.WorkerID,
			"tool":      params.Tool,
		}, nil
	}

	// Unwrap the result into a generic value.
	var result any
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		return map[string]any{
			"raw_result": string(rpcResp.Result),
			"worker_id":  params.WorkerID,
			"tool":       params.Tool,
		}, nil
	}
	return result, nil
}
