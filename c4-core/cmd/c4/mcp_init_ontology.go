package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/ontology"
)

func init() {
	registerInitHook(initOntologySeed)
	registerInitHook(initCollectiveHandlers)
}

// initOntologySeed seeds the user's L1 ontology from the project ontology
// on session start. This gives new team members project context from day one.
// Also seeds from Hub collective patterns if available (background, non-fatal).
func initOntologySeed(ctx *initContext) error {
	if ctx.projectDir == "" {
		return nil
	}

	username := os.Getenv("USER")
	if username == "" {
		return nil
	}

	// Check if user already has an L1 ontology (skip if non-empty)
	existing, err := ontology.Load(username)
	if err == nil && existing != nil && len(existing.Schema.Nodes) > 0 {
		return nil // already has ontology, skip seeding
	}

	// Seed from project ontology
	n, err := ontology.SeedFromProject(username, ctx.projectDir)
	if err != nil {
		slog.Warn("ontology: SeedFromProject failed", "error", err)
	} else if n > 0 {
		fmt.Fprintf(os.Stderr, "cq: seeded %d project ontology nodes for %s\n", n, username)
	}

	// Seed from Hub collective (best-effort, non-blocking)
	if ctx.cfgMgr != nil {
		cfg := ctx.cfgMgr.GetConfig()
		if cfg.Cloud.URL != "" && cfg.Cloud.AnonKey != "" && ctx.cloudTP != nil {
			domain := cfg.Domain
			go func() {
				dl := ontology.NewHubDownloader(cfg.Cloud.URL, cfg.Cloud.AnonKey, ctx.cloudTP.Token)
				hn, err := dl.SeedFromHub(username, domain)
				if err != nil {
					slog.Warn("ontology: SeedFromHub failed", "error", err)
				} else if hn > 0 {
					slog.Info("ontology: seeded from Hub collective", "nodes", hn, "domain", domain)
				}
			}()
		}
	}

	return nil
}

// initCollectiveHandlers registers c4_collective_sync and c4_collective_stats MCP tools.
func initCollectiveHandlers(ctx *initContext) error {
	if ctx.reg == nil || ctx.cfgMgr == nil {
		return nil
	}

	cfg := ctx.cfgMgr.GetConfig()
	var tokenFn func() string
	if ctx.cloudTP != nil {
		tokenFn = ctx.cloudTP.Token
	} else {
		tokenFn = func() string { return "" }
	}

	handlers.RegisterCollectiveHandlers(ctx.reg, handlers.CollectiveOpts{
		CloudURL:    cfg.Cloud.URL,
		CloudKey:    cfg.Cloud.AnonKey,
		TokenFn:     tokenFn,
		Domain:      cfg.Domain,
		ProjectRoot: ctx.projectDir,
	})

	return nil
}
