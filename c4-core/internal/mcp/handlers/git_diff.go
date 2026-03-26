package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const gitDiffResourceURI = "ui://cq/git-diff"

// GitDiffDeps holds optional dependencies for the git diff handler.
type GitDiffDeps struct {
	ResourceStore *apps.ResourceStore
	GitDiffHTML   string
	ProjectRoot   string
}

// RegisterGitDiffHandler registers the c4_diff_summary MCP tool and, if deps.ResourceStore
// is non-nil, registers the git diff HTML at ui://cq/git-diff.
func RegisterGitDiffHandler(reg *mcp.Registry, deps *GitDiffDeps) {
	if deps == nil {
		deps = &GitDiffDeps{}
	}

	if deps.ResourceStore != nil && deps.GitDiffHTML != "" {
		deps.ResourceStore.Register(gitDiffResourceURI, deps.GitDiffHTML)
	}

	reg.Register(mcp.ToolSchema{
		Name:        "cq_diff_summary",
		Description: "Summarize git diff — changed files with +/- line counts, status (M/A/D/R), and test-coverage warnings",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget with _meta; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
				"ref": map[string]any{
					"type":        "string",
					"description": "Git ref to diff against (default: HEAD, compares staged+unstaged changes; use 'HEAD~1' or a branch name)",
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Format string `json:"format"`
			Ref    string `json:"ref"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		return handleGitDiff(deps, args.Format, args.Ref)
	})
}

// GitDiffSummary is the structured result returned by parseDiff.
type GitDiffSummary struct {
	Branch       string        `json:"branch"`
	TotalAdded   int           `json:"total_added"`
	TotalDeleted int           `json:"total_deleted"`
	Files        []GitDiffFile `json:"files"`
}

// GitDiffFile represents a single changed file in the diff.
type GitDiffFile struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Added         int    `json:"added"`
	Deleted       int    `json:"deleted"`
	NoTestWarning bool   `json:"no_test_warning,omitempty"`
}

// handleGitDiff collects diff data and returns either a widget or text response.
func handleGitDiff(deps *GitDiffDeps, format, ref string) (any, error) {
	root := deps.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	summary, err := collectDiffSummary(root, ref)
	if err != nil {
		return nil, err
	}

	if format == "widget" {
		return map[string]any{
			"data": summary,
			"_meta": map[string]any{
				"ui": map[string]any{
					"resourceUri": gitDiffResourceURI,
				},
			},
		}, nil
	}

	return summary, nil
}

// collectDiffSummary runs git commands to build a GitDiffSummary.
func collectDiffSummary(root, ref string) (*GitDiffSummary, error) {
	// Determine current branch name (best-effort).
	branch, _ := runGit(root, "rev-parse", "--abbrev-ref", "HEAD")

	// Build the diff command.
	// When ref is empty we diff the working tree + index against HEAD.
	// When ref is provided (e.g. "HEAD~1" or "main") we diff committed changes.
	var namestatOut, numstatOut string
	var nsErr, numErr error

	if ref == "" || ref == "HEAD" {
		// staged + unstaged changes against HEAD
		namestatOut, nsErr = runGit(root, "diff", "HEAD", "--name-status")
		numstatOut, numErr = runGit(root, "diff", "HEAD", "--numstat")
	} else {
		namestatOut, nsErr = runGit(root, "diff", ref, "--name-status")
		numstatOut, numErr = runGit(root, "diff", ref, "--numstat")
	}

	// Treat git errors as "no changes" so the widget renders cleanly.
	if nsErr != nil || numErr != nil {
		return &GitDiffSummary{Branch: branch, Files: []GitDiffFile{}}, nil
	}

	files := mergeGitDiffOutputs(namestatOut, numstatOut)

	var totalAdd, totalDel int
	for i := range files {
		totalAdd += files[i].Added
		totalDel += files[i].Deleted
	}

	return &GitDiffSummary{
		Branch:       branch,
		TotalAdded:   totalAdd,
		TotalDeleted: totalDel,
		Files:        files,
	}, nil
}

// mergeGitDiffOutputs combines --name-status and --numstat output into GitDiffFile entries.
// --name-status lines: "<status>\t<path>" (or "<status>\t<old>\t<new>" for renames)
// --numstat lines:     "<added>\t<deleted>\t<path>"
func mergeGitDiffOutputs(namestat, numstat string) []GitDiffFile {
	// Parse name-status: map path -> status
	statusMap := map[string]string{}
	for _, line := range splitLines(namestat) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		var path string
		if len(parts) == 3 {
			// rename: use new path
			path = strings.TrimSpace(parts[2])
		} else {
			path = strings.TrimSpace(parts[1])
		}
		if path != "" {
			statusMap[path] = status
		}
	}

	// Parse numstat: build ordered list
	var files []GitDiffFile
	for _, line := range splitLines(numstat) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		addStr := strings.TrimSpace(parts[0])
		delStr := strings.TrimSpace(parts[1])
		path := strings.TrimSpace(parts[2])
		if path == "" {
			continue
		}

		// Binary files are represented as "-"
		add := parseIntOrZero(addStr)
		del := parseIntOrZero(delStr)

		status := statusMap[path]
		if status == "" {
			status = "M"
		}

		files = append(files, GitDiffFile{
			Name:          path,
			Status:        status,
			Added:         add,
			Deleted:       del,
			NoTestWarning: isCodeFileWithoutTest(path),
		})
	}

	return files
}

// isCodeFileWithoutTest returns true when the file is a source code file that is
// itself not a test file. This is used to flag files that may lack test coverage.
func isCodeFileWithoutTest(path string) bool {
	lower := strings.ToLower(path)

	// Skip test files themselves.
	if strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".spec.ts") ||
		strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, ".spec.js") ||
		strings.HasSuffix(lower, "_test.py") ||
		strings.Contains(lower, "test_") {
		return false
	}

	// Flag known source extensions.
	codeExts := []string{".go", ".py", ".ts", ".js", ".rs", ".java", ".cpp", ".c"}
	for _, ext := range codeExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func parseIntOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
