package webcontent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	c2webcontent "github.com/changmin/c4-core/internal/webcontent"
	"github.com/changmin/c4-core/internal/mcp"
)

// Register registers c4_web_fetch tool.
func Register(reg *mcp.Registry) {
	registerWithOpts(reg, nil)
}

func registerWithOpts(reg *mcp.Registry, defaultOpts *c2webcontent.FetchOpts) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_web_fetch",
		Description: "Fetch web content as clean markdown. Uses native markdown (Cloudflare/Vercel) when available, HTML-to-markdown fallback otherwise.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":              map[string]any{"type": "string", "description": "URL to fetch"},
				"max_length":       map[string]any{"type": "integer", "description": "Max output characters (default: 50000)"},
				"include_llms_txt": map[string]any{"type": "boolean", "description": "Also check for llms.txt (default: false)"},
			},
			"required": []string{"url"},
		},
	}, makeWebFetchHandler(defaultOpts))
}

const defaultMaxLength = 50000

func makeWebFetchHandler(defaultOpts *c2webcontent.FetchOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		rawURL, _ := params["url"].(string)
		if rawURL == "" {
			return map[string]any{"error": "url is required"}, nil
		}

		maxLength := defaultMaxLength
		if ml, ok := params["max_length"].(float64); ok && ml > 0 {
			maxLength = int(ml)
		}

		includeLLMSTxt, _ := params["include_llms_txt"].(bool)

		result, err := c2webcontent.Fetch(rawURL, defaultOpts)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("fetch failed: %v", err)}, nil
		}

		// Truncate content
		content := result.Content
		truncated := false
		if len(content) > maxLength {
			content = content[:maxLength]
			truncated = true
		}

		resp := map[string]any{
			"success":        true,
			"content":        content,
			"url":            result.URL,
			"method":         result.Method,
			"title":          result.Title,
			"token_estimate": result.TokenEstimate,
			"truncated":      truncated,
		}

		// Optionally fetch llms.txt
		if includeLLMSTxt {
			llmsTxt := fetchLLMSTxt(rawURL, defaultOpts)
			if llmsTxt != nil {
				resp["llms_txt"] = llmsTxt
			}
		}

		return resp, nil
	}
}

// fetchLLMSTxt tries to fetch /.well-known/llms.txt from the same origin.
func fetchLLMSTxt(rawURL string, baseOpts *c2webcontent.FetchOpts) map[string]any {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	llmsURL := fmt.Sprintf("%s://%s/.well-known/llms.txt", parsed.Scheme, parsed.Host)
	opts := &c2webcontent.FetchOpts{
		Timeout: 5_000_000_000, // 5s
	}
	if baseOpts != nil {
		opts.SkipSSRFCheck = baseOpts.SkipSSRFCheck
	}

	result, err := c2webcontent.Fetch(llmsURL, opts)
	if err != nil {
		return nil
	}

	// Only process if we got meaningful content
	if strings.TrimSpace(result.Content) == "" {
		return nil
	}

	llms, err := c2webcontent.ParseLLMSTxt(result.Content)
	if err != nil || llms.Title == "" {
		return nil
	}

	sections := []map[string]any{}
	for _, sec := range llms.Sections {
		links := []map[string]any{}
		for _, link := range sec.Links {
			links = append(links, map[string]any{
				"title":       link.Title,
				"url":         link.URL,
				"description": link.Description,
			})
		}
		sections = append(sections, map[string]any{
			"title": sec.Title,
			"links": links,
		})
	}

	return map[string]any{
		"title":       llms.Title,
		"description": llms.Description,
		"sections":    sections,
	}
}

// parseParams unmarshals rawArgs into a map, returning an empty map on failure.
func parseParams(rawArgs json.RawMessage) map[string]any {
	var params map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			params = nil
		}
	}
	if params == nil {
		params = make(map[string]any)
	}
	return params
}
