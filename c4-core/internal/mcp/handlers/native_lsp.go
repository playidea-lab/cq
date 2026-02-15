package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/dartast"
	"github.com/changmin/c4-core/internal/goast"
	"github.com/changmin/c4-core/internal/mcp"
)

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

		absPath := resolveLSPPath(params.Path, rootDir)
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

		absPath := resolveLSPPath(params.Path, rootDir)
		if goast.HasGoFiles(absPath) {
			return handleGoSymbolsOverview(absPath)
		}
		if dartast.HasDartFiles(absPath) {
			return handleDartSymbolsOverview(absPath)
		}
		return pyHandler(rawArgs)
	}
}

func resolveLSPPath(path, rootDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
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
	}, nil
}

func handleGoSymbolsOverview(absPath string) (any, error) {
	result, err := goast.SymbolsOverview(absPath)
	if err != nil {
		return nil, err
	}
	result["success"] = true
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
	}, nil
}

func handleDartSymbolsOverview(absPath string) (any, error) {
	result, err := dartast.SymbolsOverview(absPath)
	if err != nil {
		return nil, err
	}
	result["success"] = true
	return result, nil
}

// isRustPath checks if the path is a Rust file (not supported by native parsers).
func isRustPath(path string) bool {
	return strings.HasSuffix(path, ".rs")
}
