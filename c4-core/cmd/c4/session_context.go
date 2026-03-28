package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// matchIdeasByTag scans .c4/ideas/ in projectDir for idea files whose name
// contains any of the tag tokens (split by "-" or "_"). Tokens <= 3 chars are
// filtered out to reduce false positives. Returns deduplicated slug list.
func matchIdeasByTag(tag string) []string {
	ideasDir := filepath.Join(projectDir, ".c4", "ideas")
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return nil
	}
	// Split by both "-" and "_" (session tags use underscores).
	raw := strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool {
		return r == '-' || r == '_'
	})
	// Filter out tokens <= 3 chars to reduce false positives.
	var tokens []string
	for _, tok := range raw {
		if len(tok) > 3 {
			tokens = append(tokens, tok)
		}
	}
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var slugs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(e.Name(), ".md"))
		for _, tok := range tokens {
			if strings.Contains(name, tok) {
				slug := strings.TrimSuffix(e.Name(), ".md")
				if _, dup := seen[slug]; !dup {
					seen[slug] = struct{}{}
					slugs = append(slugs, slug)
				}
				break
			}
		}
	}
	return slugs
}

// matchIdeaByTag scans .c4/ideas/ in projectDir for an idea file whose name
// contains any of the tag tokens. Returns the first matched slug (without .md)
// and its full path, or empty strings if no match found.
func matchIdeaByTag(tag string) (slug string, ideaPath string) {
	slugs := matchIdeasByTag(tag)
	if len(slugs) == 0 {
		return "", ""
	}
	ideasDir := filepath.Join(projectDir, ".c4", "ideas")
	return slugs[0], filepath.Join(ideasDir, slugs[0]+".md")
}

// matchSessionsByIdea returns session tags whose Idea field matches slug (exact),
// or whose tag contains any long token from slug (fuzzy).
func matchSessionsByIdea(slug string) []string {
	sessions, err := loadNamedSessions()
	if err != nil {
		return nil
	}
	// Build fuzzy tokens from slug (split by "-", filter <= 3 chars).
	raw := strings.Split(strings.ToLower(slug), "-")
	var tokens []string
	for _, tok := range raw {
		if len(tok) > 3 {
			tokens = append(tokens, tok)
		}
	}
	var tags []string
	for tag, entry := range sessions {
		// Exact match on Idea field first.
		if entry.Idea == slug {
			tags = append(tags, tag)
			continue
		}
		// Fuzzy: check if any token appears in the session tag.
		tagLower := strings.ToLower(tag)
		for _, tok := range tokens {
			if strings.Contains(tagLower, tok) {
				tags = append(tags, tag)
				break
			}
		}
	}
	return tags
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

		// Show session info in context injection format.
		statusLabel := statusDisplay(entry.Status)
		fmt.Printf("📋 세션 \"%s\" 이전 맥락:\n", tag)
		fmt.Printf("  상태: %s\n", statusLabel)
		if entry.Summary != "" {
			fmt.Printf("  요약: %s\n", entry.Summary)
		}

		// Idea link: use stored Idea field first, then fuzzy-match by tag.
		ideaSlug := entry.Idea
		if ideaSlug == "" {
			ideaSlug, _ = matchIdeaByTag(tag)
		}
		if ideaSlug != "" {
			fmt.Printf("  아이디어: %s\n", ideaSlug)
		}

		// Status-based suggestion.
		fmt.Println()
		switch entry.Status {
		case "idea", "":
			fmt.Println("💡 /plan으로 계획을 시작하세요")
		case "planned":
			fmt.Println("💡 /run으로 실행을 시작하세요")
		case "in-progress", "active":
			fmt.Println("💡 /status로 진행 상황을 확인하세요")
		case "done":
			fmt.Println("✅ 완료된 세션입니다")
		default:
			fmt.Printf("💡 /status로 진행 상황을 확인하세요\n")
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
	case "active":
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
