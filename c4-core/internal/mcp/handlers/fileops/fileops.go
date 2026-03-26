package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/changmin/c4-core/internal/mcp"
)

// Register registers file operation tools.
// Pure Go implementation — no Python dependency.
func Register(reg *mcp.Registry, rootDir string) {
	// c4_find_file
	reg.Register(mcp.ToolSchema{
		Name:        "cq_find_file",
		Description: "Find files matching a glob pattern in the project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern (e.g. '**/*.py', 'src/**/*.go')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Base directory (default: project root)",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleFindFile(rootDir, args)
	})

	// c4_search_for_pattern
	reg.Register(mcp.ToolSchema{
		Name:        "cq_search_for_pattern",
		Description: "Search for a regex pattern in project files",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regex pattern to search for",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search in (default: project root)",
				},
				"file_pattern": map[string]any{
					"type":        "string",
					"description": "Glob to filter files (e.g. '*.py')",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 50)",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSearchForPattern(rootDir, args)
	})

	// c4_read_file
	reg.Register(mcp.ToolSchema{
		Name:        "cq_read_file",
		Description: "Read a file's contents with optional line range",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (relative to project root or absolute)",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "Start line (1-indexed, default: 1)",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "End line (inclusive, default: end of file)",
				},
			},
			"required": []string{"path"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleReadFile(rootDir, args)
	})

	// c4_replace_content
	reg.Register(mcp.ToolSchema{
		Name:        "cq_replace_content",
		Description: "Replace text content in a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path",
				},
				"old_text": map[string]any{
					"type":        "string",
					"description": "Text to find",
				},
				"new_text": map[string]any{
					"type":        "string",
					"description": "Replacement text",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences (default: first only)",
					"default":     false,
				},
			},
			"required": []string{"path", "old_text", "new_text"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleReplaceContent(rootDir, args)
	})

	// c4_create_text_file
	reg.Register(mcp.ToolSchema{
		Name:        "cq_create_text_file",
		Description: "Create a new text file with given content",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path to create",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "File content",
				},
			},
			"required": []string{"path", "content"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleCreateTextFile(rootDir, args)
	})

	// c4_list_dir
	reg.Register(mcp.ToolSchema{
		Name:        "cq_list_dir",
		Description: "List directory contents",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path (default: project root)",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "List recursively (default: false)",
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleListDir(rootDir, args)
	})
}

// resolvePath resolves a path relative to rootDir, preventing directory traversal.
func resolvePath(rootDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		cleaned := filepath.Clean(path)
		if !strings.HasPrefix(cleaned, filepath.Clean(rootDir)) {
			return "", fmt.Errorf("absolute path escapes project root: %s", path)
		}
		return cleaned, nil
	}
	resolved := filepath.Join(rootDir, path)
	resolved = filepath.Clean(resolved)
	if !strings.HasPrefix(resolved, filepath.Clean(rootDir)) {
		return "", fmt.Errorf("path escapes project root: %s", path)
	}
	return resolved, nil
}

type findFileArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func handleFindFile(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args findFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	baseDir := rootDir
	if args.Path != "" {
		var err error
		baseDir, err = resolvePath(rootDir, args.Path)
		if err != nil {
			return nil, err
		}
	}

	var matches []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "__pycache__" || base == ".c4" {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, _ := filepath.Rel(rootDir, path)
		if matchGlob(args.Pattern, filepath.Base(path), relPath) {
			matches = append(matches, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	return map[string]any{
		"matches": matches,
		"count":   len(matches),
	}, nil
}

// matchGlob matches a glob pattern against a filename and its relative path.
// Supports "**/" prefix for recursive matching (e.g., "**/*.py").
func matchGlob(pattern, name, relPath string) bool {
	if strings.HasPrefix(pattern, "**/") {
		sub := pattern[3:]
		if m, _ := filepath.Match(sub, name); m {
			return true
		}
		// Try matching against each path suffix
		for i := 0; i < len(relPath); i++ {
			if relPath[i] == filepath.Separator || relPath[i] == '/' {
				if m, _ := filepath.Match(sub, relPath[i+1:]); m {
					return true
				}
			}
		}
		return false
	}
	if m, _ := filepath.Match(pattern, name); m {
		return true
	}
	m, _ := filepath.Match(pattern, relPath)
	return m
}

type searchPatternArgs struct {
	Pattern     string `json:"pattern"`
	Path        string `json:"path"`
	FilePattern string `json:"file_pattern"`
	MaxResults  int    `json:"max_results"`
}

type searchMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func handleSearchForPattern(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args searchPatternArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	baseDir := rootDir
	if args.Path != "" {
		baseDir, err = resolvePath(rootDir, args.Path)
		if err != nil {
			return nil, err
		}
	}

	var (
		results []searchMatch
		mu      sync.Mutex
	)

	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "__pycache__" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if args.FilePattern != "" {
			matched, _ := filepath.Match(args.FilePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				mu.Lock()
				if len(results) >= maxResults {
					mu.Unlock()
					return fmt.Errorf("max results reached")
				}
				results = append(results, searchMatch{
					File:    relPath,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
				mu.Unlock()
			}
		}
		return nil
	})

	return map[string]any{
		"matches": results,
		"count":   len(results),
	}, nil
}

type readFileArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func handleReadFile(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args readFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	resolved, err := resolvePath(rootDir, args.Path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	start := 0
	end := len(lines)
	if args.StartLine > 0 {
		start = args.StartLine - 1
	}
	if args.EndLine > 0 && args.EndLine < end {
		end = args.EndLine
	}
	if start > len(lines) {
		start = len(lines)
	}
	if end > len(lines) {
		end = len(lines)
	}

	selected := lines[start:end]
	return map[string]any{
		"content":     strings.Join(selected, "\n"),
		"total_lines": len(lines),
		"start_line":  start + 1,
		"end_line":    end,
	}, nil
}

type replaceContentArgs struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all"`
}

func handleReplaceContent(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args replaceContentArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	resolved, err := resolvePath(rootDir, args.Path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	content := string(data)
	if !strings.Contains(content, args.OldText) {
		return map[string]any{
			"success": false,
			"error":   "old_text not found in file",
		}, nil
	}

	count := strings.Count(content, args.OldText)
	n := 1
	if args.ReplaceAll {
		n = -1
	}
	newContent := strings.Replace(content, args.OldText, args.NewText, n)
	if err := os.WriteFile(resolved, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	replacements := 1
	if args.ReplaceAll {
		replacements = count
	}
	return map[string]any{
		"success":      true,
		"replacements": replacements,
	}, nil
}

type createTextFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func handleCreateTextFile(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args createTextFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	resolved, err := resolvePath(rootDir, args.Path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return nil, fmt.Errorf("creating directories: %w", err)
	}

	if err := os.WriteFile(resolved, []byte(args.Content), 0644); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	return map[string]any{
		"success": true,
		"path":    args.Path,
		"size":    len(args.Content),
	}, nil
}

type listDirArgs struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type dirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func handleListDir(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args listDirArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
	}

	dir := rootDir
	if args.Path != "" {
		var err error
		dir, err = resolvePath(rootDir, args.Path)
		if err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var result []dirEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, dirEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}

	return map[string]any{
		"entries": result,
		"count":   len(result),
		"path":    args.Path,
	}, nil
}
