//go:build research

package researchhandler

import (
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/research"
)

// Register is the subpackage entry point — selects native or proxy based on store availability.
func Register(reg *mcp.Registry, store *research.Store, caller Caller) {
	if store != nil {
		RegisterResearchNativeHandlers(reg, store)
	} else {
		RegisterResearchProxyHandlers(reg, caller)
	}
}
