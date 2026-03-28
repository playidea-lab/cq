package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	_ "modernc.org/sqlite"
)

// --- PID file management ---

func sessionPIDDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", "running")
}

func writeSessionPID(name string, pid int) {
	dir := sessionPIDDir()
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, name+".pid"), []byte(strconv.Itoa(pid)), 0600)
}

func removeSessionPID(name string) {
	os.Remove(filepath.Join(sessionPIDDir(), name+".pid"))
}

// isSessionRunning checks if a named session has a live PID file.
func isSessionRunning(name string) bool {
	data, err := os.ReadFile(filepath.Join(sessionPIDDir(), name+".pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	// Check if process is alive (signal 0 = existence check)
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// --- External AI tool process detection ---

// aiToolProcess represents a detected running AI tool process.
type aiToolProcess struct {
	tool string
	pid  int
}

// detectRunningAITools finds running AI CLI processes not managed by CQ.
func detectRunningAITools() []aiToolProcess {
	patterns := []struct {
		tool    string
		pgrep   string
		exclude string
	}{
		{"claude", "claude", "cq"},
		{"gemini", "gemini", ""},
		{"cursor", "Cursor", ""},
		{"codex", "codex", ""},
	}

	var results []aiToolProcess
	for _, p := range patterns {
		out, err := exec.Command("pgrep", "-af", p.pgrep).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			// Skip CQ-managed processes
			if p.exclude != "" && strings.Contains(line, p.exclude) {
				continue
			}
			// Skip this detection process itself
			if strings.Contains(line, "pgrep") {
				continue
			}
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				continue
			}
			pid, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			results = append(results, aiToolProcess{tool: p.tool, pid: pid})
		}
	}
	return results
}

// --- Real-time status recalculation ---

// recalcStatuses updates all session statuses in-place based on current file/DB state.
// Running sessions (PID alive) get "running" status.
func recalcStatuses(sessions map[string]namedSessionEntry) {
	for tag, entry := range sessions {
		if isSessionRunning(tag) {
			entry.Status = "running"
			sessions[tag] = entry
			continue
		}
		// Migrate legacy "in-progress" → "active"
		if entry.Status == "in-progress" {
			entry.Status = "active"
			sessions[tag] = entry
		}
		// Only recalc if idea is set (otherwise keep stored status)
		if entry.Idea != "" || entry.Dir != "" {
			newStatus := inferStatusQuiet(entry)
			if newStatus != "" {
				entry.Status = newStatus
				sessions[tag] = entry
			}
		}
	}
}

// inferStatusQuiet is like inferStatus but without interactive prompting.
// Returns empty string if status cannot be determined.
func inferStatusQuiet(entry namedSessionEntry) string {
	dir := entry.Dir
	idea := entry.Idea

	if idea != "" {
		ideaPath := filepath.Join(dir, ".c4", "ideas", idea+".md")
		if _, err := os.Stat(ideaPath); err == nil {
			specPath := filepath.Join(dir, "docs", "specs", idea+".md")
			if _, err := os.Stat(specPath); err != nil {
				return "idea"
			}
			// spec exists — check tasks
			done, total, dbErr := captureSessionCountTasks(dir, idea)
			if dbErr == nil {
				if total == 0 {
					return "planned"
				}
				if done == total {
					return "done"
				}
				return "active"
			}
			return "planned"
		}
	}

	return "" // can't determine — keep existing
}

// captureSessionSummarizeFn is set by the llm_gateway build variant.
// When nil, LLM summarization is skipped and summary remains empty.
var captureSessionSummarizeFn func(jsonlPath, project, date string) string

// captureSessionLight updates only the session's Updated timestamp without
// changing status, generating summaries, or saving knowledge/persona.
// Used for lightweight session touches (e.g., heartbeat, non-exit events).
func captureSessionLight(name string) {
	sessions, err := loadNamedSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSessionLight: load sessions: %v\n", err)
		return
	}

	entry, ok := sessions[name]
	if !ok {
		return
	}

	entry.Updated = time.Now().Format(time.RFC3339)
	sessions[name] = entry

	if err := saveNamedSessions(sessions); err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSessionLight: save sessions: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "cq: session '%s' → light capture\n", name)
}

// captureSession is an alias for captureSessionFull for backward compatibility.
func captureSession(name string) { captureSessionFull(name) }

// captureSessionFull closes the session on exit: status→done, LLM summary,
// knowledge store, and persona learning. Best-effort: errors are logged, not fatal.
func captureSessionFull(name string) {
	sessions, err := loadNamedSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: load sessions: %v\n", err)
		return
	}

	entry, ok := sessions[name]
	if !ok {
		return
	}

	// Exit always transitions to "done".
	status := "done"

	// Find session transcript using provider system.
	tool := entry.Tool
	if tool == "" {
		tool = "claude"
	}
	var transcriptPath string
	for _, p := range providers {
		if path := p.FindTranscript(entry.Dir, entry.UUID); path != "" {
			transcriptPath = path
			break
		}
	}

	// Generate structured summary + knowledge + persona (best-effort).
	summary := ""
	if transcriptPath != "" {
		project := filepath.Base(entry.Dir)
		date := time.Now().Format("2006-01-02")
		jsonlPath, isTmp := transcriptToJSONL(transcriptPath, tool)
		if isTmp {
			defer os.Remove(jsonlPath)
		}

		// Prefer structured close pipeline (summary + decisions + preferences)
		if jsonlPath != "" && sessionCloseSummarizeFn != nil {
			result := sessionCloseSummarizeFn(jsonlPath, project, date)
			if result != nil && result.Summary != "" {
				summary = result.Summary
				captureSessionSaveKnowledge(entry.Dir, project, date, result.Summary)
				captureSessionLearnPersona(entry.Dir, result)
			}
		} else if jsonlPath != "" && captureSessionSummarizeFn != nil {
			// Fallback: simple summary only (no knowledge/persona)
			summary = captureSessionSummarizeFn(jsonlPath, project, date)
		}
	}

	// Update the entry.
	entry.Status = status
	if summary != "" {
		entry.Summary = summary
	}
	entry.Updated = time.Now().Format(time.RFC3339)
	sessions[name] = entry

	if err := saveNamedSessions(sessions); err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: save sessions: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "cq: session '%s' → done\n", name)
}

// captureSessionLight captures minimal session state on normal exit (no /done).
// Only updates status→done and timestamp; skips LLM summarization.
func captureSessionLight(name string) {
	sessions, err := loadNamedSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSessionLight: load sessions: %v\n", err)
		return
	}
	entry, ok := sessions[name]
	if !ok {
		return
	}
	entry.Status = "done"
	entry.Updated = time.Now().Format(time.RFC3339)
	sessions[name] = entry
	if err := saveNamedSessions(sessions); err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSessionLight: save sessions: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "cq: session '%s' → done (light)\n", name)
}

// captureSessionFull captures full session state on /done exit.
// Runs LLM summarization, knowledge store, and persona learning (same as captureSession).
func captureSessionFull(name string) {
	captureSession(name)
}

// transcriptToJSONL converts a transcript to JSONL format for LLM processing.
// Returns (path, isTempFile). For native JSONL, returns the original path with isTmp=false.
func transcriptToJSONL(transcriptPath, tool string) (string, bool) {
	if strings.HasSuffix(transcriptPath, ".jsonl") {
		return transcriptPath, false
	}

	p := findProviderByTool(tool)
	if p == nil {
		p = findProviderByTool("gemini")
	}
	if p == nil {
		return "", false
	}

	items := p.LoadHistory(transcriptPath)
	if len(items) == 0 {
		return "", false
	}

	tmpFile, err := os.CreateTemp("", "cq-session-*.jsonl")
	if err != nil {
		return "", false
	}

	for _, msg := range items {
		row := map[string]any{"type": "user", "message": map[string]string{"role": "user", "content": msg}}
		data, _ := json.Marshal(row)
		tmpFile.Write(data)
		tmpFile.WriteString("\n")
	}
	tmpFile.Close()
	return tmpFile.Name(), true
}

// captureSessionSaveKnowledge saves a session summary to the knowledge store (best-effort).
func captureSessionSaveKnowledge(dir, project, date, summaryText string) {
	if summaryText == "" {
		return
	}
	homeDir, _ := os.UserHomeDir()
	knowledgeDir := filepath.Join(homeDir, ".c4", "knowledge")
	if dir != "" {
		if _, err := os.Stat(filepath.Join(dir, ".c4")); err == nil {
			knowledgeDir = filepath.Join(dir, ".c4", "knowledge")
		}
	}
	ks, err := knowledge.NewStore(knowledgeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: knowledge store: %v\n", err)
		return
	}
	title := fmt.Sprintf("세션 요약: %s (%s)", project, date)
	meta := map[string]any{
		"title":  title,
		"domain": "session",
		"tags":   []string{"session", "auto-close"},
	}
	if _, err := ks.Create(knowledge.TypeInsight, meta, summaryText); err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: knowledge save: %v\n", err)
	}
}

// captureSessionLearnPersona extracts decisions/preferences and applies persona learning.
func captureSessionLearnPersona(dir string, result *sessionCloseResult) {
	if result == nil || (len(result.Decisions) == 0 && len(result.Preferences) == 0) {
		return
	}
	var suggestions []string
	for _, d := range result.Decisions {
		suggestions = append(suggestions, "[결정] "+d)
	}
	for _, p := range result.Preferences {
		suggestions = append(suggestions, "[선호] "+p)
	}
	applySessionPersona(dir, suggestions)
}

// inferStatus determines the lifecycle status of a session using a 3-step hybrid:
//  1. Deterministic file/DB checks (idea, spec, tasks).
//  2. Interactive stderr prompt for ambiguous cases (free-form sessions).
//
// Returns one of: "idea", "planned", "active", "done", "suspended".
func inferStatus(entry namedSessionEntry) string {
	dir := entry.Dir
	idea := entry.Idea

	// 1a. idea.md present but no spec → "idea"
	if idea != "" {
		ideaPath := filepath.Join(dir, ".c4", "ideas", idea+".md")
		if _, err := os.Stat(ideaPath); err == nil {
			specPath := filepath.Join(dir, "docs", "specs", idea+".md")
			if _, err := os.Stat(specPath); err != nil {
				return "idea"
			}
		}
	}

	// 1b. spec exists — check tasks in c4.db.
	if idea != "" {
		specPath := filepath.Join(dir, "docs", "specs", idea+".md")
		if _, err := os.Stat(specPath); err == nil {
			done, total, dbErr := captureSessionCountTasks(dir, idea)
			if dbErr == nil {
				if total == 0 {
					return "planned"
				}
				if done == total {
					return "done"
				}
				return "active"
			}
			// spec exists but no tasks (DB error or empty) → "planned"
			return "planned"
		}
	}

	// 1c. No idea set — check tasks by name prefix in DB.
	if idea == "" && dir != "" {
		done, total, dbErr := captureSessionCountTasksByDir(dir)
		if dbErr == nil && total > 0 {
			if done == total {
				return "done"
			}
			return "active"
		}
	}

	// 2. Ambiguous: ask the user on stderr (interactive only).
	if isTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "cq: session '%s' complete? [y/N]: ", entry.UUID[:min(8, len(entry.UUID))])
		scanner := bufio.NewScanner(os.Stdin)
		done := make(chan string, 1)
		go func() {
			if scanner.Scan() {
				done <- strings.TrimSpace(scanner.Text())
			} else {
				done <- ""
			}
		}()
		select {
		case answer := <-done:
			if strings.EqualFold(answer, "y") {
				return "done"
			}
		case <-time.After(10 * time.Second):
		}
	}

	return "suspended"
}

// captureSessionCountTasks queries c4.db for tasks matching the idea prefix.
// Returns (done, total, error).
func captureSessionCountTasks(dir, idea string) (int, int, error) {
	dbFile := filepath.Join(dir, ".c4", "c4.db")
	if _, err := os.Stat(dbFile); err != nil {
		dbFile = filepath.Join(dir, ".c4", "tasks.db")
		if _, err := os.Stat(dbFile); err != nil {
			return 0, 0, fmt.Errorf("no db")
		}
	}

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()

	// Query tasks matching idea prefix (e.g. idea="auth-feature" → task_id LIKE 'auth-feature%').
	prefix := idea + "%"
	var total, done int
	row := db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE task_id LIKE ?", prefix)
	if err := row.Scan(&total); err != nil {
		return 0, 0, err
	}
	row = db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE task_id LIKE ? AND status = 'done'", prefix)
	if err := row.Scan(&done); err != nil {
		return 0, 0, err
	}
	return done, total, nil
}

// captureSessionCountTasksByDir counts all tasks in the project's DB.
func captureSessionCountTasksByDir(dir string) (int, int, error) {
	dbFile := filepath.Join(dir, ".c4", "c4.db")
	if _, err := os.Stat(dbFile); err != nil {
		dbFile = filepath.Join(dir, ".c4", "tasks.db")
		if _, err := os.Stat(dbFile); err != nil {
			return 0, 0, fmt.Errorf("no db")
		}
	}

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()

	var total, done int
	if err := db.QueryRow("SELECT COUNT(*) FROM c4_tasks").Scan(&total); err != nil {
		return 0, 0, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status = 'done'").Scan(&done); err != nil {
		return 0, 0, err
	}
	return done, total, nil
}

// captureSessionFindJSONL looks for the JSONL file matching sessionUUID in the
// Claude project directory for the given projectDir.
func captureSessionFindJSONL(projectDir, sessionUUID string) string {
	if sessionUUID == "" {
		return ""
	}
	claudeDir, err := claudeProjectDir(projectDir)
	if err != nil {
		return ""
	}
	candidate := filepath.Join(claudeDir, sessionUUID+".jsonl")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// captureSessionExtractConversation reads a JSONL file and returns plain-text conversation.
// Mirrors the logic in sessionsummarizer.readConversation.
func captureSessionExtractConversation(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	type jsonlEntry struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Message *struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
		Content any `json:"content"`
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry jsonlEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		role := entry.Role
		var contentRaw any = entry.Content
		if entry.Message != nil {
			if entry.Message.Role != "" {
				role = entry.Message.Role
			}
			contentRaw = entry.Message.Content
		}
		if role != "user" && role != "assistant" {
			continue
		}

		text := captureSessionExtractText(contentRaw)
		if text == "" {
			continue
		}
		prefix := "User"
		if role == "assistant" {
			prefix = "Assistant"
		}
		sb.WriteString(prefix)
		sb.WriteString(": ")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// captureSessionExtractText converts a content field (string or []block) to plain text.
func captureSessionExtractText(content any) string {
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

// captureSessionTruncate truncates text to approximately maxTokens tokens.
// Keeps the last portion (most recent conversation).
func captureSessionTruncate(text string, maxTokens int) string {
	const approxCharsPerToken = 4
	maxChars := maxTokens * approxCharsPerToken
	if len(text) <= maxChars {
		return text
	}
	truncated := text[len(text)-maxChars:]
	if idx := strings.Index(truncated, "\n"); idx >= 0 && idx < 200 {
		truncated = "[이전 대화 생략]\n\n" + truncated[idx+1:]
	} else {
		truncated = "[이전 대화 생략]\n\n" + truncated
	}
	return truncated
}

