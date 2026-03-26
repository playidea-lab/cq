package handlers

import (
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDocProxyHandlers registers document parsing tools that require the Python sidecar.
func RegisterDocProxyHandlers(reg *mcp.Registry, proxy *BridgeProxy) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_parse_document",
		Description: "Parse multi-format document (HWP, DOCX, PDF, XLSX, PPTX) into IR blocks",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Path to the document file"},
			},
			"required": []string{"file_path"},
		},
	}, proxyHandlerWithTimeout(proxy, "C2ParseDocument", 30*time.Second))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_extract_text",
		Description: "Extract plain text from any supported document format",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Path to the document file"},
			},
			"required": []string{"file_path"},
		},
	}, proxyHandlerWithTimeout(proxy, "C2ExtractText", 30*time.Second))
}
