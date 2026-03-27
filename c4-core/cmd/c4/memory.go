package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/memory"
	"github.com/spf13/cobra"
)

var memorySource string

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage CQ memory (conversation import)",
}

var memoryImportCmd = &cobra.Command{
	Use:   "import [file.zip]",
	Short: "Import AI conversation sessions into knowledge store",
	Long: `Import AI conversation sessions from export files or local session directories.

Examples:
  cq memory import conversations.zip          # import from ChatGPT/Claude export ZIP
  cq memory import --source claude-code       # scan local Claude Code sessions
  cq memory import --source codex             # scan local Codex sessions`,
	RunE: runMemoryImport,
}

func init() {
	memoryImportCmd.Flags().StringVar(&memorySource, "source", "", "scan local sessions: claude-code, codex")
	memoryCmd.AddCommand(memoryImportCmd)
	rootCmd.AddCommand(memoryCmd)
}

func runMemoryImport(_ *cobra.Command, args []string) error {
	// Open knowledge store.
	knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		return fmt.Errorf("create knowledge dir: %w", err)
	}
	store, err := knowledge.NewStore(knowledgeDir)
	if err != nil {
		return fmt.Errorf("open knowledge store: %w", err)
	}
	defer store.Close()

	// Build importer without LLM (summarizer is optional).
	// LLM gateway requires serve mode; for CLI import, use fallback text.
	imp := &memory.Importer{
		Store:         store,
		Summarizer:    nil, // no LLM in CLI mode — fallback to raw text
		MaxConcurrent: 2,
	}

	var sessions []memory.Session

	if memorySource != "" {
		// Scan local session directories.
		var scanErr error
		sessions, scanErr = scanLocalSessions(memorySource)
		if scanErr != nil {
			return scanErr
		}
	} else if len(args) > 0 {
		// Import from ZIP file.
		zipPath := args[0]
		var zipErr error
		sessions, zipErr = parseZIPExport(zipPath)
		if zipErr != nil {
			return fmt.Errorf("parse ZIP: %w", zipErr)
		}
	} else {
		return fmt.Errorf("specify a ZIP file or --source flag")
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found to import.")
		return nil
	}

	fmt.Printf("Found %d sessions to import.\n", len(sessions))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := imp.ImportSessions(ctx, sessions)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Printf("\nImport complete: %d imported, %d skipped (dedup), %d errors out of %d total.\n",
		result.Imported, result.Skipped, len(result.Errors), result.Total)

	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "  error: session %s: %v\n", e.SessionID, e.Err)
	}

	return nil
}

// scanLocalSessions scans well-known local directories for AI session files.
func scanLocalSessions(source string) ([]memory.Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	var pattern string
	switch source {
	case "claude-code":
		pattern = filepath.Join(homeDir, ".claude", "projects", "**", "sessions", "*.jsonl")
	case "codex":
		pattern = filepath.Join(homeDir, ".codex", "sessions", "**", "*.json")
	default:
		return nil, fmt.Errorf("unknown source %q (supported: claude-code, codex)", source)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		// Glob doesn't support ** natively in Go; try a manual walk.
		matches, err = walkForSessions(source, homeDir)
		if err != nil {
			return nil, fmt.Errorf("scan sessions: %w", err)
		}
	}

	if len(matches) == 0 {
		// Try manual walk as fallback.
		matches, _ = walkForSessions(source, homeDir)
	}

	var sessions []memory.Session
	for _, path := range matches {
		sess, err := parseSessionFile(path, source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			continue
		}
		sessions = append(sessions, sess...)
	}
	return sessions, nil
}

// walkForSessions manually walks directories to find session files.
func walkForSessions(source, homeDir string) ([]string, error) {
	var baseDir string
	var ext string
	switch source {
	case "claude-code":
		baseDir = filepath.Join(homeDir, ".claude", "projects")
		ext = ".jsonl"
	case "codex":
		baseDir = filepath.Join(homeDir, ".codex", "sessions")
		ext = ".json"
	default:
		return nil, nil
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, nil
	}

	var matches []string
	filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ext) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, nil
}

// parseSessionFile reads a session file and returns parsed sessions.
func parseSessionFile(path, source string) ([]memory.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	// Derive session ID from file path.
	sessID := filepath.Base(path)
	sessID = strings.TrimSuffix(sessID, filepath.Ext(sessID))

	// Derive project from path.
	project := extractProjectFromPath(path, source)

	info, _ := os.Stat(path)
	startedAt := time.Now()
	if info != nil {
		startedAt = info.ModTime()
	}

	var turns []memory.Turn

	switch source {
	case "claude-code":
		turns = parseJSONLTurns(data)
	case "codex":
		turns = parseJSONTurns(data)
	default:
		turns = parseJSONLTurns(data)
	}

	if len(turns) == 0 {
		return nil, nil
	}

	return []memory.Session{{
		ID:        sessID,
		Source:    source,
		Project:   project,
		StartedAt: startedAt,
		Turns:     turns,
	}}, nil
}

// parseJSONLTurns parses JSONL format (one JSON object per line).
func parseJSONLTurns(data []byte) []memory.Turn {
	var turns []memory.Turn
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
			Message *struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		role := entry.Role
		var content any = entry.Content
		if entry.Message != nil {
			if entry.Message.Role != "" {
				role = entry.Message.Role
			}
			content = entry.Message.Content
		}

		if role != "user" && role != "assistant" {
			continue
		}
		text := extractTextContent(content)
		if text == "" {
			continue
		}
		turns = append(turns, memory.Turn{Role: role, Content: text})
	}
	return turns
}

// parseJSONTurns parses a JSON array of messages.
func parseJSONTurns(data []byte) []memory.Turn {
	// Try array of messages first.
	var messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &messages); err == nil {
		var turns []memory.Turn
		for _, m := range messages {
			if m.Role != "user" && m.Role != "assistant" {
				continue
			}
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			turns = append(turns, memory.Turn{Role: m.Role, Content: m.Content})
		}
		return turns
	}

	// Try object with messages field.
	var obj struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && len(obj.Messages) > 0 {
		var turns []memory.Turn
		for _, m := range obj.Messages {
			if m.Role != "user" && m.Role != "assistant" {
				continue
			}
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			turns = append(turns, memory.Turn{Role: m.Role, Content: m.Content})
		}
		return turns
	}

	return nil
}

// extractTextContent extracts plain text from a content field (string or []block).
func extractTextContent(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					parts = append(parts, strings.TrimSpace(t))
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// extractProjectFromPath tries to derive a project name from the file path.
func extractProjectFromPath(path, source string) string {
	switch source {
	case "claude-code":
		// Path: ~/.claude/projects/<project-hash>/sessions/<file>.jsonl
		dir := filepath.Dir(path)       // .../sessions
		dir = filepath.Dir(dir)          // .../project-hash
		return filepath.Base(dir)
	case "codex":
		dir := filepath.Dir(path)
		return filepath.Base(dir)
	}
	return "unknown"
}

// parseZIPExport extracts and parses conversations from a ZIP export file.
func parseZIPExport(zipPath string) ([]memory.Session, error) {
	zipPath = filepath.Clean(zipPath)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	var sessions []memory.Session

	for _, f := range r.File {
		name := f.Name
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 50<<20)) // 50 MiB limit per file
		rc.Close()
		if err != nil {
			continue
		}

		sessID := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		startedAt := f.Modified
		if startedAt.IsZero() {
			startedAt = time.Now()
		}

		// Try to parse as conversations.json (ChatGPT format).
		if strings.Contains(name, "conversations") {
			parsed := parseChatGPTConversations(data, name)
			sessions = append(sessions, parsed...)
			continue
		}

		// Generic: try JSONL then JSON.
		turns := parseJSONLTurns(data)
		if len(turns) == 0 {
			turns = parseJSONTurns(data)
		}
		if len(turns) == 0 {
			continue
		}

		sessions = append(sessions, memory.Session{
			ID:        sessID,
			Source:    "zip-import",
			Project:   "imported",
			StartedAt: startedAt,
			Turns:     turns,
		})
	}

	return sessions, nil
}

// parseChatGPTConversations handles the ChatGPT conversations.json format.
func parseChatGPTConversations(data []byte, filename string) []memory.Session {
	var convs []struct {
		ID      string         `json:"id"`
		Title   string         `json:"title"`
		Mapping map[string]any `json:"mapping"`
	}
	if err := json.Unmarshal(data, &convs); err != nil {
		return nil
	}

	var sessions []memory.Session
	for _, c := range convs {
		var turns []memory.Turn
		// ChatGPT mapping is a tree; extract messages in order.
		for _, node := range c.Mapping {
			nodeMap, ok := node.(map[string]any)
			if !ok {
				continue
			}
			msg, ok := nodeMap["message"].(map[string]any)
			if !ok {
				continue
			}
			author, _ := msg["author"].(map[string]any)
			role, _ := author["role"].(string)
			if role != "user" && role != "assistant" {
				continue
			}
			contentMap, _ := msg["content"].(map[string]any)
			parts, _ := contentMap["parts"].([]any)
			var texts []string
			for _, p := range parts {
				if s, ok := p.(string); ok && strings.TrimSpace(s) != "" {
					texts = append(texts, s)
				}
			}
			if len(texts) > 0 {
				turns = append(turns, memory.Turn{
					Role:    role,
					Content: strings.Join(texts, "\n"),
				})
			}
		}
		if len(turns) > 0 {
			title := c.Title
			if title == "" {
				title = "chatgpt-conversation"
			}
			sessions = append(sessions, memory.Session{
				ID:      c.ID,
				Source:  "chatgpt",
				Project: title,
				Turns:   turns,
			})
		}
	}
	return sessions
}
