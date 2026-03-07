package api

// MCP Streamable HTTP transport (JSON-RPC 2.0)
//
// Implements the Model Context Protocol server-side transport so that Claude Code
// and other MCP clients can add C5 Hub as an MCP server:
//
//	.mcp.json:
//	  {"mcpServers": {"c5": {"type":"url","url":"https://hub/v1/mcp","headers":{"X-API-Key":"..."}}}
//
// Supported methods:
//   - initialize                → server info
//   - tools/list               → capabilities + built-in hub tools
//   - tools/call               → invoke capability (async job) or built-in
//   - notifications/initialized (client→server, no-op)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// mcpRequest is a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // number or string; nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse is a JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleMCP serves POST /v1/mcp (JSON-RPC 2.0 over HTTP).
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// SSE channel for server-initiated notifications (optional, minimal).
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		// Keep the SSE channel alive; we don't send proactive notifications yet.
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}

	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req mcpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMCPError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	if req.JSONRPC != "2.0" {
		writeMCPError(w, req.ID, -32600, "invalid JSON-RPC version")
		return
	}

	projectID := projectIDFromContext(r)

	switch req.Method {
	case "initialize":
		s.mcpInitialize(w, req)
	case "notifications/initialized":
		// Client notification — no response needed (notification has no id)
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		s.mcpToolsList(w, req, projectID)
	case "tools/call":
		s.mcpToolsCall(w, r, req, projectID)
	case "ping":
		writeMCPResult(w, req.ID, map[string]any{})
	default:
		writeMCPError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) mcpInitialize(w http.ResponseWriter, req mcpRequest) {
	writeMCPResult(w, req.ID, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "c5-hub",
			"version": s.version,
		},
	})
}

// builtinTools are always available regardless of worker capabilities.
var builtinTools = []map[string]any{
	{
		"name":        "hub_queue_stats",
		"description": "Get the current job queue statistics (queued, running, succeeded, failed counts)",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "hub_list_workers",
		"description": "List all connected workers with their GPU and status information",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Filter by project ID (optional)"},
			},
		},
	},
	{
		"name":        "hub_list_jobs",
		"description": "List recent jobs from the queue",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string", "description": "Filter by status: QUEUED, RUNNING, SUCCEEDED, FAILED"},
				"limit":  map[string]any{"type": "integer", "description": "Max results (default 20)"},
			},
		},
	},
	{
		"name":        "hub_job_status",
		"description": "Get the status and result of a specific job",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"job_id"},
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
		},
	},
	{
		"name":        "hub_cancel_job",
		"description": "Cancel a queued or running job",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"job_id"},
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID to cancel"},
			},
		},
	},
}

func (s *Server) mcpToolsList(w http.ResponseWriter, req mcpRequest, projectID string) {
	caps, err := s.store.ListCapabilities(projectID)
	if err != nil {
		writeMCPError(w, req.ID, -32000, "store error: "+err.Error())
		return
	}

	tools := make([]map[string]any, 0, len(builtinTools)+len(caps))
	tools = append(tools, builtinTools...)

	for _, c := range caps {
		schema := c.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"inputSchema": schema,
		})
	}

	writeMCPResult(w, req.ID, map[string]any{"tools": tools})
}

func (s *Server) mcpToolsCall(w http.ResponseWriter, r *http.Request, req mcpRequest, projectID string) {
	if len(req.Params) == 0 {
		writeMCPError(w, req.ID, -32602, "params is required")
		return
	}
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeMCPError(w, req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if params.Name == "" {
		writeMCPError(w, req.ID, -32602, "name is required")
		return
	}

	// Built-in tools
	switch params.Name {
	case "hub_queue_stats":
		stats, err := s.store.GetQueueStats()
		if err != nil {
			writeMCPError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPTextResult(w, req.ID, stats)
		return

	case "hub_list_workers":
		pid := projectID
		if v, ok := params.Arguments["project_id"].(string); ok && v != "" && projectID == "" {
			pid = v
		}
		workers, err := s.store.ListWorkers(pid)
		if err != nil {
			writeMCPError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPTextResult(w, req.ID, workers)
		return

	case "hub_list_jobs":
		status := ""
		if v, ok := params.Arguments["status"].(string); ok {
			status = v
		}
		limit := 20
		if v, ok := params.Arguments["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		jobs, err := s.store.ListJobs(status, projectID, limit, 0)
		if err != nil {
			writeMCPError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPTextResult(w, req.ID, jobs)
		return

	case "hub_job_status":
		jobID, _ := params.Arguments["job_id"].(string)
		if jobID == "" {
			writeMCPError(w, req.ID, -32602, "job_id is required")
			return
		}
		job, err := s.store.GetJob(jobID)
		if err != nil {
			writeMCPError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPTextResult(w, req.ID, job)
		return

	case "hub_cancel_job":
		jobID, _ := params.Arguments["job_id"].(string)
		if jobID == "" {
			writeMCPError(w, req.ID, -32602, "job_id is required")
			return
		}
		if err := s.store.UpdateJobStatus(jobID, model.StatusCancelled, ""); err != nil {
			writeMCPError(w, req.ID, -32000, err.Error())
			return
		}
		s.store.DeleteLease(jobID)
		if s.eventPub.IsEnabled() {
			s.eventPub.Publish("hub.job.cancelled", "c5", map[string]any{"job_id": jobID}) //nolint:errcheck
		}
		writeMCPTextResult(w, req.ID, map[string]any{"job_id": jobID, "status": "CANCELLED"})
		return
	}

	// Capability invocation: create a job and poll for result.
	regs, err := s.store.FindCapability(params.Name, projectID)
	if err != nil {
		writeMCPError(w, req.ID, -32000, err.Error())
		return
	}
	if len(regs) == 0 {
		writeMCPError(w, req.ID, -32601, "tool not found: "+params.Name)
		return
	}

	command := regs[0].Command
	if command == "" {
		command = "__capability__:" + params.Name
	}

	jobReq := &model.JobSubmitRequest{
		Name:       params.Name,
		Workdir:    ".",
		Command:    command,
		ProjectID:  projectID,
		Capability: params.Name,
		Params:     params.Arguments,
	}
	job, err := s.store.CreateJob(jobReq)
	if err != nil {
		writeMCPError(w, req.ID, -32000, "create job: "+err.Error())
		return
	}

	// Register completion channel BEFORE notifying workers so that
	// handleWorkerComplete (called from handleJobComplete in jobs.go) cannot
	// fire and miss the channel in the window between CreateJob and Store.
	// Buffered 1 so handleWorkerComplete never blocks even if we haven't
	// entered the select yet.
	completionCh := make(chan struct{}, 1)
	s.completionHub.Store(job.ID, completionCh)

	s.notifyJobAvailable()

	// Compensate for the race where an in-process worker completed the job
	// between CreateJob and the Store above: if the job is already terminal,
	// signal the channel now so waitForCompletion returns immediately.
	if existing, getErr := s.store.GetJob(job.ID); getErr == nil && existing.Status.IsTerminal() {
		s.handleWorkerComplete(job.ID)
	}

	// Wait for completion (max 5 minutes for MCP sync tool calls).
	// Respects client disconnect via r.Context().Done().
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if err := s.waitForCompletion(ctx, job.ID); err != nil {
		// Timeout: write error to caller. Client disconnect: silently return
		// (the job continues running; client disconnected intentionally).
		if ctx.Err() == context.DeadlineExceeded {
			writeMCPError(w, req.ID, -32000, fmt.Sprintf("job %s timed out waiting for completion", job.ID))
		}
		return
	}

	j, err := s.store.GetJob(job.ID)
	if err != nil {
		writeMCPError(w, req.ID, -32000, "get job: "+err.Error())
		return
	}
	if j.Status == model.StatusSucceeded {
		result := map[string]any{"job_id": j.ID, "status": string(j.Status)}
		if j.Result != nil {
			result["result"] = j.Result
		}
		writeMCPTextResult(w, req.ID, result)
	} else {
		writeMCPError(w, req.ID, -32000, fmt.Sprintf("job %s %s", j.ID, j.Status))
	}
}

// handleWorkerComplete signals any waiting mcpToolsCall that a job has reached
// a terminal state. It is called from handleJobComplete in jobs.go.
// Idempotent: a second call for the same jobID is a no-op (channel already deleted).
func (s *Server) handleWorkerComplete(jobID string) {
	if v, loaded := s.completionHub.LoadAndDelete(jobID); loaded {
		close(v.(chan struct{}))
	}
}

// waitForCompletion blocks until the job's completion channel is closed or ctx
// is cancelled. The caller is responsible for providing a context with timeout.
func (s *Server) waitForCompletion(ctx context.Context, jobID string) error {
	v, ok := s.completionHub.Load(jobID)
	if !ok {
		// Channel was already consumed (very fast completion path).
		return nil
	}
	ch := v.(chan struct{})
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		s.completionHub.Delete(jobID)
		return ctx.Err()
	}
}

func writeMCPResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeMCPTextResult(w http.ResponseWriter, id any, data any) {
	b, _ := json.Marshal(data)
	writeMCPResult(w, id, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(b)},
		},
	})
}

func writeMCPError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpError{Code: code, Message: msg},
	})
}
