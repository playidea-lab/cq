package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

// registerCoreServeComponents registers always-available components:
// EventBus, GPU, and Agent. Returns the started EventBus component
// so optional components (EventSink, HubPoller) can wire up a publisher.
func registerCoreServeComponents(mgr *serve.Manager, cfg config.C4Config, home string) *serve.EventBusComponent {
	var ebComp *serve.EventBusComponent

	// EventBus
	if cfg.Serve.EventBus.Enabled {
		ebComp = serve.NewEventBusComponent(serve.EventBusConfigFromConfig(cfg.EventBus, projectDir))
		mgr.Register(ebComp)
		fmt.Fprintf(os.Stderr, "cq serve: registered eventbus\n")
	}

	// GPU scheduler
	if cfg.Serve.GPU.Enabled {
		dataDir := filepath.Join(home, ".c4", "daemon")
		mgr.Register(serve.NewGPUComponent(serve.GPUComponentConfig{
			DataDir: dataDir,
			MaxJobs: 0,
			Version: "dev",
		}))
		fmt.Fprintf(os.Stderr, "cq serve: registered gpu\n")
	}

	// Agent (Supabase Realtime @cq mention listener)
	if cfg.Serve.Agent.Enabled && cfg.Cloud.URL != "" && cfg.Cloud.AnonKey != "" {
		mgr.Register(serve.NewAgent(serve.AgentConfig{
			SupabaseURL: cfg.Cloud.URL,
			APIKey:      cfg.Cloud.AnonKey,
	
			ProjectID:   cfg.Cloud.ProjectID,
		}))
		fmt.Fprintf(os.Stderr, "cq serve: registered agent\n")
	}

	return ebComp
}
