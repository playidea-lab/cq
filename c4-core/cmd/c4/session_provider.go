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
	chatgptProvider(),
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
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return nil
	}
	var questions []string
	for _, msg := range session.Messages {
		if msg.Type != "user" {
			continue
		}
		text := extractGeminiContentText(msg.Content)
		if text != "" && !strings.HasPrefix(text, "<") {
			questions = append(questions, text)
		}
	}
	return questions
}

// extractGeminiContentText extracts text from Gemini content which can be
// string or []interface{} with {"text": "..."} entries.
func extractGeminiContentText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					return strings.TrimSpace(t)
				}
			}
		}
	}
	return ""
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
			// Check file mod time — recent sessions are likely active
			status := "done"
			if info, err := cf.Info(); err == nil {
				if time.Since(info.ModTime()) < 10*time.Minute {
					status = "active"
				}
			}
			result[tag] = namedSessionEntry{
				UUID:    meta.sessionID,
				Dir:     projDir,
				Tool:    "gemini",
				Status:  status,
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
	f, err := os.Open(path)
	if err != nil {
		return geminiSessionMeta{}
	}
	defer f.Close()

	// Stream-parse: read only top-level fields + first user message.
	// Avoids loading entire file (can be >10MB) into memory.
	dec := json.NewDecoder(f)

	// Read opening '{'
	t, err := dec.Token()
	if err != nil || t != json.Delim('{') {
		return geminiSessionMeta{}
	}

	var meta geminiSessionMeta
	for dec.More() {
		// Read key
		t, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := t.(string)
		if !ok {
			break
		}

		switch key {
		case "sessionId":
			var v string
			if dec.Decode(&v) == nil {
				meta.sessionID = v
			}
		case "lastUpdated":
			var v string
			if dec.Decode(&v) == nil {
				meta.lastUpdated = v
			}
		case "messages":
			// Stream through messages array, stop at first user message
			t2, err := dec.Token() // '['
			if err != nil || t2 != json.Delim('[') {
				break
			}
			for dec.More() {
				var msg struct {
					Type    string `json:"type"`
					Content any    `json:"content"`
				}
				if dec.Decode(&msg) != nil {
					break
				}
				if msg.Type != "user" {
					continue
				}
				// Content can be string or []interface{} with {text: "..."}
				text := ""
				switch v := msg.Content.(type) {
				case string:
					text = v
				case []interface{}:
					for _, item := range v {
						if m, ok := item.(map[string]interface{}); ok {
							if t, ok := m["text"].(string); ok && t != "" {
								text = t
								break
							}
						}
					}
				}
				if text == "" {
					continue
				}
				text = strings.TrimSpace(text)
				if strings.HasPrefix(text, "# ") || strings.HasPrefix(text, "## ") {
					continue
				}
				if idx := strings.IndexByte(text, '\n'); idx > 0 {
					text = text[:idx]
				}
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				if len(text) > 80 {
					text = text[:80] + "..."
				}
				meta.firstMessage = text
				break
			}
			// Don't parse remaining messages — skip to end of array
			// (decoder will skip when we move to next key)
		default:
			// Skip unknown fields
			var skip json.RawMessage
			dec.Decode(&skip)
		}

		// Early exit if we have everything
		if meta.sessionID != "" && meta.firstMessage != "" {
			break
		}
	}
	return meta
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

	// Scan both archived and active sessions
	scanDirs := []string{
		filepath.Join(homeDir, ".codex", "archived_sessions"),
	}
	// Active sessions: ~/.codex/sessions/YYYY/MM/DD/
	sessionsDir := filepath.Join(homeDir, ".codex", "sessions")
	_ = filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		scanDirs = append(scanDirs, "file:"+path)
		return nil
	})

	for _, dir := range scanDirs {
		var files []string
		if strings.HasPrefix(dir, "file:") {
			files = []string{strings.TrimPrefix(dir, "file:")}
		} else {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".jsonl") {
					files = append(files, filepath.Join(dir, e.Name()))
				}
			}
		}
		for _, path := range files {
			meta := readCodexSessionMeta(path)
			if meta.id == "" {
				continue
			}
			summary := threadNames[meta.id]
			if summary == "" {
				summary = meta.firstMessage
			}
			tag := fmt.Sprintf("codex/%s", meta.id[:8])
			if _, exists := result[tag]; exists {
				continue // dedup
			}
			status := "done"
			if info, err := os.Stat(path); err == nil {
				if time.Since(info.ModTime()) < 10*time.Minute {
					status = "active"
				}
			}
			result[tag] = namedSessionEntry{
				UUID:    meta.id,
				Dir:     meta.cwd,
				Tool:    "codex",
				Status:  status,
				Summary: summary,
				Updated: meta.timestamp,
			}
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

// --- ChatGPT Provider ---

func chatgptProvider() sessionProvider {
	return sessionProvider{
		Tool: "chatgpt",
		FindTranscript: func(dir, uuid string) string {
			return findChatGPTTranscript(uuid)
		},
		LoadHistory: loadChatGPTHistory,
		ScanSessions: scanChatGPTSessions,
	}
}

// scanChatGPTSessions reads sessions from ~/.c4/imports/chatgpt/sessions.json.
// These are pre-built by `cq import chatgpt`.
func scanChatGPTSessions() map[string]namedSessionEntry {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return nil
	}
	path := filepath.Join(homeDir, ".c4", "imports", "chatgpt", "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sessions map[string]namedSessionEntry
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil
	}
	return sessions
}

// findChatGPTTranscript locates the conversation file containing a given UUID.
// Returns "filepath|uuid" composite key for loadChatGPTHistory to parse.
func findChatGPTTranscript(uuid string) string {
	if uuid == "" {
		return ""
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	convDir := filepath.Join(homeDir, ".c4", "imports", "chatgpt", "conversations")
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(convDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), uuid) {
			return path + "|" + uuid // composite key: filepath|uuid
		}
	}
	return ""
}

// loadChatGPTHistory extracts user messages from a ChatGPT conversation.
// The path argument is a "filepath|uuid" composite from findChatGPTTranscript.
func loadChatGPTHistory(path string) []string {
	parts := strings.SplitN(path, "|", 2)
	if len(parts) != 2 {
		return nil
	}
	filePath, uuid := parts[0], parts[1]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var convs []chatGPTConversation
	if err := json.Unmarshal(data, &convs); err != nil {
		return nil
	}

	// Find the conversation matching uuid
	for _, conv := range convs {
		if conv.ID != uuid {
			continue
		}
		return extractChatGPTUserMessages(conv)
	}
	return nil
}

// extractChatGPTUserMessages walks the conversation tree from current_node
// to root, reverses to chronological order, and extracts user message text.
func extractChatGPTUserMessages(conv chatGPTConversation) []string {
	if conv.CurrentNode == "" {
		return nil
	}

	// Walk from current_node to root collecting node IDs
	var nodePath []string
	node := conv.CurrentNode
	for node != "" {
		nodePath = append(nodePath, node)
		n, ok := conv.Mapping[node]
		if !ok {
			break
		}
		node = n.Parent
	}

	// Reverse to chronological order
	for i, j := 0, len(nodePath)-1; i < j; i, j = i+1, j-1 {
		nodePath[i], nodePath[j] = nodePath[j], nodePath[i]
	}

	var messages []string
	for _, nodeID := range nodePath {
		n, ok := conv.Mapping[nodeID]
		if !ok || n.Message == nil {
			continue
		}
		if n.Message.Author.Role != "user" {
			continue
		}
		text := extractChatGPTPartText(n.Message.Content.Parts)
		if text != "" {
			messages = append(messages, text)
		}
	}
	return messages
}

// extractChatGPTPartText extracts text from ChatGPT message content parts.
func extractChatGPTPartText(parts []interface{}) string {
	var texts []string
	for _, p := range parts {
		if s, ok := p.(string); ok && s != "" {
			texts = append(texts, strings.TrimSpace(s))
		}
	}
	return strings.Join(texts, "\n")
}
