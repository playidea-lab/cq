//go:build c3_eventbus

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/mcp/handlers/eventbushandler"
	"github.com/changmin/c4-core/internal/mcp/handlers/knowledgehandler"
	"github.com/changmin/c4-core/internal/serve"
)

func init() {
	registerInitHook(initEventBus)
	registerShutdownHook(shutdownEventBus)
}

// initEventBus starts the embedded or remote EventBus and wires all subscribers.
func initEventBus(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().EventBus.Enabled {
		wireLocalDispatcher(ctx)
		return nil
	}
	ebCfg := ctx.cfgMgr.GetConfig().EventBus

	// If cq serve is running, skip embedded and connect to its remote EventBus.
	// This avoids socket conflicts since serve already manages the EventBus.
	if ebCfg.AutoStart && serve.IsServeRunning() {
		fmt.Fprintln(os.Stderr, "cq: serve running, using remote eventbus")
		initRemoteEB(ctx)
	} else if ebCfg.AutoStart {
		initEmbeddedEB(ctx)
	} else {
		initRemoteEB(ctx)
	}

	// Wire local Dispatcher for C1 posting if no embedded server was started
	wireLocalDispatcher(ctx)
	return nil
}

// initEmbeddedEB starts the in-process EventBus and wires all components.
func initEmbeddedEB(ctx *initContext) {
	ebCfg := ctx.cfgMgr.GetConfig().EventBus
	dataDir := ebCfg.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(ctx.projectDir, ".c4", "eventbus")
	}
	defaultRulesPath := filepath.Join(ctx.projectDir, "c4-core", "internal", "eventbus", "default_rules.yaml")
	if _, err := os.Stat(defaultRulesPath); err != nil {
		defaultRulesPath = filepath.Join(dataDir, "default_rules.yaml")
		if _, err := os.Stat(defaultRulesPath); err != nil {
			defaultRulesPath = ""
		}
	}

	eb, ebErr := eventbus.StartEmbedded(eventbus.EmbeddedConfig{
		DataDir:          dataDir,
		RetentionDays:    ebCfg.RetentionDays,
		MaxEvents:        ebCfg.MaxEvents,
		DefaultRulesPath: defaultRulesPath,
		WSPort:           ebCfg.WSPort,
		WSHost:           ebCfg.WSHost,
	})
	if ebErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventbus auto-start failed: %v\n", ebErr)
		return
	}
	ctx.embeddedEB = eb

	if ctx.keeper != nil {
		eb.Dispatcher().SetC1Poster(ctx.keeper)
	}

	ebClient, ebErr := eventbus.NewClient(eb.SocketPath())
	if ebErr != nil {
		return
	}

	wireAllEventBus(ctx, ebClient)
	eventbushandler.RegisterEventBusHandlers(ctx.reg, ebClient, ctx.cfgMgr)
	eventbushandler.RegisterDoorayRespondTool(ctx.reg)
	ctx.sqliteStore.SetEventBus(ebClient)
	ctx.proxy.SetEventBus(ebClient)

	ctx.sqliteStore.SetDispatcher(eb.Dispatcher())

	if js := hubJobSubmitter(ctx); js != nil {
		eb.Dispatcher().SetHubSubmitter(js)
	}

	fmt.Fprintf(os.Stderr, "cq: eventbus auto-started (embedded, %s)\n", eb.SocketPath())
}

// initRemoteEB connects to a remote EventBus daemon.
func initRemoteEB(ctx *initContext) {
	ebCfg := ctx.cfgMgr.GetConfig().EventBus
	sockPath := ebCfg.SocketPath
	if sockPath == "" {
		home, _ := os.UserHomeDir()
		sockPath = filepath.Join(home, ".c4", "eventbus", "c3.sock")
	}
	ebClient, ebErr := eventbus.NewClient(sockPath)
	if ebErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventbus not reachable (unix:%s): %v\n", sockPath, ebErr)
		return
	}

	wireAllEventBus(ctx, ebClient)
	eventbushandler.RegisterEventBusHandlers(ctx.reg, ebClient, ctx.cfgMgr)
	eventbushandler.RegisterDoorayRespondTool(ctx.reg)
	ctx.sqliteStore.SetEventBus(ebClient)
	ctx.proxy.SetEventBus(ebClient)

	fmt.Fprintf(os.Stderr, "cq: eventbus connected (unix:%s, 8 tools)\n", sockPath)
}

// wireAllEventBus fires all component EB wiring hooks plus core wiring.
func wireAllEventBus(ctx *initContext, ebClient *eventbus.Client) {
	for _, fn := range componentEBWireHooks {
		fn(ctx, ebClient)
	}
	// Core wiring (always active, not build-tagged)
	handlers.SetValidationEventBus(ebClient)
	knowledgehandler.SetKnowledgeEventBus(ebClient, ctx.sqliteStore.GetProjectID())
}

// wireLocalDispatcher creates a minimal local dispatcher for C1 posting
// when no EventBus is available.
func wireLocalDispatcher(ctx *initContext) {
	if ctx.keeper == nil || ctx.embeddedEB != nil {
		return
	}
	localDBPath := filepath.Join(ctx.projectDir, ".c4", "eventbus", "local.db")
	localStore, localErr := eventbus.NewStore(localDBPath)
	if localErr != nil {
		fmt.Fprintf(os.Stderr, "cq: local eventbus store failed: %v\n", localErr)
		return
	}
	localDispatcher := eventbus.NewDispatcher(localStore)
	localDispatcher.SetC1Poster(ctx.keeper)

	rules, _ := localStore.MatchRules("task.completed")
	if len(rules) == 0 {
		localStore.AddRule(
			"c1-task-updates",
			"task.*",
			"",
			"c1_post",
			`{"channel":"#updates","template":"[{{event_type}}] {{task_id}}: {{title}}"}`,
			true,
			0,
		)
	}

	ctx.sqliteStore.SetDispatcher(localDispatcher)
	fmt.Fprintln(os.Stderr, "cq: local dispatcher wired (c1_post rules)")
}

// startEventSink starts the EventSink HTTP server if configured.
func startEventSink(ctx *initContext) {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().EventSink.Enabled {
		return
	}
	if serve.IsServeRunning() {
		fmt.Fprintln(os.Stderr, serve.StatusMessage("eventsink"))
		return
	}
	esCfg := ctx.cfgMgr.GetConfig().EventSink
	hubEventPub := handlers.GetHubEventPub()
	esSrv, esErr := eventbushandler.StartEventSinkServer(esCfg.Port, esCfg.Token, hubEventPub)
	if esErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventsink start failed: %v\n", esErr)
		return
	}
	if esSrv != nil {
		ctx.eventsinkSrv = esSrv
		fmt.Fprintf(os.Stderr, "cq: eventsink listening on :%d\n", esCfg.Port)
	}
}

// shutdownEventBus cleans up EventBus resources.
func shutdownEventBus(ctx *initContext) {
	if ctx.eventsinkSrv != nil {
		ctx.eventsinkSrv.Close()
	}
	if ctx.embeddedEB != nil {
		ctx.embeddedEB.Stop()
	}
}
