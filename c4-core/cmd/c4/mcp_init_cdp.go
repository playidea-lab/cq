//go:build cdp

package main

import (
	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerInitHook(initCDP)
}

// initCDP registers CDP and WebMCP handlers.
func initCDP(ctx *initContext) error {
	cdpRunner := cdp.NewRunner()
	handlers.RegisterCDPHandlers(ctx.reg, cdpRunner)
	return nil
}
