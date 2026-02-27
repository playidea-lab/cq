package researchhandler

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
)

// Caller abstracts the JSON-RPC proxy so researchhandler does not import handlers.
type Caller interface {
	Call(method string, params map[string]any) (map[string]any, error)
}

// proxyHandler creates an MCP handler that delegates to caller.Call(method, params).
func proxyHandler(caller Caller, method string) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params == nil {
			params = make(map[string]any)
		}
		return caller.Call(method, params)
	}
}
