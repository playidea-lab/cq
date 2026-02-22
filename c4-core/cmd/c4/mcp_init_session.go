package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/session"
)

func init() {
	registerInitHook(initSession)
	registerShutdownHook(shutdownSession)
}

// initSession starts a session monitor for the current project directory and
// emits a cloud-upgrade warning to stderr when the active session count exceeds
// the configured limit (sessions.limit, default 4).
//
// The check is skipped when sessions.enabled is false or when the monitor
// returns ActiveCount() == 0 (Windows stub always returns 0).
func initSession(ctx *initContext) error {
	if ctx.cfgMgr == nil {
		return nil
	}
	cfg := ctx.cfgMgr.GetConfig()
	if !cfg.Sessions.Enabled {
		return nil
	}

	c4Dir := filepath.Join(ctx.projectDir, ".c4")
	mon, err := session.New(c4Dir)
	if err != nil {
		// Non-fatal: session monitor failure must not block MCP startup.
		fmt.Fprintf(os.Stderr, "cq: session monitor init failed (skipping): %v\n", err)
		return nil
	}

	if err := mon.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "cq: session monitor start failed (skipping): %v\n", err)
		return nil
	}

	// Store the monitor on initContext for shutdown cleanup.
	ctx.sessionMonitor = mon

	count := mon.ActiveCount()
	limit := cfg.Sessions.Limit
	// Windows stub returns 0 → warning never fires (correct behavior).
	if count > limit {
		fmt.Fprintf(os.Stderr, "cq: ⚠️  활성 세션 수 (%d/%d)가 권장 한도를 초과했습니다.\n", count, limit)
		fmt.Fprintf(os.Stderr, "cq:    현재 설정에서 SQLite 경합이 발생할 수 있습니다.\n")
		fmt.Fprintf(os.Stderr, "cq:    클라우드 모드로 전환하면 무제한 병렬 세션이 가능합니다: cq cloud mode set cloud-primary\n")
	}

	return nil
}

// shutdownSession stops the session monitor, removing this process's lock file.
func shutdownSession(ctx *initContext) {
	if ctx.sessionMonitor != nil {
		_ = ctx.sessionMonitor.Stop()
	}
}
