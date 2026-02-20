//go:build !llm_gateway

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterLLMHandlers is a no-op stub when llm_gateway build tag is disabled.
func RegisterLLMHandlers(_ *mcp.Registry, _ any) {}
