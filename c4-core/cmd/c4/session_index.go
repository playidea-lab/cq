package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var sessionIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index a session into the sessions table",
	Long: `Insert or update a session row in the sessions table of c4.db.

Examples:
  cq session index --session-id abc123 --tool claude-code --project myapp
  cq session index --session-id abc123 --tool codex --project myapp --jsonl-path ~/.claude/abc123.jsonl --turns 42`,
	RunE: runSessionIndex,
}

var (
	sessionIndexID      string
	sessionIndexTool    string
	sessionIndexProject string
	sessionIndexJSONL   string
	sessionIndexTurns   int
)

func init() {
	sessionIndexCmd.Flags().StringVar(&sessionIndexID, "session-id", "", "session ID (required)")
	sessionIndexCmd.Flags().StringVar(&sessionIndexTool, "tool", "", "tool name, e.g. claude-code, codex, gemini-cli (required)")
	sessionIndexCmd.Flags().StringVar(&sessionIndexProject, "project", "", "project name or path")
	sessionIndexCmd.Flags().StringVar(&sessionIndexJSONL, "jsonl-path", "", "path to the JSONL transcript file")
	sessionIndexCmd.Flags().IntVar(&sessionIndexTurns, "turns", 0, "number of conversation turns")
	_ = sessionIndexCmd.MarkFlagRequired("session-id")
	_ = sessionIndexCmd.MarkFlagRequired("tool")
	sessionCmd.AddCommand(sessionIndexCmd)
}

func runSessionIndex(cmd *cobra.Command, args []string) error {
	db, err := openSessionDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Ensure sessions table exists (idempotent)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		session_id    TEXT PRIMARY KEY,
		tool          TEXT NOT NULL,
		project       TEXT,
		jsonl_path    TEXT,
		turn_count    INTEGER DEFAULT 0,
		started_at    TIMESTAMP,
		ended_at      TIMESTAMP,
		summarized_at TIMESTAMP,
		summary_doc_id TEXT
	)`)
	if err != nil {
		return fmt.Errorf("failed to ensure sessions table: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.Exec(`
		INSERT INTO sessions (session_id, tool, project, jsonl_path, turn_count, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			tool       = excluded.tool,
			project    = excluded.project,
			jsonl_path = excluded.jsonl_path,
			turn_count = excluded.turn_count,
			ended_at   = excluded.ended_at
	`, sessionIndexID, sessionIndexTool, sessionIndexProject, sessionIndexJSONL, sessionIndexTurns, now, now)
	if err != nil {
		return fmt.Errorf("failed to upsert session: %w", err)
	}

	result := map[string]any{
		"session_id": sessionIndexID,
		"tool":       sessionIndexTool,
		"project":    sessionIndexProject,
		"jsonl_path": sessionIndexJSONL,
		"turn_count": sessionIndexTurns,
		"started_at": now,
		"ended_at":   now,
		"status":     "indexed",
	}
	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
