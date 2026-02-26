package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/serve"
)

// registerCoreServeComponents registers always-available components:
// EventBus, GPU, Hub, and Agent. Returns the started EventBus component
// so optional components (EventSink, HubPoller) can wire up a publisher,
// and the GPU component so the caller can mount its HTTP handler.
func registerCoreServeComponents(mgr *serve.Manager, cfg config.C4Config, home string) (*serve.EventBusComponent, *serve.GPUComponent) {
	var ebComp *serve.EventBusComponent
	var gpuComp *serve.GPUComponent

	// EventBus
	if cfg.Serve.EventBus.Enabled {
		ebComp = serve.NewEventBusComponent(serve.EventBusConfigFromConfig(cfg.EventBus, projectDir))
		mgr.Register(ebComp)
		fmt.Fprintf(os.Stderr, "cq serve: registered eventbus\n")
	}

	// GPU scheduler
	if cfg.Serve.GPU.Enabled {
		dataDir := filepath.Join(home, ".c4", "daemon")
		gpuComp = serve.NewGPUComponent(serve.GPUComponentConfig{
			DataDir: dataDir,
			MaxJobs: 0,
			Version: "dev",
		})
		mgr.Register(gpuComp)
		fmt.Fprintf(os.Stderr, "cq serve: registered gpu\n")
	}

	// C5 Hub subprocess
	if cfg.Serve.Hub.Enabled {
		mgr.Register(serve.NewHubComponent(serve.HubComponentConfig{
			Binary: cfg.Serve.Hub.Binary,
			Port:   cfg.Serve.Hub.Port,
			Args:   cfg.Serve.Hub.Args,
		}))
		fmt.Fprintf(os.Stderr, "cq serve: registered hub\n")
	}

	// Agent (Supabase Realtime @cq mention listener)
	if cfg.Serve.Agent.Enabled && cfg.Cloud.URL != "" && cfg.Cloud.AnonKey != "" {
		mgr.Register(serve.NewAgent(serve.AgentConfig{
			SupabaseURL: cfg.Cloud.URL,
			APIKey:      cfg.Cloud.AnonKey,
			ProjectID:   cfg.Cloud.ProjectID,
			ProjectDir:  projectDir,
		}))
		fmt.Fprintf(os.Stderr, "cq serve: registered agent\n")
	}

	return ebComp, gpuComp
}

// registerStaleCheckerServeComponent registers the StaleChecker component when enabled.
// It opens the project database and creates a minimal SQLiteStore for stale-task queries.
// eb is the EventBus component used to wire a publisher (nil if EventBus is disabled).
func registerStaleCheckerServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.StaleChecker.Enabled {
		return
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: stale_checker: open db failed: %v\n", err)
		return
	}

	sqliteStore, err := handlers.NewSQLiteStore(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: stale_checker: create store failed: %v\n", err)
		db.Close()
		return
	}

	var pub eventbus.Publisher
	if eb != nil {
		pub = eb.Publisher()
	}

	mgr.Register(serve.NewStaleChecker(sqliteStore, pub, cfg.Serve.StaleChecker).WithCloser(db))
	fmt.Fprintf(os.Stderr, "cq serve: registered stale_checker\n")
}
