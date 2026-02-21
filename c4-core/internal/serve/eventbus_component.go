package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
)

// EventBusConfig holds configuration for the EventBus component.
type EventBusConfig struct {
	// DataDir is the directory for EventBus data (socket, DB).
	// Default: ~/.c4/eventbus
	DataDir string

	// ProjectDir is the project root, used to locate default_rules.yaml.
	ProjectDir string

	// RetentionDays controls auto-purge of old events. 0 = no auto-purge.
	RetentionDays int

	// MaxEvents caps stored events. 0 = unlimited.
	MaxEvents int

	// WSPort enables WebSocket bridge when > 0.
	WSPort int

	// WSHost is the WebSocket bind address. Default: "127.0.0.1".
	WSHost string
}

// EventBusConfigFromConfig converts a config.EventBusConfig + projectDir
// into an EventBusConfig suitable for the serve component.
func EventBusConfigFromConfig(cfg config.EventBusConfig, projectDir string) EventBusConfig {
	return EventBusConfig{
		DataDir:       cfg.DataDir,
		ProjectDir:    projectDir,
		RetentionDays: cfg.RetentionDays,
		MaxEvents:     cfg.MaxEvents,
		WSPort:        cfg.WSPort,
		WSHost:        cfg.WSHost,
	}
}

// EventBusComponent wraps eventbus.StartEmbedded as a Component.
type EventBusComponent struct {
	cfg    EventBusConfig
	mu     sync.RWMutex
	server *eventbus.EmbeddedServer

	healthMu    sync.Mutex
	healthCache struct {
		result ComponentHealth
		at     time.Time
	}
}

// NewEventBusComponent creates a new EventBus component with the given config.
func NewEventBusComponent(cfg EventBusConfig) *EventBusComponent {
	return &EventBusComponent{cfg: cfg}
}

func (c *EventBusComponent) Name() string { return "eventbus" }

func (c *EventBusComponent) Start(ctx context.Context) error {
	dataDir := c.cfg.DataDir
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("eventbus: home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".c4", "eventbus")
	}

	// Resolve default_rules.yaml path
	defaultRulesPath := ""
	if c.cfg.ProjectDir != "" {
		candidate := filepath.Join(c.cfg.ProjectDir, "c4-core", "internal", "eventbus", "default_rules.yaml")
		if _, err := os.Stat(candidate); err == nil {
			defaultRulesPath = candidate
		}
	}
	if defaultRulesPath == "" {
		candidate := filepath.Join(dataDir, "default_rules.yaml")
		if _, err := os.Stat(candidate); err == nil {
			defaultRulesPath = candidate
		}
	}

	eb, err := eventbus.StartEmbedded(eventbus.EmbeddedConfig{
		DataDir:          dataDir,
		RetentionDays:    c.cfg.RetentionDays,
		MaxEvents:        c.cfg.MaxEvents,
		DefaultRulesPath: defaultRulesPath,
		WSPort:           c.cfg.WSPort,
		WSHost:           c.cfg.WSHost,
	})
	if err != nil {
		return fmt.Errorf("eventbus: start: %w", err)
	}

	c.mu.Lock()
	c.server = eb
	c.mu.Unlock()

	return nil
}

func (c *EventBusComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	eb := c.server
	c.server = nil
	c.mu.Unlock()

	if eb != nil {
		eb.Stop()
	}
	return nil
}

func (c *EventBusComponent) Health() ComponentHealth {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	if !c.healthCache.at.IsZero() && time.Since(c.healthCache.at) < 5*time.Second {
		return c.healthCache.result
	}

	result := c.doHealth()
	c.healthCache.result = result
	c.healthCache.at = time.Now()
	return result
}

func (c *EventBusComponent) doHealth() ComponentHealth {
	c.mu.RLock()
	eb := c.server
	c.mu.RUnlock()

	if eb == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}

	sockPath := eb.SocketPath()
	if _, err := os.Stat(sockPath); err != nil {
		return ComponentHealth{Status: "error", Detail: fmt.Sprintf("socket missing: %s", sockPath)}
	}

	// Try connecting a client to verify the gRPC server is responsive.
	client, err := eventbus.NewClient(sockPath)
	if err != nil {
		return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("client connect: %v", err)}
	}
	defer client.Close()

	if _, err := client.GetStats(); err != nil {
		return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("ping failed: %v", err)}
	}

	return ComponentHealth{Status: "ok"}
}

// Server returns the underlying EmbeddedServer, or nil if not started.
// This allows callers to wire additional components (Dispatcher, Client, etc.).
func (c *EventBusComponent) Server() *eventbus.EmbeddedServer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.server
}

// SocketPath returns the Unix socket path, or empty string if not started.
func (c *EventBusComponent) SocketPath() string {
	c.mu.RLock()
	eb := c.server
	c.mu.RUnlock()
	if eb == nil {
		return ""
	}
	return eb.SocketPath()
}
