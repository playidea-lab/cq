//go:build c8_gate

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/gate"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerInitHook(initGate)
	registerShutdownHook(shutdownGate)
}

// initGate initializes the C8 Gate subsystem: WebhookManager, Scheduler, and connectors.
func initGate(ctx *initContext) error {
	if ctx.cfgMgr == nil {
		return nil
	}
	cfg := ctx.cfgMgr.GetConfig()
	gateCfg := cfg.Gate
	if !gateCfg.Enabled {
		return nil
	}

	wm := gate.NewWebhookManager(gate.WebhookConfig{})
	store := gate.NewMemoryJobStore()
	sched := gate.NewScheduler(store)

	var slack *gate.SlackConnector
	if gateCfg.Connectors.Slack.Enabled && gateCfg.Connectors.Slack.WebhookURL != "" {
		slack = gate.NewSlackConnector(gateCfg.Connectors.Slack.WebhookURL)
	}

	var github *gate.GitHubConnector
	if gateCfg.Connectors.GitHub.Enabled && gateCfg.Connectors.GitHub.PAT != "" {
		github = gate.NewGitHubConnector(gate.GitHubConfig{PAT: gateCfg.Connectors.GitHub.PAT})
	}

	handlers.RegisterGateHandlers(ctx.reg, wm, sched, slack, github)

	ctx.gateWebhookManager = wm
	ctx.gateScheduler = sched

	fmt.Fprintln(os.Stderr, "cq: gate enabled (6 tools)")
	return nil
}

// shutdownGate stops the scheduler on server shutdown.
func shutdownGate(ctx *initContext) {
	if ctx.gateScheduler != nil {
		ctx.gateScheduler.Stop()
	}
}
