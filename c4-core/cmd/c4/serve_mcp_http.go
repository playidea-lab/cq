package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

// mcpHTTPComponent implements serve.Component.
// It exposes the shared mcpServer over Streamable HTTP (JSON-RPC 2.0),
// enabling remote Claude Code instances to connect as MCP clients via .mcp.json type:"url".
//
// Claude Code .mcp.json example:
//
//	{
//	  "mcpServers": {
//	    "remote-gpu": {
//	      "type": "url",
//	      "url": "http://192.168.1.100:4142/mcp",
//	      "headers": { "X-API-Key": "<key>" }
//	    }
//	  }
//	}
type mcpHTTPComponent struct {
	srv     *mcpServer
	cfg     config.ServeMCPHTTPConfig
	httpSrv *http.Server
	apiKey  string // resolved at Start()
}

// compile-time interface assertion
var _ serve.Component = (*mcpHTTPComponent)(nil)

func newMCPHTTPComponent(srv *mcpServer, cfg config.ServeMCPHTTPConfig) *mcpHTTPComponent {
	return &mcpHTTPComponent{srv: srv, cfg: cfg}
}

func (c *mcpHTTPComponent) Name() string { return "mcp-http" }

// Start resolves the API key, validates it is non-empty, and begins listening.
// Returns an error immediately if the key is empty or the port is unavailable.
func (c *mcpHTTPComponent) Start(_ context.Context) error {
	apiKey := c.resolveAPIKey()
	if apiKey == "" {
		return errors.New("mcp-http: api_key is required — set via 'cq secret set mcp_http.api_key <key>' or CQ_MCP_API_KEY env")
	}
	c.apiKey = apiKey

	addr := fmt.Sprintf("%s:%d", c.cfg.Bind, c.cfg.Port)
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", c.withAuth(c.handleMCP))

	c.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- c.httpSrv.ListenAndServe() }()

	// Wait briefly to detect immediate bind errors (e.g. port already in use).
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("mcp-http: listen %s: %w", addr, err)
		}
	case <-time.After(50 * time.Millisecond):
		// Server started successfully.
	}
	return nil
}

func (c *mcpHTTPComponent) Stop(ctx context.Context) error {
	if c.httpSrv != nil {
		return c.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (c *mcpHTTPComponent) Health() serve.ComponentHealth {
	if c.httpSrv == nil || c.apiKey == "" {
		return serve.ComponentHealth{Status: "error", Detail: "not started"}
	}
	return serve.ComponentHealth{
		Status: "ok",
		Detail: fmt.Sprintf("%s:%d", c.cfg.Bind, c.cfg.Port),
	}
}

// resolveAPIKey returns the API key using the following priority:
//  1. secrets.db entry "mcp_http.api_key"
//  2. CQ_MCP_API_KEY environment variable
//  3. config.yaml api_key field (dev/test fallback only)
func (c *mcpHTTPComponent) resolveAPIKey() string {
	if c.srv.secretStore != nil {
		if v, err := c.srv.secretStore.Get("mcp_http.api_key"); err == nil && v != "" {
			return v
		}
	}
	if v := os.Getenv("CQ_MCP_API_KEY"); v != "" {
		return v
	}
	return c.cfg.APIKey
}

// withAuth wraps a handler with API key authentication.
// Accepts the key in either X-API-Key header or Authorization: Bearer <key>.
// Uses constant-time comparison to prevent timing attacks.
func (c *mcpHTTPComponent) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if subtle.ConstantTimeCompare([]byte(key), []byte(c.apiKey)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// mcpHTTPMaxBodyBytes limits request body size to prevent memory exhaustion.
const mcpHTTPMaxBodyBytes = 1 << 20 // 1 MB

// handleMCP handles POST (JSON-RPC request) and GET (SSE keepalive).
func (c *mcpHTTPComponent) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.handleSSE(w, r)
	case http.MethodPost:
		c.handleJSONRPC(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSE streams keepalive comments at 15-second intervals.
// This satisfies the Streamable HTTP MCP spec GET endpoint requirement.
func (c *mcpHTTPComponent) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, canFlush := w.(http.Flusher)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n") //nolint:errcheck
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

// handleJSONRPC decodes a JSON-RPC 2.0 request, dispatches it via handleRequestWithCtx,
// and writes the response. Notifications (id == null) are accepted with 202.
func (c *mcpHTTPComponent) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, mcpHTTPMaxBodyBytes))
	if err != nil {
		writeMCPHTTPError(w, nil, -32700, "read error")
		return
	}

	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeMCPHTTPError(w, nil, -32700, "parse error")
		return
	}

	// Notification: id is absent/null — acknowledge without dispatching.
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Set write deadline to allow slow tools to complete (mirrors tool_socket.go).
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(65 * time.Second)) //nolint:errcheck

	callCtx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp := c.srv.handleRequestWithCtx(&req, callCtx)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// writeMCPHTTPError writes a JSON-RPC 2.0 error response.
// JSON-RPC errors always use HTTP 200 (the error is in the payload).
func writeMCPHTTPError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcpResponse{ //nolint:errcheck
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpError{Code: code, Message: msg},
	})
}

// registerMCPHTTPComponent registers the MCP HTTP component with the serve manager.
func registerMCPHTTPComponent(mgr *serve.Manager, cfg config.ServeMCPHTTPConfig, srv *mcpServer) {
	mgr.Register(newMCPHTTPComponent(srv, cfg))
}
