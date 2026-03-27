package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
				return "in-progress"
			}
			return "planned"
		}
	}

	return "" // can't determine — keep existing
}

// captureSessionSummarizeFn is set by the llm_gateway build variant.
// When nil, LLM summarization is skipped and summary remains empty.
var captureSessionSummarizeFn func(jsonlPath, project, date string) string

// captureSession infers the session lifecycle status, generates a 2-line LLM summary
// (best-effort), and updates the named-sessions.json entry for the given session name.
// This function is best-effort: any errors are logged to stderr but do not block
// the caller.
func captureSession(name string) {
	sessions, err := loadNamedSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: load sessions: %v\n", err)
		return
	}

	entry, ok := sessions[name]
	if !ok {
		return
	}

	// Infer session lifecycle status.
	status := inferStatus(entry)

	// Find the JSONL file for this session UUID.
	jsonlPath := ""
	if entry.UUID != "" && entry.Dir != "" {
		jsonlPath = captureSessionFindJSONL(entry.Dir, entry.UUID)
	}

	// Generate LLM summary (best-effort, 5-second timeout enforced in the LLM variant).
	summary := ""
	if jsonlPath != "" && captureSessionSummarizeFn != nil {
		project := filepath.Base(entry.Dir)
		date := time.Now().Format("2006-01-02")
		summary = captureSessionSummarizeFn(jsonlPath, project, date)
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
}

// inferStatus determines the lifecycle status of a session using a 3-step hybrid:
//  1. Deterministic file/DB checks (idea, spec, tasks).
//  2. Interactive stderr prompt for ambiguous cases (free-form sessions).
//
// Returns one of: "idea", "planned", "in-progress", "done", "suspended".
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
				return "in-progress"
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
			return "in-progress"
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

