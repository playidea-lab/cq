package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// WSEvent is the JSON payload sent to WebSocket clients.
type WSEvent struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Source        string          `json:"source"`
	Data          json.RawMessage `json:"data"`
	ProjectID     string          `json:"project_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	TimestampMs   int64           `json:"timestamp_ms"`
}

// WSBridge bridges EventBus events to WebSocket clients.
type WSBridge struct {
	server     *Server
	httpServer *http.Server
	port       int

	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

type wsClient struct {
	conn      net.Conn
	pattern   string
	done      chan struct{}
	closeOnce sync.Once
}

// NewWSBridge creates a new WebSocket bridge for the given EventBus server.
func NewWSBridge(srv *Server, port int) *WSBridge {
	return &WSBridge{
		server:  srv,
		port:    port,
		clients: make(map[*wsClient]struct{}),
	}
}

// Start begins listening for WebSocket connections. Blocks until Stop is called.
func (b *WSBridge) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/events", b.handleEvents)

	b.httpServer = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", b.port),
		Handler: mux,
	}

	fmt.Fprintf(os.Stderr, "c4: eventbus: ws bridge listening on 127.0.0.1:%d\n", b.port)
	err := b.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the WebSocket bridge.
func (b *WSBridge) Stop() {
	if b.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b.httpServer.Shutdown(ctx)
	}

	b.mu.Lock()
	for c := range b.clients {
		c.closeOnce.Do(func() { close(c.done) })
		c.conn.Close()
	}
	b.clients = make(map[*wsClient]struct{})
	b.mu.Unlock()
}

func (b *WSBridge) handleEvents(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}

	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		pattern = "*"
	}

	client := &wsClient{
		conn:    conn,
		pattern: pattern,
		done:    make(chan struct{}),
	}

	b.mu.Lock()
	b.clients[client] = struct{}{}
	b.mu.Unlock()

	// Register as EventBus subscriber
	evCh := make(chan *pb.Event, 64)
	b.server.addSubscriber(pattern, evCh)

	// Read goroutine: detect client disconnect
	go func() {
		for {
			if _, _, err := wsutil.ReadClientData(conn); err != nil {
				client.closeOnce.Do(func() { close(client.done) })
				return
			}
		}
	}()

	// Forward events
	defer func() {
		b.server.removeSubscriber(pattern, evCh)
		b.mu.Lock()
		delete(b.clients, client)
		b.mu.Unlock()
		conn.Close()
	}()

	for {
		select {
		case ev, ok := <-evCh:
			if !ok {
				return
			}

			wsEv := WSEvent{
				ID:            ev.Id,
				Type:          ev.Type,
				Source:        ev.Source,
				Data:          json.RawMessage(ev.Data),
				ProjectID:     ev.ProjectId,
				CorrelationID: ev.CorrelationId,
				TimestampMs:   ev.TimestampMs,
			}

			data, err := json.Marshal(wsEv)
			if err != nil {
				continue
			}

			if err := wsutil.WriteServerText(conn, data); err != nil {
				return
			}

		case <-client.done:
			return
		}
	}
}

