package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

// server is the relay server state.
type server struct {
	upgrader  websocket.Upgrader
	workers   sync.RWMutex
	conns     map[string]*workerConn // worker_id → workerConn
	jwtSecret string
}

func newServer() *server {
	return &server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		conns:     make(map[string]*workerConn),
		jwtSecret: os.Getenv("SUPABASE_JWT_SECRET"),
	}
}

// validateToken performs minimal HMAC-SHA256 JWT signature verification.
// When jwtSecret is empty (dev mode), validation is skipped.
func (s *server) validateToken(tokenStr string) error {
	if s.jwtSecret == "" {
		return nil // dev mode
	}
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT format")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(s.jwtSecret))
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("invalid JWT signature encoding")
	}
	if !hmac.Equal(sigBytes, expected) {
		return fmt.Errorf("invalid JWT signature")
	}
	return nil
}

func (s *server) workerCount() int {
	s.workers.RLock()
	defer s.workers.RUnlock()
	return len(s.conns)
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
	s.conns[id] = w
}

func (s *server) removeWorker(id string) {
	s.workers.Lock()
	defer s.workers.Unlock()
	delete(s.conns, id)
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
		s.removeWorker(workerID)
		log.Printf("worker disconnected: %s", workerID)
	}()

	// Read loop: receive responses from worker and deliver to waiting handlers.
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("worker %s read error: %v", workerID, err)
			}
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

// handleMCP handles POST /w/{id}/mcp — HTTP→WSS relay.
func (s *server) handleMCP(w http.ResponseWriter, r *http.Request) {
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
		"status":  "ok",
		"workers": s.workerCount(),
	})
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
	mux.HandleFunc("GET /w/{id}/health", srv.handleWorkerHealth)
	mux.HandleFunc("GET /health", srv.handleHealth)

	addr := ":" + port
	log.Printf("relay server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
