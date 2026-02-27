//go:build research

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers/researchhandler"
	"github.com/changmin/c4-core/internal/research"
)

func init() {
	registerPreStoreHook(initResearch)
	registerEBWireHook(wireResearchEventBus)
	registerShutdownHook(shutdownResearch)
}

// initResearch creates the research store.
// Runs as a pre-store hook so ctx.researchStore is available for NativeOpts
// before handler registration.
func initResearch(ctx *initContext) error {
	researchDir := filepath.Join(ctx.projectDir, ".c4", "research")
	os.MkdirAll(researchDir, 0755)
	rs, rsErr := research.NewStore(researchDir)
	if rsErr != nil {
		fmt.Fprintf(os.Stderr, "cq: research store init failed (proxy fallback): %v\n", rsErr)
		return nil
	}
	ctx.researchStore = rs
	return nil
}

// wireResearchEventBus wires the eventbus to Research components.
func wireResearchEventBus(ctx *initContext, ebClient *eventbus.Client) {
	researchhandler.SetResearchEventBus(ebClient, ctx.sqliteStore.GetProjectID())
}

// shutdownResearch cleans up research resources.
func shutdownResearch(ctx *initContext) {
	if ctx.researchStore != nil {
		ctx.researchStore.Close()
	}
}
