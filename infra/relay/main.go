package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	pingInterval   = 30 * time.Second
	requestTimeout = 30 * time.Second
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
)

// relayMessage is the envelope wrapping JSON-RPC over WSS.
type relayMessage struct {
	RelayID string          `json:"relay_id"`
	Body    json.RawMessage `json:"body"`
}

// workerConn holds a connected worker's websocket and pending response channels.
type workerConn struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	pending map[string]chan json.RawMessage // relay_id → response channel
}

func newWorkerConn(c *websocket.Conn) *workerConn {
	return &workerConn{
		conn:    c,
		pending: make(map[string]chan json.RawMessage),
	}
}

// sendAndWait registers a response channel, sends the relay message, and blocks until response or timeout.
func (w *workerConn) sendAndWait(ctx context.Context, relayID string, body json.RawMessage) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)

	w.mu.Lock()
	w.pending[relayID] = ch
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.pending, relayID)
		w.mu.Unlock()
	}()

	msg := relayMessage{RelayID: relayID, Body: body}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal relay message: %w", err)
	}

	w.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := w.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, fmt.Errorf("write to worker: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("request timeout waiting for worker response")
	}
}

// deliver routes an incoming WSS message to the waiting HTTP handler.
func (w *workerConn) deliver(relayID string, body json.RawMessage) bool {
	w.mu.Lock()
	ch, ok := w.pending[relayID]
	w.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- body:
	default:
	}
	return true
}

// tunnel holds a bidirectional binary pipe between a sender and receiver.
type tunnel struct {
	id        string
	sender    *websocket.Conn
	receiver  *websocket.Conn
	ready     chan struct{} // closed when both sides connected
	done      chan struct{} // closed when tunnel finished
	createdAt time.Time
	mu        sync.Mutex
}

// server is the relay server state.
type server struct {
	upgrader    websocket.Upgrader
	workers     sync.RWMutex
	conns       map[string]*workerConn // worker_id → workerConn
	tunnels     sync.RWMutex
	tunnelMap   map[string]*tunnel // tunnel_id → tunnel
	supabaseURL string // e.g. "https://xxx.supabase.co"
	supabaseKey string // anon key for Supabase Auth API
	httpClient  *http.Client
}

func newServer() *server {
	srv := &server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		conns:       make(map[string]*workerConn),
		tunnelMap:   make(map[string]*tunnel),
		supabaseURL: os.Getenv("SUPABASE_URL"),
		supabaseKey: os.Getenv("SUPABASE_ANON_KEY"),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
	go srv.sweepTunnels()
	return srv
}

// sweepTunnels periodically removes tunnels older than 5 minutes.
func (s *server) sweepTunnels() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.tunnels.Lock()
		for id, t := range s.tunnelMap {
			if time.Since(t.createdAt) > 5*time.Minute {
				select {
				case <-t.done:
				default:
					close(t.done)
				}
				t.mu.Lock()
				if t.sender != nil {
					t.sender.Close()
				}
				if t.receiver != nil {
					t.receiver.Close()
				}
				t.mu.Unlock()
				delete(s.tunnelMap, id)
				log.Printf("tunnel swept (5m): %s", id)
			}
		}
		s.tunnels.Unlock()
	}
}

// validateToken verifies a Supabase Auth JWT by calling the Supabase Auth API.
// When supabaseURL is empty (dev mode), validation is skipped.
func (s *server) validateToken(tokenStr string) error {
	if s.supabaseURL == "" {
		return nil // dev mode
	}

	// Call Supabase Auth /auth/v1/user to validate the token
	req, err := http.NewRequest("GET", s.supabaseURL+"/auth/v1/user", nil)
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	req.Header.Set("apikey", s.supabaseKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid token (status %d)", resp.StatusCode)
	}
	return nil
}

func (s *server) workerCount() int {
	s.workers.RLock()
	defer s.workers.RUnlock()
	return len(s.conns)
}

func (s *server) workerNames() []string {
	s.workers.RLock()
	defer s.workers.RUnlock()
	names := make([]string, 0, len(s.conns))
	for id := range s.conns {
		names = append(names, id)
	}
	return names
}

func (s *server) getWorker(id string) (*workerConn, bool) {
	s.workers.RLock()
	defer s.workers.RUnlock()
	w, ok := s.conns[id]
	return w, ok
}

func (s *server) addWorker(id string, w *workerConn) {
	s.workers.Lock()
	defer s.workers.Unlock()
	if old, ok := s.conns[id]; ok {
		old.conn.Close() // close stale connection to prevent goroutine leak
	}
	s.conns[id] = w
}

// removeWorkerConn removes a worker only if the registered connection matches.
// This prevents a stale goroutine from removing a newer connection that replaced it.
func (s *server) removeWorkerConn(id string, wc *workerConn) {
	s.workers.Lock()
	defer s.workers.Unlock()
	if current, ok := s.conns[id]; ok && current == wc {
		delete(s.conns, id)
	}
}

// handleConnect handles GET /connect?token=JWT&worker_id=xxx — WebSocket upgrade.
func (s *server) handleConnect(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	workerID := r.URL.Query().Get("worker_id")

	if workerID == "" {
		http.Error(w, "worker_id required", http.StatusBadRequest)
		return
	}

	if err := s.validateToken(token); err != nil {
		http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error for worker %s: %v", workerID, err)
		return
	}

	wc := newWorkerConn(conn)
	s.addWorker(workerID, wc)
	log.Printf("worker connected: %s", workerID)

	conn.SetReadLimit(1 << 20) // 1 MiB max message size
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Ping ticker keeps the connection alive.
	ticker := time.NewTicker(pingInterval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	defer func() {
		conn.Close()
		ticker.Stop()
		s.removeWorkerConn(workerID, wc)
		log.Printf("worker disconnected: %s", workerID)
	}()

	// Read loop: receive responses from worker and deliver to waiting handlers.
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("worker %s read error (type=%T): %v", workerID, err, err)
			return
		}
		conn.SetReadDeadline(time.Now().Add(pongWait))

		var msg relayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("worker %s sent malformed message: %v", workerID, err)
			continue
		}
		if !wc.deliver(msg.RelayID, msg.Body) {
			log.Printf("worker %s sent response for unknown relay_id %s", workerID, msg.RelayID)
		}
	}
}

// handleMCP handles POST and GET /w/{id}/mcp — MCP Streamable HTTP relay.
// POST: forwards JSON-RPC requests (initialize, tools/list, tools/call) to worker via WSS.
// GET: returns 405 (SSE not supported through relay; notifications delivered via POST response).
// DELETE: returns 200 (session termination acknowledgement).
// Requires Bearer token authentication (same JWT as worker connect).
func (s *server) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Handle GET (SSE endpoint) — relay doesn't support server-initiated events
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Send a keep-alive comment and close — client will fall back to POST-only mode
		fmt.Fprint(w, ": relay does not support SSE\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	// Handle DELETE (session termination)
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}

	// POST: relay JSON-RPC to worker
	// Authenticate client request
	authHeader := r.Header.Get("Authorization")
	if s.supabaseURL != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader { // no Bearer prefix
			http.Error(w, "unauthorized: Bearer token required", http.StatusUnauthorized)
			return
		}
		if err := s.validateToken(token); err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	workerID := r.PathValue("id")

	wc, ok := s.getWorker(workerID)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "worker offline"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB limit
	if err != nil {
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}

	relayID := uuid.New().String()
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	resp, err := wc.sendAndWait(ctx, relayID, json.RawMessage(body))
	if err != nil {
		log.Printf("relay to worker %s failed (relay_id=%s): %v", workerID, relayID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

// handleWorkerHealth handles GET /w/{id}/health — 200 if connected, 503 if not.
func (s *server) handleWorkerHealth(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	_, ok := s.getWorker(workerID)
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleHealth handles GET /health — always 200 with worker count.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"workers":      s.workerCount(),
		"worker_names": s.workerNames(),
	})
}

// handleCreateTunnel handles POST /tunnel — creates a new tunnel session.
// Requires Bearer JWT authentication (same as handleMCP).
func (s *server) handleCreateTunnel(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if s.supabaseURL != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			http.Error(w, "unauthorized: Bearer token required", http.StatusUnauthorized)
			return
		}
		if err := s.validateToken(token); err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	id := uuid.New().String()
	t := &tunnel{
		id:        id,
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
		createdAt: time.Now(),
	}

	s.tunnels.Lock()
	s.tunnelMap[id] = t
	s.tunnels.Unlock()

	log.Printf("tunnel created: %s", id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"tunnel_id": id})
}

// handleTunnelConnect handles GET /tunnel/{id}?role=sender|receiver — WSS upgrade for tunnel pipe.
func (s *server) handleTunnelConnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	role := r.URL.Query().Get("role")

	if role != "sender" && role != "receiver" {
		http.Error(w, "role must be sender or receiver", http.StatusBadRequest)
		return
	}

	s.tunnels.RLock()
	t, ok := s.tunnelMap[id]
	s.tunnels.RUnlock()
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("tunnel %s: upgrade error for role %s: %v", id, role, err)
		return
	}

	t.mu.Lock()
	if role == "sender" {
		if t.sender != nil {
			t.mu.Unlock()
			conn.Close()
			log.Printf("tunnel %s: sender already connected", id)
			return
		}
		t.sender = conn
	} else {
		if t.receiver != nil {
			t.mu.Unlock()
			conn.Close()
			log.Printf("tunnel %s: receiver already connected", id)
			return
		}
		t.receiver = conn
	}
	bothReady := t.sender != nil && t.receiver != nil
	t.mu.Unlock()

	log.Printf("tunnel %s: %s connected", id, role)

	if bothReady {
		close(t.ready)
		go s.runTunnel(t)
		return
	}

	// Wait for the other side or timeout.
	select {
	case <-t.ready:
		// glue started by the other goroutine; this one exits
	case <-t.done:
		conn.Close()
		log.Printf("tunnel %s: done before both sides connected (%s)", id, role)
	case <-time.After(30 * time.Second):
		select {
		case <-t.done:
		default:
			close(t.done)
		}
		s.tunnels.Lock()
		delete(s.tunnelMap, id)
		s.tunnels.Unlock()
		conn.Close()
		log.Printf("tunnel %s: timeout waiting for peer (role=%s)", id, role)
	}
}

// runTunnel starts the bidirectional binary glue between sender and receiver.
func (s *server) runTunnel(t *tunnel) {
	defer func() {
		select {
		case <-t.done:
		default:
			close(t.done)
		}
		t.mu.Lock()
		if t.sender != nil {
			t.sender.Close()
		}
		if t.receiver != nil {
			t.receiver.Close()
		}
		t.mu.Unlock()
		s.tunnels.Lock()
		delete(s.tunnelMap, t.id)
		s.tunnels.Unlock()
		log.Printf("tunnel closed: %s", t.id)
	}()

	errc := make(chan error, 2)

	// goroutine 1: sender → receiver
	go func() {
		for {
			mt, data, err := t.sender.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := t.receiver.WriteMessage(mt, data); err != nil {
				errc <- err
				return
			}
		}
	}()

	// goroutine 2: receiver → sender
	go func() {
		for {
			mt, data, err := t.receiver.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := t.sender.WriteMessage(mt, data); err != nil {
				errc <- err
				return
			}
		}
	}()

	<-errc
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connect", srv.handleConnect)
	mux.HandleFunc("POST /w/{id}/mcp", srv.handleMCP)
	mux.HandleFunc("GET /w/{id}/mcp", srv.handleMCP)
	mux.HandleFunc("DELETE /w/{id}/mcp", srv.handleMCP)
	mux.HandleFunc("POST /w/{id}/mcp/", srv.handleMCP)
	mux.HandleFunc("GET /w/{id}/mcp/", srv.handleMCP)
	mux.HandleFunc("DELETE /w/{id}/mcp/", srv.handleMCP)
	mux.HandleFunc("GET /w/{id}/health", srv.handleWorkerHealth)
	mux.HandleFunc("GET /health", srv.handleHealth)
	mux.HandleFunc("POST /tunnel", srv.handleCreateTunnel)
	mux.HandleFunc("GET /tunnel/{id}", srv.handleTunnelConnect)

	if srv.supabaseURL == "" {
		log.Printf("WARNING: SUPABASE_URL not set — authentication disabled (dev mode)")
	} else {
		log.Printf("auth: Supabase token validation enabled (%s)", srv.supabaseURL)
	}

	addr := ":" + port
	log.Printf("relay server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
