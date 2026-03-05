package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/serve"
)

// ── Protocol ──────────────────────────────────────────────────────────────────

// sockRequest is the request envelope for the tool socket.
type sockRequest struct {
	Op   string         `json:"op"`             // "list" | "schema" | "call"
	Tool string         `json:"tool,omitempty"` // required for schema, call
	Args map[string]any `json:"args,omitempty"` // required for call
}

// sockResponse is the response envelope for the tool socket.
type sockResponse struct {
	Tools  []mcp.ToolSchema `json:"tools,omitempty"`
	Schema *mcp.ToolSchema  `json:"schema,omitempty"`
	Result any              `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// sockMaxResponseBytes caps the response size read by the client (4 MB).
const sockMaxResponseBytes = 4 << 20

// ── Server-side component ─────────────────────────────────────────────────────

// toolSocketComponent implements serve.Component.
// It initialises an MCP server once on Start and exposes it over a Unix socket,
// letting "cq tool" skip the cold-start cost when "cq serve" is running.
type toolSocketComponent struct {
	sockPath string
	srv      *mcpServer
	ln       net.Listener
}

// compile-time interface assertion
var _ serve.Component = (*toolSocketComponent)(nil)

func newToolSocketComponent(sockPath string) *toolSocketComponent {
	return &toolSocketComponent{sockPath: sockPath}
}

func (t *toolSocketComponent) Name() string { return "tool-socket" }

func (t *toolSocketComponent) Start(ctx context.Context) error {
	// Ensure the parent directory exists (e.g. .c4/) before listening.
	if err := os.MkdirAll(filepath.Dir(t.sockPath), 0700); err != nil {
		return fmt.Errorf("tool socket: mkdir %s: %w", filepath.Dir(t.sockPath), err)
	}
	// Remove any stale socket left by a previous run.
	os.Remove(t.sockPath) //nolint:errcheck

	ln, err := net.Listen("unix", t.sockPath)
	if err != nil {
		return fmt.Errorf("tool socket: listen %s: %w", t.sockPath, err)
	}
	t.ln = ln

	srv, err := newMCPServer()
	if err != nil {
		ln.Close()
		return fmt.Errorf("tool socket: MCP init: %w", err)
	}
	t.srv = srv

	go t.accept(ctx, ln)
	return nil
}

func (t *toolSocketComponent) Stop(_ context.Context) error {
	if t.ln != nil {
		t.ln.Close()
		t.ln = nil // #6: Health() must return "error" after Stop
	}
	if t.srv != nil {
		t.srv.shutdown()
	}
	os.Remove(t.sockPath) //nolint:errcheck
	return nil
}

func (t *toolSocketComponent) Health() serve.ComponentHealth {
	if t.ln == nil {
		return serve.ComponentHealth{Status: "error", Detail: "not started"}
	}
	return serve.ComponentHealth{Status: "ok", Detail: t.sockPath}
}

// accept runs in a goroutine. ln is passed directly (not read from t.ln) to
// avoid a race with Stop() setting t.ln = nil before this goroutine starts.
func (t *toolSocketComponent) accept(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				// Normal shutdown — stay quiet.
			default:
				// Unexpected error after Stop() races with ctx cancel.
				fmt.Fprintf(os.Stderr, "tool-socket: accept error: %v\n", err)
			}
			return
		}
		go t.handle(ctx, conn)
	}
}

// handle decodes one request, dispatches it, and encodes the response.
// Read deadline is short (5s) so a slow sender can't tie up the connection;
// write deadline is also short (5s). Tool execution time is governed by the
// ctx timeout inside dispatch, not by the connection deadline.
func (t *toolSocketComponent) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Short I/O deadlines — independent of tool execution time.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))  //nolint:errcheck
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req sockRequest
	if err := dec.Decode(&req); err != nil {
		enc.Encode(sockResponse{Error: "bad request: " + err.Error()}) //nolint:errcheck
		return
	}

	// Reset write deadline before potentially long dispatch.
	conn.SetWriteDeadline(time.Now().Add(65 * time.Second)) //nolint:errcheck
	enc.Encode(t.dispatch(ctx, req))                        //nolint:errcheck
}

// dispatch executes the requested operation.
// ctx is the serve process context — SIGTERM propagates to tool calls.
func (t *toolSocketComponent) dispatch(ctx context.Context, req sockRequest) sockResponse {
	switch req.Op {
	case "list":
		return sockResponse{Tools: t.srv.registry.ListTools()}

	case "schema":
		schema, ok := t.srv.registry.GetToolSchema(req.Tool)
		if !ok {
			return sockResponse{Error: "unknown tool: " + req.Tool}
		}
		return sockResponse{Schema: &schema}

	case "call":
		argsJSON, err := json.Marshal(req.Args)
		if err != nil {
			return sockResponse{Error: "bad args: " + err.Error()}
		}
		// Use serve ctx so SIGTERM cancels in-flight tool calls.
		callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		result, err := t.srv.registry.CallWithContext(callCtx, req.Tool, argsJSON)
		if err != nil {
			return sockResponse{Error: err.Error()}
		}
		return sockResponse{Result: result}

	default:
		return sockResponse{Error: "unknown op: " + req.Op}
	}
}

// ── Client helpers (used by tool.go) ─────────────────────────────────────────

// toolSockPath returns the conventional socket path for the current project.
func toolSockPath() string {
	return filepath.Join(projectDir, ".c4", "tool.sock")
}

// callSocket sends one request to the tool socket and returns the response.
// Returns an error if the socket is unavailable, the transport fails, or
// the server returned a non-empty Error field.
func callSocket(sockPath string, req sockRequest) (sockResponse, error) {
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return sockResponse{}, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return sockResponse{}, fmt.Errorf("socket send: %w", err)
	}
	var resp sockResponse
	// LimitReader prevents unbounded memory use on large responses (e.g. full tool list).
	if err := json.NewDecoder(io.LimitReader(conn, sockMaxResponseBytes)).Decode(&resp); err != nil {
		return sockResponse{}, fmt.Errorf("socket recv: %w", err)
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// registerToolSocketComponent registers the tool socket with the serve manager.
func registerToolSocketComponent(mgr *serve.Manager) {
	mgr.Register(newToolSocketComponent(toolSockPath()))
}
