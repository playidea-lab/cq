//go:build !llm_gateway

package llmhandler

import (
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

// RegisterLLMHandlers is a no-op stub when llm_gateway build tag is disabled.
func RegisterLLMHandlers(_ *mcp.Registry, _ any, _ any) {}

// RegisterCostTrackerWidget is a no-op stub when llm_gateway build tag is disabled.
func RegisterCostTrackerWidget(_ *apps.ResourceStore, _ string) {}
