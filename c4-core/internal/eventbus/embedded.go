package eventbus

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"google.golang.org/grpc"
)

// EmbeddedServer runs an in-process gRPC EventBus server.
// Used for auto-start mode where the MCP server manages the EventBus lifecycle.
type EmbeddedServer struct {
	store      *Store
	dispatcher *Dispatcher
	server     *Server
	grpcServer *grpc.Server
	wsBridge   *WSBridge
	sockPath   string
	listener   net.Listener
	stopPurge  chan struct{}
	stopOnce   sync.Once
}

// EmbeddedConfig holds configuration for the embedded server.
type EmbeddedConfig struct {
	DataDir           string
	RetentionDays     int    // 0 = no auto-purge
	MaxEvents         int    // 0 = unlimited
	DefaultRulesPath  string // path to default_rules.yaml (empty = skip)
	WSPort            int    // 0 = WebSocket bridge disabled
	DLQRetentionDays  int    // 0 = use 2x RetentionDays; >0 = explicit DLQ retention
}

// StartEmbedded creates and starts an in-process gRPC EventBus server.
// The server listens on a Unix socket in dataDir and manages its own store.
func StartEmbedded(cfg EmbeddedConfig) (*EmbeddedServer, error) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".c4", "eventbus")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "events.db")
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	dispatcher := NewDispatcher(store)
	srv := NewServer(ServerConfig{
		Store:      store,
		Dispatcher: dispatcher,
	})

	sockPath := filepath.Join(dataDir, "embedded.sock")
	// Remove stale socket
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("listen unix %s: %w", sockPath, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterEventBusServer(grpcServer, srv)

	go grpcServer.Serve(ln)

	e := &EmbeddedServer{
		store:      store,
		dispatcher: dispatcher,
		server:     srv,
		grpcServer: grpcServer,
		sockPath:   sockPath,
		listener:   ln,
		stopPurge:  make(chan struct{}),
	}

	// Load default rules (best-effort)
	if cfg.DefaultRulesPath != "" {
		if yamlData, err := os.ReadFile(cfg.DefaultRulesPath); err == nil {
			if err := store.EnsureDefaultRules(yamlData); err != nil {
				fmt.Fprintf(os.Stderr, "c4: eventbus: load default rules: %v\n", err)
			}
		}
	}

	// Start WebSocket bridge if configured
	if cfg.WSPort > 0 {
		e.wsBridge = NewWSBridge(srv, cfg.WSPort)
		go e.wsBridge.Start()
	}

	// Start auto-purge goroutine if configured
	if cfg.RetentionDays > 0 || cfg.MaxEvents > 0 {
		dlqDays := cfg.DLQRetentionDays
		if dlqDays <= 0 && cfg.RetentionDays > 0 {
			dlqDays = cfg.RetentionDays * 2 // DLQ entries retained 2x longer by default
		}
		go e.autoPurge(cfg.RetentionDays, cfg.MaxEvents, dlqDays)
	}

	return e, nil
}

// Stop gracefully shuts down the embedded server. Safe to call multiple times.
func (e *EmbeddedServer) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopPurge)
		if e.wsBridge != nil {
			e.wsBridge.Stop()
		}
		e.grpcServer.GracefulStop()
		e.listener.Close()
		os.Remove(e.sockPath)
		e.store.Close()
	})
}

// SocketPath returns the Unix socket path for client connections.
func (e *EmbeddedServer) SocketPath() string {
	return e.sockPath
}

// Store returns the underlying store (for direct access from in-process code).
func (e *EmbeddedServer) Store() *Store {
	return e.store
}

// Dispatcher returns the underlying dispatcher (for wiring C1Poster, etc.).
func (e *EmbeddedServer) Dispatcher() *Dispatcher {
	return e.dispatcher
}

func (e *EmbeddedServer) autoPurge(retentionDays, maxEvents, dlqRetentionDays int) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if retentionDays > 0 {
				maxAge := time.Duration(retentionDays) * 24 * time.Hour
				if n, err := e.store.PurgeOldEvents(maxAge); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "c4: eventbus: purged %d old events\n", n)
				}
				if n, err := e.store.PurgeOldLogs(maxAge); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "c4: eventbus: purged %d old logs\n", n)
				}
			}
			if maxEvents > 0 {
				if n, err := e.store.PurgeByCount(maxEvents); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "c4: eventbus: purged %d events (count limit)\n", n)
				}
			}
			// Purge old DLQ entries (separate retention, default 2x events)
			if dlqRetentionDays > 0 {
				dlqMaxAge := time.Duration(dlqRetentionDays) * 24 * time.Hour
				if n, err := e.store.PurgeDLQ(dlqMaxAge); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "c4: eventbus: purged %d old DLQ entries\n", n)
				}
			}
		case <-e.stopPurge:
			return
		}
	}
}
