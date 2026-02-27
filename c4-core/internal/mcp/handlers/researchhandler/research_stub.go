//go:build !research

package researchhandler

import (
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/research"
)

// Register is a no-op stub when the research build tag is disabled.
func Register(_ *mcp.Registry, _ *research.Store, _ Caller) {}

// RegisterResearchNativeHandlers is a no-op stub when the research build tag is disabled.
func RegisterResearchNativeHandlers(_ *mcp.Registry, _ *research.Store) {}

// RegisterResearchProxyHandlers is a no-op stub when the research build tag is disabled.
func RegisterResearchProxyHandlers(_ *mcp.Registry, _ Caller) {}

// SetResearchEventBus is a no-op stub when the research build tag is disabled.
func SetResearchEventBus(_ any, _ string) {}
