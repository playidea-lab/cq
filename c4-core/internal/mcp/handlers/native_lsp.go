package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/changmin/c4-core/internal/dartast"
	"github.com/changmin/c4-core/internal/goast"
	"github.com/changmin/c4-core/internal/mcp"
)

// languageGuardedProxy wraps a Python sidecar method with a language check.
// For Go/Dart/Rust files it returns an early error with a hint to use the Edit tool.
// toolName is the registered MCP tool name (e.g. "c4_replace_symbol_body").
func languageGuardedProxy(proxy *BridgeProxy, method, toolName string) mcp.HandlerFunc {
	pyHandler := proxyHandler(proxy, method)
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil || params.FilePath == "" {
			return pyHandler(rawArgs)
		}

		ext := filepath.Ext(filepath.Clean(params.FilePath))
		var lang string
		switch ext {
		case ".go":
			lang = "go"
		case ".dart":
			lang = "dart"
		case ".rs":
			lang = "rust"
		}
		if lang != "" {
			return map[string]any{
				"success":             false,
				"error":               fmt.Sprintf("%s does not support %s files", toolName, lang),
				"language":            lang,
				"hint":                "Use Edit tool for Go/Dart/Rust. Supported: Python/JS/TS only.",
				"edit_example":        "Edit(file_path=..., old_string=..., new_string=...)",
				"supported_languages": []string{"python", "javascript", "typescript"},
			}, nil
		}
		return pyHandler(rawArgs)
	}
}

// goAwareFindSymbol routes find_symbol calls to native parsers for Go/Dart,
// falling back to the Python sidecar for Python/JS/TS.
func goAwareFindSymbol(proxy *BridgeProxy, rootDir string) mcp.HandlerFunc {
	pyHandler := proxyHandler(proxy, "FindSymbol")
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return pyHandler(rawArgs)
		}

		absPath, err := resolvePath(rootDir, params.Path)
		if err != nil {
			return nil, err
		}
		if goast.HasGoFiles(absPath) {
			return handleGoFindSymbol(params.Name, absPath, rootDir)
		}
		if dartast.HasDartFiles(absPath) {
			return handleDartFindSymbol(params.Name, absPath, rootDir)
		}
		return pyHandler(rawArgs)
	}
}

// goAwareSymbolsOverview routes symbols_overview calls to native parsers for Go/Dart,
// falling back to the Python sidecar for Python/JS/TS.
func goAwareSymbolsOverview(proxy *BridgeProxy, rootDir string) mcp.HandlerFunc {
	pyHandler := proxyHandler(proxy, "GetSymbolsOverview")
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return pyHandler(rawArgs)
		}

		absPath, err := resolvePath(rootDir, params.Path)
		if err != nil {
			return nil, err
		}
		if goast.HasGoFiles(absPath) {
			return handleGoSymbolsOverview(absPath)
		}
		if dartast.HasDartFiles(absPath) {
			return handleDartSymbolsOverview(absPath)
		}
		return pyHandler(rawArgs)
	}
}

// --- Go handlers ---

func handleGoFindSymbol(name, absPath, rootDir string) (any, error) {
	symbols, err := goast.FindSymbolByName(name, absPath)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(symbols))
	for _, s := range symbols {
		m := map[string]any{
			"name":        s.Name,
			"type":        s.Kind,
			"line":        s.Line,
			"end_line":    s.EndLine,
			"column":      s.Column,
			"module_path": s.FilePath,
			"full_name":   s.FullName,
			"description": s.Description,
			"parent_type": s.ParentType,
			"parent_name": s.ParentName,
		}
		if s.Receiver != "" {
			m["receiver"] = s.Receiver
		}
		if s.Signature != "" {
			m["signature"] = s.Signature
		}
		if s.Doc != "" {
			m["docstring"] = s.Doc
		}
		m["_edit_hint"] = "Go file: use Edit tool for modifications (c4_replace_symbol_body not supported)"
		results = append(results, m)
	}

	relPath := absPath
	if rel, err := filepath.Rel(rootDir, absPath); err == nil {
		relPath = rel
	}

	return map[string]any{
		"success":       true,
		"pattern":       name,
		"relative_path": relPath,
		"symbols":       results,
		"count":         len(results),
		"_edit_hint":    "Go file: use Edit tool for modifications (c4_replace_symbol_body not supported)",
	}, nil
}

func handleGoSymbolsOverview(absPath string) (any, error) {
	result, err := goast.SymbolsOverview(absPath)
	if err != nil {
		return nil, err
	}
	result["success"] = true
	hint := "Go file: use Edit tool for modifications (c4_replace_symbol_body not supported)"
	result["_edit_hint"] = hint
	for _, key := range []string{"functions", "methods", "structs", "interfaces", "types", "constants", "variables"} {
		if items, ok := result[key].([]map[string]any); ok {
			for _, item := range items {
				item["_edit_hint"] = hint
			}
		}
	}
	return result, nil
}

// --- Dart handlers ---

func handleDartFindSymbol(name, absPath, rootDir string) (any, error) {
	symbols, err := dartast.FindSymbolByName(name, absPath)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(symbols))
	for _, s := range symbols {
		m := map[string]any{
			"name":        s.Name,
			"type":        s.Kind,
			"line":        s.Line,
			"end_line":    s.EndLine,
			"column":      s.Column,
			"module_path": s.FilePath,
			"parent_type": s.ParentType,
			"parent_name": s.ParentName,
		}
		if s.ParentName != "" {
			m["full_name"] = fmt.Sprintf("%s.%s", s.ParentName, s.Name)
		} else {
			m["full_name"] = s.Name
		}
		m["description"] = s.Signature
		if s.Signature != "" {
			m["signature"] = s.Signature
		}
		if s.Docstring != "" {
			m["docstring"] = s.Docstring
		}
		m["_edit_hint"] = "Dart file: use Edit tool for modifications (c4_replace_symbol_body not supported)"
		results = append(results, m)
	}

	relPath := absPath
	if rel, err := filepath.Rel(rootDir, absPath); err == nil {
		relPath = rel
	}

	return map[string]any{
		"success":       true,
		"pattern":       name,
		"relative_path": relPath,
		"symbols":       results,
		"count":         len(results),
		"_edit_hint":    "Dart file: use Edit tool for modifications (c4_replace_symbol_body not supported)",
	}, nil
}

func handleDartSymbolsOverview(absPath string) (any, error) {
	result, err := dartast.SymbolsOverview(absPath)
	if err != nil {
		return nil, err
	}
	result["success"] = true
	hint := "Dart file: use Edit tool for modifications (c4_replace_symbol_body not supported)"
	result["_edit_hint"] = hint
	for _, key := range []string{"classes", "enums", "typedefs", "functions"} {
		if items, ok := result[key].([]map[string]any); ok {
			for _, item := range items {
				item["_edit_hint"] = hint
			}
		}
	}
	return result, nil
}
