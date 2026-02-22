//go:build c8_gate

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/gate"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerInitHook(initGate)
	registerEBWireHook(wireGateEventBus)
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

// wireGateEventBus subscribes the WebhookManager to EventBus events via a bridge.
// Goroutine lifecycle is tied to gctx; shutdownGate cancels it via gateBridgeCancel.
func wireGateEventBus(ctx *initContext, ebClient *eventbus.Client) {
	if ctx.gateWebhookManager == nil {
		return
	}
	wm, ok := ctx.gateWebhookManager.(*gate.WebhookManager)
	if !ok {
		return
	}
	bridge := gate.NewEventBusBridge(wm)
	gctx, cancel := context.WithCancel(context.Background())
	ctx.gateBridgeCancel = cancel

	go func() {
		ch, err := ebClient.Subscribe(gctx, "task.completed", "")
		if err != nil {
			slog.Warn("gate: failed to subscribe to task.completed", "err", err)
			cancel() // release the context; sibling goroutine will observe Done()
			return
		}
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				bridge.Feed("task.completed", ev.Data)
			case <-gctx.Done():
				return
			}
		}
	}()

	go func() {
		ch, err := ebClient.Subscribe(gctx, "hub.job.completed", "")
		if err != nil {
			slog.Warn("gate: failed to subscribe to hub.job.completed", "err", err)
			cancel() // release the context; sibling goroutine will observe Done()
			return
		}
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				bridge.Feed("hub.job.completed", ev.Data)
			case <-gctx.Done():
				return
			}
		}
	}()
}

// shutdownGate stops the scheduler and bridge goroutines on server shutdown.
func shutdownGate(ctx *initContext) {
	if ctx.gateBridgeCancel != nil {
		ctx.gateBridgeCancel()
	}
	if ctx.gateScheduler != nil {
		ctx.gateScheduler.Stop()
	}
}
