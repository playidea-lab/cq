package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sessionProvider abstracts session discovery and history extraction for different AI tools.
type sessionProvider struct {
	// Tool name (e.g. "claude", "gemini", "codex")
	Tool string
	// FindTranscript locates the session transcript file given projectDir and sessionUUID.
	// Returns empty string if not found.
	FindTranscript func(dir, uuid string) string
	// LoadHistory extracts user questions from a transcript file.
	// Returns nil if file can't be read or has no user messages.
	LoadHistory func(path string) []string
	// ScanSessions discovers sessions from the tool's local storage.
	// Returns entries keyed by a display tag (e.g. "codex/abc12345").
	ScanSessions func() map[string]namedSessionEntry
}

// providers is the registry of all session providers.
var providers = []sessionProvider{
	claudeProvider(),
	geminiProvider(),
	codexProvider(),
}

// findProviderByTool returns the provider for the given tool name, or nil.
func findProviderByTool(tool string) *sessionProvider {
	for i := range providers {
		if providers[i].Tool == tool {
			return &providers[i]
		}
	}
	return nil
}

// --- Claude Provider ---

func claudeProvider() sessionProvider {
	return sessionProvider{
		Tool: "claude",
		FindTranscript: func(dir, uuid string) string {
			return captureSessionFindJSONL(dir, uuid)
		},
		LoadHistory: func(path string) []string {
			return loadClaudeHistory(path)
		},
		ScanSessions: nil, // Claude sessions are managed by named-sessions.json
	}
}

// loadClaudeHistory reads user questions from a Claude JSONL file.
func loadClaudeHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var questions []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, `"user"`) {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message *struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "user" || entry.Message == nil || entry.Message.Role != "user" {
			continue
		}
		text := extractUserText(entry.Message.Content)
		if text == "" || strings.HasPrefix(text, "<command-") || strings.HasPrefix(text, "Base directory") {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			questions = append(questions, text)
		}
	}
	return questions
}

// --- Gemini Provider ---

func geminiProvider() sessionProvider {
	return sessionProvider{
		Tool: "gemini",
		FindTranscript: func(dir, uuid string) string {
			return findGeminiTranscript(dir, uuid)
		},
		LoadHistory: func(path string) []string {
			return loadGeminiSessionHistory(path)
		},
		ScanSessions: scanGeminiSessions,
	}
}

// findGeminiTranscript finds a Gemini session JSON by project hash + UUID prefix.
func findGeminiTranscript(dir, uuid string) string {
	if uuid == "" || dir == "" {
		return ""
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	h := sha256.Sum256([]byte(absDir))
	projectHash := fmt.Sprintf("%x", h)
	chatsDir := filepath.Join(homeDir, ".gemini", "tmp", projectHash, "chats")
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		return ""
	}
	uuidPrefix := uuid[:min(8, len(uuid))]
	for _, e := range entries {
		if strings.Contains(e.Name(), uuidPrefix) {
			return filepath.Join(chatsDir, e.Name())
		}
	}
	return ""
}

// loadGeminiSessionHistory reads user messages from a Gemini session JSON.
func loadGeminiSessionHistory(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var session struct {
		Messages []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return nil
	}
	var questions []string
	for _, msg := range session.Messages {
		if msg.Type == "user" && msg.Content != "" {
			text := strings.TrimSpace(msg.Content)
			if text != "" && !strings.HasPrefix(text, "<") {
				questions = append(questions, text)
			}
		}
	}
	return questions
}

// scanGeminiSessions discovers Gemini sessions from ~/.gemini/tmp/*/chats/.
func scanGeminiSessions() map[string]namedSessionEntry {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return nil
	}
	tmpDir := filepath.Join(homeDir, ".gemini", "tmp")
	projectDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}
	historyDir := filepath.Join(homeDir, ".gemini", "history")
	result := make(map[string]namedSessionEntry)
	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		chatsDir := filepath.Join(tmpDir, pd.Name(), "chats")
		chatFiles, err := os.ReadDir(chatsDir)
		if err != nil {
			continue
		}
		// Resolve project dir from history
		projDir := resolveGeminiProjectDir(historyDir, pd.Name())
		for _, cf := range chatFiles {
			if !strings.HasPrefix(cf.Name(), "session-") || !strings.HasSuffix(cf.Name(), ".json") {
				continue
			}
			path := filepath.Join(chatsDir, cf.Name())
			meta := readGeminiSessionMeta(path)
			if meta.sessionID == "" {
				continue
			}
			tag := fmt.Sprintf("gemini/%s", meta.sessionID[:8])
			result[tag] = namedSessionEntry{
				UUID:    meta.sessionID,
				Dir:     projDir,
				Tool:    "gemini",
				Status:  "done",
				Summary: meta.firstMessage,
				Updated: meta.lastUpdated,
			}
		}
	}
	return result
}

type geminiSessionMeta struct {
	sessionID    string
	firstMessage string
	lastUpdated  string
}

func readGeminiSessionMeta(path string) geminiSessionMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return geminiSessionMeta{}
	}
	// Only parse the fields we need (file can be huge)
	var raw struct {
		SessionID   string `json:"sessionId"`
		LastUpdated string `json:"lastUpdated"`
		Messages    []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return geminiSessionMeta{}
	}
	first := ""
	for _, m := range raw.Messages {
		if m.Type == "user" && m.Content != "" {
			msg := strings.TrimSpace(m.Content)
			// Skip markdown headers and IDE context
			if strings.HasPrefix(msg, "# ") || strings.HasPrefix(msg, "## ") || strings.HasPrefix(msg, "# Context") {
				continue
			}
			// Take first line only
			if idx := strings.IndexByte(msg, '\n'); idx > 0 {
				msg = msg[:idx]
			}
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
			if len(msg) > 80 {
				msg = msg[:80] + "..."
			}
			first = msg
			break
		}
	}
	return geminiSessionMeta{
		sessionID:    raw.SessionID,
		firstMessage: first,
		lastUpdated:  raw.LastUpdated,
	}
}

// resolveGeminiProjectDir finds the actual project path from ~/.gemini/history/.
func resolveGeminiProjectDir(historyDir, projectHash string) string {
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rootFile := filepath.Join(historyDir, e.Name(), ".project_root")
		data, err := os.ReadFile(rootFile)
		if err != nil {
			continue
		}
		absDir := strings.TrimSpace(string(data))
		h := sha256.Sum256([]byte(absDir))
		if fmt.Sprintf("%x", h) == projectHash {
			return absDir
		}
	}
	return ""
}

// --- Codex Provider ---

func codexProvider() sessionProvider {
	return sessionProvider{
		Tool: "codex",
		FindTranscript: func(dir, uuid string) string {
			return findCodexTranscript(uuid)
		},
		LoadHistory: func(path string) []string {
			return loadCodexHistory(path)
		},
		ScanSessions: scanCodexSessions,
	}
}

// findCodexTranscript finds a Codex session JSONL by UUID.
func findCodexTranscript(uuid string) string {
	if uuid == "" {
		return ""
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	// Check archived sessions
	archiveDir := filepath.Join(homeDir, ".codex", "archived_sessions")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), uuid[:min(8, len(uuid))]) {
			return filepath.Join(archiveDir, e.Name())
		}
	}
	return ""
}

// loadCodexHistory reads user messages from a Codex JSONL file.
func loadCodexHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var questions []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, `"user_message"`) {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Payload.Type != "user_message" {
			continue
		}
		text := strings.TrimSpace(entry.Payload.Message)
		if text != "" {
			questions = append(questions, text)
		}
	}
	return questions
}

// scanCodexSessions discovers sessions from ~/.codex/session_index.jsonl and archived_sessions/.
func scanCodexSessions() map[string]namedSessionEntry {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return nil
	}
	result := make(map[string]namedSessionEntry)

	// Read session index for thread names
	indexPath := filepath.Join(homeDir, ".codex", "session_index.jsonl")
	threadNames := make(map[string]string) // id → thread_name
	if f, err := os.Open(indexPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var entry struct {
				ID         string `json:"id"`
				ThreadName string `json:"thread_name"`
				UpdatedAt  string `json:"updated_at"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
				threadNames[entry.ID] = entry.ThreadName
			}
		}
		f.Close()
	}

	// Scan archived sessions
	archiveDir := filepath.Join(homeDir, ".codex", "archived_sessions")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(archiveDir, e.Name())
		meta := readCodexSessionMeta(path)
		if meta.id == "" {
			continue
		}
		summary := threadNames[meta.id]
		if summary == "" {
			summary = meta.firstMessage
		}
		tag := fmt.Sprintf("codex/%s", meta.id[:8])
		result[tag] = namedSessionEntry{
			UUID:    meta.id,
			Dir:     meta.cwd,
			Tool:    "codex",
			Status:  "done",
			Summary: summary,
			Updated: meta.timestamp,
		}
	}
	return result
}

type codexSessionMeta struct {
	id           string
	cwd          string
	timestamp    string
	firstMessage string
}

func readCodexSessionMeta(path string) codexSessionMeta {
	f, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}
	}
	defer f.Close()

	var meta codexSessionMeta
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		var entry struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				ID        string `json:"id"`
				Timestamp string `json:"timestamp"`
				CWD       string `json:"cwd"`
				Type      string `json:"type"`
				Message   string `json:"message"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type == "session_meta" {
			meta.id = entry.Payload.ID
			meta.cwd = entry.Payload.CWD
			meta.timestamp = entry.Payload.Timestamp
			if meta.timestamp == "" {
				meta.timestamp = entry.Timestamp
			}
		}
		if entry.Payload.Type == "user_message" && meta.firstMessage == "" {
			msg := strings.TrimSpace(entry.Payload.Message)
			// Skip IDE context injection (not a real user message)
			if strings.HasPrefix(msg, "# Context from") || strings.HasPrefix(msg, "## Active") {
				continue
			}
			// Take first meaningful line only
			if idx := strings.IndexByte(msg, '\n'); idx > 0 {
				msg = msg[:idx]
			}
			msg = strings.TrimSpace(msg)
			if len(msg) > 80 {
				msg = msg[:80] + "..."
			}
			if msg != "" {
				meta.firstMessage = msg
			}
		}
		// Stop after we have both
		if meta.id != "" && meta.firstMessage != "" {
			break
		}
	}

	// Format timestamp
	if meta.timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, meta.timestamp); err == nil {
			meta.timestamp = t.Format(time.RFC3339)
		}
	}
	return meta
}
