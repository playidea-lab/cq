package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// matchIdeaByTag scans .c4/ideas/ in projectDir for an idea file whose name
// contains any of the "-"-split tokens of tag. Returns the matched slug (without .md)
// and its full path, or empty strings if no match found.
func matchIdeaByTag(tag string) (slug string, ideaPath string) {
	ideasDir := filepath.Join(projectDir, ".c4", "ideas")
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return "", ""
	}
	// Split tag by "-" to get tokens for fuzzy matching.
	tokens := strings.Split(strings.ToLower(tag), "-")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(e.Name(), ".md"))
		for _, tok := range tokens {
			if tok != "" && strings.Contains(name, tok) {
				slug = strings.TrimSuffix(e.Name(), ".md")
				ideaPath = filepath.Join(ideasDir, e.Name())
				return slug, ideaPath
			}
		}
	}
	return "", ""
}

// sessionContextCmd shows context for the named session: idea match, status-based suggestions.
var sessionContextCmd = &cobra.Command{
	Use:   "context <session-name>",
	Short: "Show context and suggestions for a named session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tag := args[0]
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}

		entry, ok := sessions[tag]
		if !ok {
			// Session not found — still try idea match.
			slug, ideaPath := matchIdeaByTag(tag)
			if slug != "" {
				fmt.Printf("💡 Idea matched: %s\n", ideaPath)
				fmt.Printf("   Start with: /plan --from-pi %s\n", slug)
			}
			return nil
		}

		// Show session info.
		statusColor := statusColorCode(entry.Status)
		reset := "\033[0m"
		fmt.Printf("📋 Session: %s  [%s%s%s]\n", tag, statusColor, statusDisplay(entry.Status), reset)
		if entry.Summary != "" {
			fmt.Printf("   %s\n", entry.Summary)
		}

		// Idea link: use stored Idea field first, then fuzzy-match by tag.
		ideaSlug := entry.Idea
		if ideaSlug == "" {
			ideaSlug, _ = matchIdeaByTag(tag)
		}
		if ideaSlug != "" {
			ideaPath := filepath.Join(projectDir, ".c4", "ideas", ideaSlug+".md")
			if _, statErr := os.Stat(ideaPath); statErr == nil {
				fmt.Printf("   ├─ 💡 %s\n", ideaPath)
			}
		}

		// Status-based suggestion.
		fmt.Println()
		switch entry.Status {
		case "idea", "":
			if ideaSlug != "" {
				fmt.Printf("👉 /plan --from-pi %s\n", ideaSlug)
			} else {
				fmt.Println("👉 아직 idea.md가 없습니다. /pi 로 아이디어를 구체화하세요.")
			}
		case "planned":
			fmt.Println("👉 /run 으로 실행하세요.")
		case "in-progress":
			fmt.Println("👉 /c4-status 로 진행 중 태스크를 확인하세요.")
		case "done":
			fmt.Println("✅ 완료됨. 새 작업을 시작하려면 /pi 를 실행하세요.")
		default:
			fmt.Printf("ℹ️  status: %s\n", entry.Status)
		}

		return nil
	},
}

// statusColorCode returns ANSI color for a session status.
func statusColorCode(status string) string {
	switch status {
	case "idea":
		return "\033[33m" // yellow
	case "planned":
		return "\033[34m" // blue
	case "in-progress":
		return "\033[32m" // green
	case "done":
		return "\033[90m" // gray
	default:
		return "" // white/default
	}
}

// statusDisplay returns a user-friendly display string for status.
func statusDisplay(status string) string {
	if status == "" {
		return "active"
	}
	return status
}
