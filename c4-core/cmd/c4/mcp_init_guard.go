//go:build c6_guard

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerInitHook(initGuard)
	registerEBWireHook(wireGuardEventBus)
}

// initGuard initializes the C6 guard engine and registers guard MCP tools.
func initGuard(ctx *initContext) error {
	cfg := guard.Config{Enabled: false} // default: disabled until explicitly configured
	if ctx.cfgMgr != nil {
		raw := ctx.cfgMgr.GetConfig().Guard
		cfg.Enabled = raw.Enabled
		cfg.Policies = make([]guard.PolicyRule, 0, len(raw.Policies))
		for _, p := range raw.Policies {
			var action guard.Action
			switch p.Action {
			case "deny":
				action = guard.ActionDeny
			case "audit_only":
				action = guard.ActionAuditOnly
			default:
				action = guard.ActionAllow
			}
			cfg.Policies = append(cfg.Policies, guard.PolicyRule{
				Tool:     p.Tool,
				Action:   action,
				Reason:   p.Reason,
				Priority: p.Priority,
			})
		}
	}

	dbPath := filepath.Join(ctx.projectDir, ".c4", "guard.db")
	eng, err := guard.NewEngine(dbPath, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: guard engine init failed: %v\n", err)
		return nil // non-fatal: guard unavailable but MCP server still starts
	}

	ctx.guardEngine = eng
	handlers.RegisterGuardHandlers(ctx.reg, eng)
	fmt.Fprintf(os.Stderr, "cq: guard enabled (5 tools, db=%s)\n", dbPath)
	return nil
}

// wireGuardEventBus wires the EventBus client into the guard engine so that
// ActionDeny decisions emit a "guard.denied" event.
func wireGuardEventBus(ctx *initContext, ebClient *eventbus.Client) {
	eng, ok := ctx.guardEngine.(*guard.Engine)
	if !ok || eng == nil {
		return
	}
	eng.SetPublisher(ebClient)
}
