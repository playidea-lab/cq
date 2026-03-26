package gitops

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
)

// Register registers git-related tools.
// Pure Go implementation using git CLI.
func Register(reg *mcp.Registry, rootDir string) {
	// c4_worktree_status
	reg.Register(mcp.ToolSchema{
		Name:        "cq_worktree_status",
		Description: "Get git worktree and branch status",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleWorktreeStatus(rootDir)
	})

	// c4_worktree_cleanup
	reg.Register(mcp.ToolSchema{
		Name:        "cq_worktree_cleanup",
		Description: "Clean up stale git worktrees",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Only show what would be cleaned (default: true)",
					"default":     true,
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleWorktreeCleanup(rootDir, args)
	})

	// c4_analyze_history
	reg.Register(mcp.ToolSchema{
		Name:        "cq_analyze_history",
		Description: "Analyze git commit history for patterns",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"since": map[string]any{
					"type":        "string",
					"description": "Start date (e.g. '2 weeks ago', '2026-01-01')",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Filter by author",
				},
				"max_count": map[string]any{
					"type":        "integer",
					"description": "Max commits to analyze (default: 100)",
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleAnalyzeHistory(rootDir, args)
	})

	// c4_search_commits
	reg.Register(mcp.ToolSchema{
		Name:        "cq_search_commits",
		Description: "Search git commits by message or content",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query (matches commit messages)",
				},
				"max_count": map[string]any{
					"type":        "integer",
					"description": "Max results (default: 20)",
				},
			},
			"required": []string{"query"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSearchCommits(rootDir, args)
	})
}

// runGit executes a git command in rootDir and returns stdout.
func runGit(rootDir string, gitArgs ...string) (string, error) {
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", gitArgs[0], string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", gitArgs[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func handleWorktreeStatus(rootDir string) (any, error) {
	// Current branch
	branch, err := runGit(rootDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}

	// Status (short format)
	status, err := runGit(rootDir, "status", "--short")
	if err != nil {
		return nil, err
	}

	// Worktree list
	worktrees, err := runGit(rootDir, "worktree", "list", "--porcelain")
	if err != nil {
		worktrees = ""
	}

	// Count changes
	var modified, untracked, staged int
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		if len(line) >= 2 {
			if line[0] != ' ' && line[0] != '?' {
				staged++
			}
			if line[1] == 'M' || line[1] == 'D' {
				modified++
			}
			if line[0] == '?' {
				untracked++
			}
		}
	}

	return map[string]any{
		"branch":    branch,
		"modified":  modified,
		"untracked": untracked,
		"staged":    staged,
		"clean":     status == "",
		"worktrees": worktrees,
	}, nil
}

type worktreeCleanupArgs struct {
	DryRun bool `json:"dry_run"`
}

func handleWorktreeCleanup(rootDir string, rawArgs json.RawMessage) (any, error) {
	args := worktreeCleanupArgs{DryRun: false}
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
	}

	// List worktrees
	output, err := runGit(rootDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var stale []string
	for _, block := range strings.Split(output, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "prunable") {
				// Extract worktree path from preceding lines
				for _, l := range strings.Split(block, "\n") {
					if strings.HasPrefix(l, "worktree ") {
						stale = append(stale, strings.TrimPrefix(l, "worktree "))
						break
					}
				}
			}
		}
	}

	if !args.DryRun && len(stale) > 0 {
		_, err := runGit(rootDir, "worktree", "prune")
		if err != nil {
			return nil, err
		}
	}

	return map[string]any{
		"stale":   stale,
		"count":   len(stale),
		"dry_run": args.DryRun,
		"pruned":  !args.DryRun && len(stale) > 0,
	}, nil
}

type analyzeHistoryArgs struct {
	Since    string `json:"since"`
	Author   string `json:"author"`
	MaxCount int    `json:"max_count"`
}

func handleAnalyzeHistory(rootDir string, rawArgs json.RawMessage) (any, error) {
	args := analyzeHistoryArgs{MaxCount: 100}
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
	}

	gitArgs := []string{"log", "--pretty=format:%H|%an|%ad|%s", "--date=short"}
	if args.Since != "" {
		gitArgs = append(gitArgs, "--since="+args.Since)
	}
	if args.Author != "" {
		gitArgs = append(gitArgs, "--author="+args.Author)
	}
	gitArgs = append(gitArgs, fmt.Sprintf("-n%d", args.MaxCount))

	output, err := runGit(rootDir, gitArgs...)
	if err != nil {
		return nil, err
	}

	type commitInfo struct {
		SHA     string `json:"sha"`
		Author  string `json:"author"`
		Date    string `json:"date"`
		Message string `json:"message"`
	}

	var commits []commitInfo
	authors := make(map[string]int)

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, commitInfo{
			SHA:     parts[0][:8],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
		authors[parts[1]]++
	}

	return map[string]any{
		"commits":      commits,
		"total":        len(commits),
		"authors":      authors,
		"author_count": len(authors),
	}, nil
}

type searchCommitsArgs struct {
	Query    string `json:"query"`
	MaxCount int    `json:"max_count"`
}

func handleSearchCommits(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args searchCommitsArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	maxCount := args.MaxCount
	if maxCount <= 0 {
		maxCount = 20
	}

	output, err := runGit(rootDir, "log", "--grep="+args.Query, "-i",
		"--pretty=format:%H|%an|%ad|%s", "--date=short",
		fmt.Sprintf("-n%d", maxCount))
	if err != nil {
		return nil, err
	}

	type commitResult struct {
		SHA     string `json:"sha"`
		Author  string `json:"author"`
		Date    string `json:"date"`
		Message string `json:"message"`
	}

	var results []commitResult
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		results = append(results, commitResult{
			SHA:     parts[0][:8],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}

	return map[string]any{
		"matches": results,
		"count":   len(results),
		"query":   args.Query,
	}, nil
}
