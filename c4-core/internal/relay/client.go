// Package relay provides a WebSocket relay client for forwarding MCP requests
// from a relay server to a local MCP handler and returning responses.
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// tcpKeepAliveInterval is the TCP-level keepalive period.
// WSL2's NAT drops idle conntrack entries after ~2-5 minutes.
// 30s TCP keepalive ensures the NAT sees activity before timeout.
const tcpKeepAliveInterval = 30 * time.Second

// MCPHandler processes a JSON-RPC MCP request and returns a JSON-RPC response.
type MCPHandler func(ctx context.Context, request json.RawMessage) (json.RawMessage, error)

// relayEnvelope is the wire format for messages between relay server and worker.
type relayEnvelope struct {
	RelayID string          `json:"relay_id"`
	Body    json.RawMessage `json:"body"`
}

// RelayClient connects to a relay server via WebSocket, receives MCP requests,
// delegates them to an MCPHandler, and sends back responses.
type RelayClient struct {
	relayURL   string
	workerID   string
	tokenFunc  func() string
	mcpHandler MCPHandler

	mu          sync.Mutex
	conn        net.Conn
	done        chan struct{}
	closeOnce   sync.Once
	connected   bool
}

// NewRelayClient creates a RelayClient.
// relayURL is the WebSocket URL of the relay server (ws:// or wss://).
// workerID identifies this worker to the relay server.
// tokenFunc is called to obtain a fresh auth token for each connection attempt.
// handler processes incoming MCP requests.
func NewRelayClient(relayURL, workerID string, tokenFunc func() string, handler MCPHandler) *RelayClient {
	return &RelayClient{
		relayURL:   relayURL,
		workerID:   workerID,
		tokenFunc:  tokenFunc,
		mcpHandler: handler,
		done:       make(chan struct{}),
	}
}

// Connect dials the relay server and starts a background reconnect loop.
// It returns after the first successful connection or an error if the initial
// dial fails (ctx cancelled, invalid URL, etc.).
// Use reconnectLoop semantics: call Connect once, it manages its own lifecycle.
func (c *RelayClient) Connect(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	go c.readLoop(ctx, conn)
	go c.reconnectLoop(ctx)
	go c.pingLoop(ctx)
	return nil
}

// Close shuts down the relay client, closing the WebSocket connection and
// stopping the reconnect loop.
func (c *RelayClient) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.mu.Lock()
		conn := c.conn
		c.connected = false
		c.mu.Unlock()
		if conn != nil {
			conn.Close()
		}
	})
}

// IsConnected reports whether the client currently has an active connection.
func (c *RelayClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// dial builds the connection URL and dials the relay server.
func (c *RelayClient) dial(ctx context.Context) (net.Conn, error) {
	u, err := url.Parse(c.relayURL)
	if err != nil {
		return nil, fmt.Errorf("relay: invalid URL %q: %w", c.relayURL, err)
	}

	// Ensure /connect path is present
	if u.Path == "" || u.Path == "/" {
		u.Path = "/connect"
	}

	q := u.Query()
	q.Set("worker_id", c.workerID)
	if c.tokenFunc != nil {
		q.Set("token", c.tokenFunc())
	}
	u.RawQuery = q.Encode()

	dialer := ws.Dialer{Timeout: 10 * time.Second}
	conn, _, _, err := dialer.Dial(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("relay: dial %s: %w", c.relayURL, err)
	}

	// Enable TCP-level keepalive to survive NAT conntrack timeouts (WSL2, cloud NAT, etc.).
	// Application-level WebSocket pings alone are insufficient when the NAT silently drops
	// the TCP connection — the ping never reaches the peer.
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(tcpKeepAliveInterval)
	}

	return conn, nil
}

// readLoop reads messages from conn, dispatches them to the handler, and writes
// responses back. It exits when conn is closed or an error occurs.
//
// Uses wsutil.ReadServerData which correctly handles the WebSocket state machine
// (fragmentation, masking, control frames). Server Pings are kept alive by
// pingLoop sending unsolicited Pong frames — the relay server's PongHandler
// resets ReadDeadline on any Pong received.
func (c *RelayClient) readLoop(ctx context.Context, conn net.Conn) {
	defer func() {
		conn.Close()
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
			c.connected = false
		}
		c.mu.Unlock()
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		// Read deadline: server sends pings every 30s. If we get nothing in 90s,
		// the connection is dead (NAT rebind, silent TCP drop, etc.).
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		data, op, err := wsutil.ReadServerData(conn)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Connection error — reconnectLoop will re-dial.
			return
		}

		if op == ws.OpClose {
			return
		}
		if op != ws.OpText {
			continue
		}

		go c.handleMessage(ctx, conn, data)
	}
}

// handleMessage parses a relay envelope, calls the MCPHandler, and sends the
// response envelope back on conn.
func (c *RelayClient) handleMessage(ctx context.Context, conn net.Conn, data []byte) {
	var env relayEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		// Malformed envelope — drop silently.
		return
	}
	if env.RelayID == "" {
		return
	}

	var respBody json.RawMessage
	if c.mcpHandler != nil {
		result, err := c.mcpHandler(ctx, env.Body)
		if err != nil {
			// Encode error as JSON-RPC error response.
			errResp := map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32000,
					"message": err.Error(),
				},
			}
			b, _ := json.Marshal(errResp)
			respBody = b
		} else {
			respBody = result
		}
	}

	resp := relayEnvelope{
		RelayID: env.RelayID,
		Body:    respBody,
	}
	respData, err := json.Marshal(resp)
	if err != nil {
		return
	}

	c.mu.Lock()
	writeConn := c.conn
	c.mu.Unlock()

	if writeConn == nil {
		return
	}
	// Use a mutex around writes to avoid concurrent write races.
	c.mu.Lock()
	writeErr := wsutil.WriteClientText(writeConn, respData)
	c.mu.Unlock()
	_ = writeErr
}

// reconnectLoop monitors the connection and re-dials with exponential backoff
// when the connection is lost. It stops when ctx is cancelled or Close is called.
func (c *RelayClient) reconnectLoop(ctx context.Context) {
	const maxBackoff = 60 * time.Second
	backoff := 1 * time.Second

	for {
		// Wait until we lose the connection.
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-time.After(500 * time.Millisecond):
		}

		c.mu.Lock()
		alive := c.connected
		c.mu.Unlock()

		if alive {
			backoff = 1 * time.Second
			continue
		}

		// Connection lost — attempt to reconnect.
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-time.After(backoff):
		}

		fmt.Printf("relay: reconnecting (backoff=%s)…\n", backoff)

		// tokenFunc() triggers TokenProvider.Token() which auto-refreshes
		// if within 5 min of expiry — no extra action needed here.

		conn, err := c.dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Printf("relay: reconnect failed: %v\n", err)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		c.mu.Lock()
		c.conn = conn
		c.connected = true
		c.mu.Unlock()

		backoff = 1 * time.Second
		go c.readLoop(ctx, conn)
	}
}

// pingLoop sends WebSocket pings AND unsolicited pongs to keep the connection alive.
//
// Why both Ping and Pong?
// - Ping: keeps intermediate proxies (Fly.io, NAT) from closing idle connections.
// - Pong: the relay server (gorilla/websocket) has a PongHandler that resets its
//   ReadDeadline on any Pong. Since wsutil.ReadServerData does not automatically
//   respond to server Pings with Pongs, we send unsolicited Pongs to prevent the
//   server from closing the connection after pongWait (60s).
//
// On WSL2, the interval is halved (15s) because the Hyper-V NAT has aggressive
// conntrack timeouts.
func (c *RelayClient) pingLoop(ctx context.Context) {
	interval := 30 * time.Second
	if isWSL2() {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			alive := c.connected
			c.mu.Unlock()

			if !alive || conn == nil {
				continue
			}

			// Send both Ping (for proxies) and Pong (for gorilla server's PongHandler).
			c.mu.Lock()
			_ = wsutil.WriteClientMessage(conn, ws.OpPing, nil)
			err := wsutil.WriteClientMessage(conn, ws.OpPong, nil)
			c.mu.Unlock()
			if err != nil {
				// Write failed — connection is dead. readLoop will detect and trigger reconnect.
				continue
			}

			// Reset readLoop's ReadDeadline. wsutil.ReadServerData consumes control
			// frames internally without returning, so the 90s deadline would expire
			// even on healthy connections that only exchange Ping/Pong. A successful
			// write proves the TCP connection is alive — extend the read deadline.
			conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		}
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// isWSL2 detects if the process is running inside WSL2 by checking
// /proc/version for "microsoft" (case-insensitive) on Linux.
func isWSL2() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}
