package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/spf13/cobra"
)

var (
	sessionCloseID        string
	sessionCloseName      string
	sessionCloseDir       string
	sessionCloseSkipPersona bool
)

// sessionCloseSummarizeFn is the LLM summarization function.
// Overridden in session_close_llm.go when llm_gateway build tag is set.
var sessionCloseSummarizeFn func(jsonlPath, project, date string) *sessionCloseResult

// sessionCloseResult holds structured output from LLM summarization.
type sessionCloseResult struct {
	Summary     string   `json:"summary"`
	Decisions   []string `json:"decisions"`
	Preferences []string `json:"preferences"`
}

var sessionCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close a session: status→done + summarize + knowledge + persona",
	Long: `Close a session and run the full close pipeline:
1. Set named session status to "done"
2. Generate LLM summary from JSONL transcript
3. Save summary to knowledge store
4. Extract decisions/preferences for persona learning

Examples:
  cq session close --session-id abc123
  cq session close --name relay-notify
  cq session close --session-id abc123 --skip-persona`,
	RunE: runSessionClose,
}

func init() {
	sessionCloseCmd.Flags().StringVar(&sessionCloseID, "session-id", "", "session UUID")
	sessionCloseCmd.Flags().StringVar(&sessionCloseName, "name", "", "named session tag")
	sessionCloseCmd.Flags().StringVar(&sessionCloseDir, "dir", "", "project directory")
	sessionCloseCmd.Flags().BoolVar(&sessionCloseSkipPersona, "skip-persona", false, "skip persona learning")
	sessionCmd.AddCommand(sessionCloseCmd)
}

func runSessionClose(cmd *cobra.Command, args []string) error {
	// --- 1. Resolve session info ---
	// Ensure projectDir is set for LLM gateway (cloud auth resolution)
	if sessionCloseDir != "" && projectDir == "" {
		projectDir = sessionCloseDir
	}

	var sessionID, jsonlPath, project string

	if sessionCloseID != "" {
		sessionID = sessionCloseID
	}

	// Try to find session in DB (global fallback for non-C4 projects)
	db, err := openSessionDB()
	if err == nil {
		defer db.Close()
		if sessionID != "" {
			var jp, proj sql.NullString
			err := db.QueryRow(
				"SELECT jsonl_path, project FROM sessions WHERE session_id = ?", sessionID,
			).Scan(&jp, &proj)
			if err == nil {
				jsonlPath = jp.String
				project = proj.String
			}
		}
	}

	// --- 2. Update named session status → done ---
	namedSessions, err := loadNamedSessions()
	if err == nil {
		updated := false
		for tag, entry := range namedSessions {
			match := false
			if sessionCloseName != "" && tag == sessionCloseName {
				match = true
			} else if sessionID != "" && entry.UUID == sessionID {
				match = true
			}
			if match {
				if entry.Status == "done" {
					fmt.Fprintf(os.Stderr, "cq: session %q already done, skipping\n", tag)
					return nil // idempotent (R5)
				}
				entry.Status = "done"
				entry.Updated = time.Now().UTC().Format(time.RFC3339)
				namedSessions[tag] = entry
				updated = true

				// Use session UUID from named entry if not provided
				if sessionID == "" {
					sessionID = entry.UUID
				}
				if project == "" {
					project = filepath.Base(entry.Dir)
				}
				break
			}
		}
		if updated {
			if err := saveNamedSessions(namedSessions); err != nil {
				fmt.Fprintf(os.Stderr, "cq: failed to save named sessions: %v\n", err)
			}
		}
	}

	// --- 3. Find JSONL path if not from DB ---
	if jsonlPath == "" && sessionID != "" {
		projectDir := sessionCloseDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}
		jsonlPath = findJSONLPath(sessionID, projectDir)
	}

	if jsonlPath == "" || sessionID == "" {
		fmt.Fprintf(os.Stderr, "cq: no JSONL transcript found for session, done status set\n")
		return nil
	}

	// --- 4. LLM summarization ---
	date := time.Now().Format("2006-01-02")
	var result *sessionCloseResult

	if sessionCloseSummarizeFn != nil {
		result = sessionCloseSummarizeFn(jsonlPath, project, date)
	}

	if result == nil || result.Summary == "" {
		// LLM unavailable — save metadata-only knowledge so session existence is recorded.
		// Background summarizer will enrich with full summary later.
		fmt.Fprintf(os.Stderr, "cq: LLM unavailable, saving metadata knowledge + deferring summary\n")
		knowledgeDir := resolveKnowledgeDir(sessionCloseDir, jsonlPath)
		if ks, err := knowledge.NewStore(knowledgeDir); err == nil {
			title := fmt.Sprintf("세션: %s (%s)", project, date)
			metaContent := fmt.Sprintf("session_id: %s\ntool: claude-code\nproject: %s\njsonl_path: %s\nstatus: unsummarized",
				sessionID, project, jsonlPath)
			meta := map[string]any{
				"title":  title,
				"domain": "session",
				"tags":   []string{"session", "unsummarized"},
			}
			if id, err := ks.Create(knowledge.TypeInsight, meta, metaContent); err == nil {
				// Mark in sessions DB with doc_id but no summarized_at
				if db != nil {
					db.Exec("UPDATE sessions SET summary_doc_id = ? WHERE session_id = ?", id, sessionID)
				}
			}
		}
		return nil
	}

	// --- 5. Save to knowledge store ---
	knowledgeDir := resolveKnowledgeDir(sessionCloseDir, jsonlPath)

	var docID string
	if ks, err := knowledge.NewStore(knowledgeDir); err == nil {
		title := fmt.Sprintf("세션 요약: %s (%s)", project, date)
		meta := map[string]any{
			"title":  title,
			"domain": "session",
			"tags":   []string{"session", "auto-close"},
		}
		if id, err := ks.Create(knowledge.TypeInsight, meta, result.Summary); err == nil {
			docID = id
		} else {
			fmt.Fprintf(os.Stderr, "cq: knowledge store failed: %v\n", err)
		}
	}

	// --- 6. Persona learning + Growth Loop ---
	if !sessionCloseSkipPersona {
		captureSessionLearnPersona(sessionCloseDir, result)
	}

	// --- 7. Mark summarized in sessions DB ---
	if db != nil && sessionID != "" && docID != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		db.Exec("UPDATE sessions SET summarized_at = ?, summary_doc_id = ? WHERE session_id = ?",
			now, docID, sessionID)
	}

	// Output result
	out, _ := json.Marshal(map[string]any{
		"session_id":  sessionID,
		"status":      "done",
		"summary":     result.Summary,
		"decisions":   result.Decisions,
		"preferences": result.Preferences,
		"doc_id":      docID,
	})
	fmt.Println(string(out))
	return nil
}

// findJSONLPath searches for a session's JSONL transcript file.
func findJSONLPath(sessionID, projectDir string) string {
	homeDir, _ := os.UserHomeDir()

	// Walk from projectDir up to home
	checkDir := projectDir
	for checkDir != "/" && checkDir != homeDir {
		encoded := strings.ReplaceAll(checkDir, string(os.PathSeparator), "-")
		candidate := filepath.Join(homeDir, ".claude", "projects", encoded, sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		checkDir = filepath.Dir(checkDir)
	}
	return ""
}

// applySessionPersona writes decisions/preferences to the user's Soul Learned section.
// Best-effort: failures are logged, not fatal.
func applySessionPersona(projectDir string, suggestions []string) {
	if len(suggestions) == 0 {
		return
	}

	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	// Read team.yaml for username
	username := readTeamUsername(projectDir)
	if username == "" {
		username = "default"
	}

	soulDir := filepath.Join(projectDir, ".c4", "souls", username)
	soulPath := filepath.Join(soulDir, "soul-session.md")

	existing, err := os.ReadFile(soulPath)
	var content string
	if err != nil {
		if err := os.MkdirAll(soulDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "cq: persona dir create failed: %v\n", err)
			return
		}
		content = fmt.Sprintf("# Soul: %s/session\n\n## Learned\n", username)
	} else {
		content = string(existing)
	}

	// Append new suggestions with dedup
	date := time.Now().Format("2006-01-02")
	existingSet := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			if idx := strings.Index(trimmed, "] "); idx >= 0 {
				existingSet[strings.TrimSpace(trimmed[idx+2:])] = true
			}
		}
	}

	var newLines []string
	for _, s := range suggestions {
		if !existingSet[s] {
			newLines = append(newLines, fmt.Sprintf("- [%s] %s", date, s))
		}
	}

	if len(newLines) == 0 {
		return
	}

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(newLines, "\n") + "\n"

	if err := os.WriteFile(soulPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cq: persona write failed: %v\n", err)
	}
}

// resolveKnowledgeDir picks the best knowledge directory:
// project .c4/knowledge if closeDir has .c4/, otherwise global ~/.c4/knowledge.
func resolveKnowledgeDir(closeDir, jsonlPath string) string {
	if closeDir != "" {
		if fi, err := os.Stat(filepath.Join(closeDir, ".c4")); err == nil && fi.IsDir() {
			return filepath.Join(closeDir, ".c4", "knowledge")
		}
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", "knowledge")
}

// readTeamUsername reads the active username from team.yaml.
func readTeamUsername(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, ".c4", "team.yaml"))
	if err != nil {
		return ""
	}
	// Simple extraction: find "active:" or first "- name:" line
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "active:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "active:"))
		}
	}
	return ""
}
