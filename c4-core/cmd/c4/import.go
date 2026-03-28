package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import conversation history from AI tools",
}

var importChatGPTCmd = &cobra.Command{
	Use:   "chatgpt <export.zip>",
	Short: "Import ChatGPT conversations",
	Args:  cobra.ExactArgs(1),
	RunE:  runImportChatGPT,
}

func init() {
	importCmd.AddCommand(importChatGPTCmd)
	rootCmd.AddCommand(importCmd)
}

// --- ChatGPT export data types ---

type chatGPTConversation struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	CreateTime   float64                 `json:"create_time"`
	DefaultModel string                  `json:"default_model_slug"`
	Mapping      map[string]chatGPTNode  `json:"mapping"`
	CurrentNode  string                  `json:"current_node"`
}

type chatGPTNode struct {
	Message  *chatGPTMessage `json:"message"`
	Parent   string          `json:"parent"`
	Children []string        `json:"children"`
}

type chatGPTMessage struct {
	Author struct {
		Role string `json:"role"`
	} `json:"author"`
	Content struct {
		Parts []interface{} `json:"parts"`
	} `json:"content"`
}

func runImportChatGPT(cmd *cobra.Command, args []string) error {
	zipPath := args[0]

	// 1. Open zip
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	// 2. Find and parse conversations-*.json files
	var allConvs []chatGPTConversation
	var convFiles []*zip.File
	for _, f := range r.File {
		if !strings.HasPrefix(filepath.Base(f.Name), "conversations") || !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		convFiles = append(convFiles, f)
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		var convs []chatGPTConversation
		if err := json.NewDecoder(rc).Decode(&convs); err != nil {
			rc.Close()
			return fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}
		rc.Close()
		allConvs = append(allConvs, convs...)
	}

	if len(allConvs) == 0 {
		fmt.Println("  No conversations found in zip.")
		return nil
	}

	// 3. Prepare import directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	importDir := filepath.Join(homeDir, ".c4", "imports", "chatgpt")
	convDir := filepath.Join(importDir, "conversations")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		return fmt.Errorf("failed to create import directory: %w", err)
	}

	// 4. Load existing sessions (merge)
	sessionsFile := filepath.Join(importDir, "sessions.json")
	sessions := make(map[string]namedSessionEntry)
	if data, err := os.ReadFile(sessionsFile); err == nil {
		_ = json.Unmarshal(data, &sessions)
	}

	// 5. Build sessions from conversations
	var newCount int
	var totalNodes int
	modelCounts := make(map[string]int)
	var minTime, maxTime float64

	for _, conv := range allConvs {
		if conv.ID == "" {
			continue
		}

		// Track time range
		if minTime == 0 || (conv.CreateTime > 0 && conv.CreateTime < minTime) {
			minTime = conv.CreateTime
		}
		if conv.CreateTime > maxTime {
			maxTime = conv.CreateTime
		}

		// Count nodes
		totalNodes += len(conv.Mapping)

		// Count models
		model := conv.DefaultModel
		if model == "" {
			model = "unknown"
		}
		modelCounts[model]++

		// Build tag
		idPrefix := conv.ID
		if len(idPrefix) > 8 {
			idPrefix = idPrefix[:8]
		}
		tag := fmt.Sprintf("chatgpt/%s", idPrefix)

		// Skip if already exists
		if _, exists := sessions[tag]; exists {
			continue
		}

		// Extract summary
		summary := extractFirstUserMessage(conv)
		if summary == "" {
			summary = conv.Title
		}
		summary = sanitizeSessionSummary(summary)
		// Truncate to 80 chars
		if len(summary) > 80 {
			summary = summary[:80]
		}

		// Format timestamp
		var t time.Time
		if conv.CreateTime > 0 {
			t = time.Unix(int64(conv.CreateTime), 0)
		} else {
			t = time.Now()
		}

		sessions[tag] = namedSessionEntry{
			UUID:    conv.ID,
			Tool:    "chatgpt",
			Status:  "done",
			Summary: summary,
			Updated: t.Format(time.RFC3339),
		}
		newCount++
	}

	// 6. Save sessions.json
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}
	if err := os.WriteFile(sessionsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write sessions: %w", err)
	}

	// 7. Copy conversation files
	for _, f := range convFiles {
		dest := filepath.Join(convDir, filepath.Base(f.Name))
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open %s for copy: %w", f.Name, err)
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create %s: %w", dest, err)
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return fmt.Errorf("failed to copy %s: %w", f.Name, err)
		}
	}

	// 8. Print summary
	fmt.Printf("\n  \U0001f4ca %s conversations found\n", formatNumber(len(allConvs)))

	if minTime > 0 && maxTime > 0 {
		minT := time.Unix(int64(minTime), 0)
		maxT := time.Unix(int64(maxTime), 0)
		fmt.Printf("  \u251c\u2500 Period: %s ~ %s\n", minT.Format("Jan 2006"), maxT.Format("Jan 2006"))
	}
	fmt.Printf("  \u251c\u2500 Messages: ~%s nodes\n", formatNumber(totalNodes))

	// Sort models by count descending
	type modelCount struct {
		name  string
		count int
	}
	var models []modelCount
	for name, count := range modelCounts {
		models = append(models, modelCount{name, count})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].count > models[j].count })

	var parts []string
	for _, m := range models {
		parts = append(parts, fmt.Sprintf("%s (%s)", m.name, formatNumber(m.count)))
	}
	fmt.Printf("  \u2514\u2500 Models: %s\n", strings.Join(parts, ", "))

	fmt.Printf("\n  \u2713 %s sessions registered", formatNumber(newCount))
	if newCount < len(allConvs) {
		fmt.Printf(" (%s already existed)", formatNumber(len(allConvs)-newCount))
	}
	fmt.Println()

	fmt.Println("\n  Next:")
	fmt.Println("    cq sessions     \u2192 \uacfc\uac70 \ub300\ud654 \uc5f4\ub78c")
	fmt.Println()

	return nil
}

// extractFirstUserMessage walks the conversation tree from root to find the first user message.
func extractFirstUserMessage(conv chatGPTConversation) string {
	// Find root node (no parent or empty parent)
	var rootID string
	for id, node := range conv.Mapping {
		if node.Parent == "" {
			rootID = id
			break
		}
	}
	if rootID == "" {
		return ""
	}

	// BFS from root to find first user message
	queue := []string{rootID}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		node, ok := conv.Mapping[current]
		if !ok {
			continue
		}

		if node.Message != nil && node.Message.Author.Role == "user" {
			if len(node.Message.Content.Parts) > 0 {
				if s, ok := node.Message.Content.Parts[0].(string); ok && s != "" {
					// Take first line, max 80 chars
					if idx := strings.IndexByte(s, '\n'); idx >= 0 {
						s = s[:idx]
					}
					s = strings.TrimSpace(s)
					if len(s) > 80 {
						s = s[:80]
					}
					return s
				}
			}
		}

		queue = append(queue, node.Children...)
	}
	return ""
}

// formatNumber formats an integer with comma separators.
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
