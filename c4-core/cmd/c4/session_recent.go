package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var sessionRecentCmd = &cobra.Command{
	Use:   "recent",
	Short: "Show recent summarized session contexts for a project",
	Long: `Query and display recent summarized session summaries for the given project.
Useful for injecting past session context into a new session.

Examples:
  cq session recent --project myapp --limit 3
  cq session recent --project cq`,
	RunE: runSessionRecent,
}

var (
	sessionRecentProject string
	sessionRecentLimit   int
)

func init() {
	sessionRecentCmd.Flags().StringVar(&sessionRecentProject, "project", "", "project name to filter sessions")
	sessionRecentCmd.Flags().IntVar(&sessionRecentLimit, "limit", 3, "maximum number of recent sessions to show")
	sessionCmd.AddCommand(sessionRecentCmd)
}

type recentSession struct {
	ID          string
	Tool        string
	SummarizedAt string
	SummaryDocID string
}

func runSessionRecent(cmd *cobra.Command, args []string) error {
	db, err := openDB()
	if err != nil {
		// Silently exit if DB not available (hook context)
		return nil
	}
	defer db.Close()

	sessions, err := queryRecentSummarizedSessions(db, sessionRecentProject, sessionRecentLimit)
	if err != nil {
		// Silently exit on error (hook context — must not block session start)
		return nil
	}

	if len(sessions) == 0 {
		return nil
	}

	// Determine knowledge docs directory (relative to projectDir)
	knowledgeDocsDir := filepath.Join(c4Dir(), "knowledge", "docs")

	fmt.Println("## 최근 세션 컨텍스트")
	for _, s := range sessions {
		// Format timestamp
		ts := formatSessionTimestamp(s.SummarizedAt)
		fmt.Printf("\n### [%s] %s\n", ts, s.Tool)

		// Read summary from knowledge doc if available
		if s.SummaryDocID != "" {
			body, readErr := readKnowledgeDocBody(knowledgeDocsDir, s.SummaryDocID)
			if readErr == nil && body != "" {
				fmt.Println(body)
				continue
			}
		}
		fmt.Println("(요약 없음)")
	}

	return nil
}

func queryRecentSummarizedSessions(db *sql.DB, project string, limit int) ([]recentSession, error) {
	query := `
		SELECT session_id, tool, summarized_at, COALESCE(summary_doc_id, '')
		FROM sessions
		WHERE summarized_at IS NOT NULL
		  AND summarized_at != ''
	`
	var queryArgs []any

	if project != "" {
		query += " AND project = ?"
		queryArgs = append(queryArgs, project)
	}

	query += " ORDER BY summarized_at DESC LIMIT ?"
	queryArgs = append(queryArgs, limit)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []recentSession
	for rows.Next() {
		var s recentSession
		if err := rows.Scan(&s.ID, &s.Tool, &s.SummarizedAt, &s.SummaryDocID); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// readKnowledgeDocBody reads a knowledge document markdown file and returns
// only the body (everything after the YAML frontmatter).
func readKnowledgeDocBody(docsDir, docID string) (string, error) {
	filePath := filepath.Join(docsDir, docID+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	content := string(data)

	// Strip YAML frontmatter (--- ... ---)
	if strings.HasPrefix(content, "---") {
		// Find the closing ---
		rest := content[3:]
		idx := strings.Index(rest, "\n---")
		if idx >= 0 {
			// Skip past the closing ---\n
			body := rest[idx+4:]
			body = strings.TrimSpace(body)
			return body, nil
		}
	}

	return strings.TrimSpace(content), nil
}

// formatSessionTimestamp parses a timestamp string and formats it for display.
func formatSessionTimestamp(ts string) string {
	if ts == "" {
		return "unknown"
	}

	// Try RFC3339 first
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		t, err := time.Parse(layout, ts)
		if err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
	}

	// Return as-is if parsing fails
	return ts
}
