package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ChatGPTParser parses ChatGPT's conversations.json export format.
type ChatGPTParser struct{}

func (p *ChatGPTParser) Source() string { return "chatgpt" }

// Parse reads a conversations.json (the JSON array inside a ChatGPT export ZIP)
// and returns one Session per conversation.
func (p *ChatGPTParser) Parse(r io.Reader) ([]Session, error) {
	var convs []chatGPTConversation
	if err := json.NewDecoder(r).Decode(&convs); err != nil {
		return nil, fmt.Errorf("chatgpt: %w", err)
	}

	sessions := make([]Session, 0, len(convs))
	for _, c := range convs {
		s := Session{
			ID:        c.ID,
			Title:     c.Title,
			Source:    "chatgpt",
			StartedAt: time.Unix(int64(c.CreateTime), 0).UTC(),
			Turns:     extractTurns(c.Mapping),
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// chatGPTConversation is the top-level structure in conversations.json.
type chatGPTConversation struct {
	ID         string                    `json:"id"`
	Title      string                    `json:"title"`
	CreateTime float64                   `json:"create_time"`
	Mapping    map[string]chatGPTMapping `json:"mapping"`
}

type chatGPTMapping struct {
	Message  *chatGPTMessage `json:"message"`
	Parent   *string         `json:"parent"`
	Children []string        `json:"children"`
}

type chatGPTMessage struct {
	Author     chatGPTAuthor  `json:"author"`
	Content    chatGPTContent `json:"content"`
	CreateTime float64        `json:"create_time"`
}

type chatGPTAuthor struct {
	Role string `json:"role"`
}

type chatGPTContent struct {
	Parts []any `json:"parts"`
}

// extractTurns walks the mapping tree from root to leaf, collecting turns in order.
func extractTurns(mapping map[string]chatGPTMapping) []Turn {
	// Find the root node: the one with no parent or parent is nil/empty.
	rootID := ""
	for id, node := range mapping {
		if node.Parent == nil {
			rootID = id
			break
		}
	}
	if rootID == "" {
		return nil
	}

	var turns []Turn
	walkTree(mapping, rootID, &turns)
	return turns
}

// walkTree performs a depth-first walk through the conversation tree,
// following the first child at each level (linear conversation path).
func walkTree(mapping map[string]chatGPTMapping, nodeID string, turns *[]Turn) {
	node, ok := mapping[nodeID]
	if !ok {
		return
	}

	if node.Message != nil {
		role := node.Message.Author.Role
		text := extractParts(node.Message.Content.Parts)
		if role != "" && text != "" {
			*turns = append(*turns, Turn{Role: role, Content: text})
		}
	}

	// Follow children. For branching conversations, follow the last child
	// (ChatGPT typically appends the active branch last).
	for _, childID := range node.Children {
		walkTree(mapping, childID, turns)
	}
}

// extractParts concatenates string parts from a ChatGPT content.parts array.
func extractParts(parts []any) string {
	var strs []string
	for _, part := range parts {
		if s, ok := part.(string); ok && s != "" {
			strs = append(strs, strings.TrimSpace(s))
		}
	}
	return strings.Join(strs, " ")
}
