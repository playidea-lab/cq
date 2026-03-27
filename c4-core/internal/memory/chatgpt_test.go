package memory

import (
	"strings"
	"testing"
	"time"
)

func TestChatGPTParser_Source(t *testing.T) {
	p := &ChatGPTParser{}
	if got := p.Source(); got != "chatgpt" {
		t.Errorf("Source() = %q, want %q", got, "chatgpt")
	}
}

func TestChatGPTParser_Parse_TwoConversations(t *testing.T) {
	// Minimal conversations.json with 2 conversations, each with a linear chain.
	// Conversation 1: root -> user -> assistant
	// Conversation 2: root -> user
	input := `[
		{
			"id": "conv-1",
			"title": "First Chat",
			"create_time": 1700000000.0,
			"mapping": {
				"root": {
					"message": null,
					"parent": null,
					"children": ["msg-1"]
				},
				"msg-1": {
					"message": {
						"author": {"role": "user"},
						"content": {"parts": ["Hello world"]},
						"create_time": 1700000001.0
					},
					"parent": "root",
					"children": ["msg-2"]
				},
				"msg-2": {
					"message": {
						"author": {"role": "assistant"},
						"content": {"parts": ["Hi there!"]},
						"create_time": 1700000002.0
					},
					"parent": "msg-1",
					"children": []
				}
			}
		},
		{
			"id": "conv-2",
			"title": "Second Chat",
			"create_time": 1700100000.0,
			"mapping": {
				"root2": {
					"message": null,
					"parent": null,
					"children": ["msg-3"]
				},
				"msg-3": {
					"message": {
						"author": {"role": "user"},
						"content": {"parts": ["Just a question"]},
						"create_time": 1700100001.0
					},
					"parent": "root2",
					"children": []
				}
			}
		}
	]`

	p := &ChatGPTParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}

	// Conversation 1
	s1 := sessions[0]
	if s1.ID != "conv-1" {
		t.Errorf("session[0].ID = %q, want %q", s1.ID, "conv-1")
	}
	if s1.Title != "First Chat" {
		t.Errorf("session[0].Title = %q, want %q", s1.Title, "First Chat")
	}
	if s1.Source != "chatgpt" {
		t.Errorf("session[0].Source = %q, want %q", s1.Source, "chatgpt")
	}
	expectedTime := time.Unix(1700000000, 0).UTC()
	if !s1.StartedAt.Equal(expectedTime) {
		t.Errorf("session[0].StartedAt = %v, want %v", s1.StartedAt, expectedTime)
	}
	if len(s1.Turns) != 2 {
		t.Fatalf("session[0] got %d turns, want 2", len(s1.Turns))
	}
	if s1.Turns[0].Role != "user" || s1.Turns[0].Content != "Hello world" {
		t.Errorf("session[0].Turns[0] = %+v, want user/Hello world", s1.Turns[0])
	}
	if s1.Turns[1].Role != "assistant" || s1.Turns[1].Content != "Hi there!" {
		t.Errorf("session[0].Turns[1] = %+v, want assistant/Hi there!", s1.Turns[1])
	}

	// Conversation 2
	s2 := sessions[1]
	if s2.ID != "conv-2" {
		t.Errorf("session[1].ID = %q, want %q", s2.ID, "conv-2")
	}
	if len(s2.Turns) != 1 {
		t.Fatalf("session[1] got %d turns, want 1", len(s2.Turns))
	}
	if s2.Turns[0].Role != "user" || s2.Turns[0].Content != "Just a question" {
		t.Errorf("session[1].Turns[0] = %+v, want user/Just a question", s2.Turns[0])
	}
}

func TestChatGPTParser_Parse_EmptyConversations(t *testing.T) {
	input := `[]`
	p := &ChatGPTParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("got %d sessions, want 0", len(sessions))
	}
}

func TestChatGPTParser_Parse_NullMessages(t *testing.T) {
	// Nodes with null messages should be skipped in turn extraction.
	input := `[{
		"id": "conv-null",
		"title": "Null Test",
		"create_time": 1700000000.0,
		"mapping": {
			"root": {
				"message": null,
				"parent": null,
				"children": ["placeholder"]
			},
			"placeholder": {
				"message": null,
				"parent": "root",
				"children": ["real-msg"]
			},
			"real-msg": {
				"message": {
					"author": {"role": "user"},
					"content": {"parts": ["actual content"]},
					"create_time": 1700000001.0
				},
				"parent": "placeholder",
				"children": []
			}
		}
	}]`

	p := &ChatGPTParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Content != "actual content" {
		t.Errorf("turn content = %q, want %q", sessions[0].Turns[0].Content, "actual content")
	}
}

func TestChatGPTParser_Parse_MixedParts(t *testing.T) {
	// content.parts can contain non-string elements (images etc); only strings extracted.
	input := `[{
		"id": "conv-parts",
		"title": "Parts Test",
		"create_time": 1700000000.0,
		"mapping": {
			"root": {
				"message": null,
				"parent": null,
				"children": ["msg"]
			},
			"msg": {
				"message": {
					"author": {"role": "user"},
					"content": {"parts": ["text part", {"type": "image"}, "another part"]},
					"create_time": 1700000001.0
				},
				"parent": "root",
				"children": []
			}
		}
	}]`

	p := &ChatGPTParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Content != "text part another part" {
		t.Errorf("content = %q, want %q", sessions[0].Turns[0].Content, "text part another part")
	}
}

func TestChatGPTParser_Parse_SystemRoleSkipped(t *testing.T) {
	// System messages should be included as turns with role "system".
	input := `[{
		"id": "conv-sys",
		"title": "System Test",
		"create_time": 1700000000.0,
		"mapping": {
			"root": {
				"message": null,
				"parent": null,
				"children": ["sys"]
			},
			"sys": {
				"message": {
					"author": {"role": "system"},
					"content": {"parts": ["You are helpful"]},
					"create_time": 1700000001.0
				},
				"parent": "root",
				"children": ["usr"]
			},
			"usr": {
				"message": {
					"author": {"role": "user"},
					"content": {"parts": ["Hi"]},
					"create_time": 1700000002.0
				},
				"parent": "sys",
				"children": []
			}
		}
	}]`

	p := &ChatGPTParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 2 {
		t.Fatalf("got %d turns, want 2 (system + user)", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Role != "system" {
		t.Errorf("turn[0].Role = %q, want %q", sessions[0].Turns[0].Role, "system")
	}
	if sessions[0].Turns[1].Role != "user" {
		t.Errorf("turn[1].Role = %q, want %q", sessions[0].Turns[1].Role, "user")
	}
}

func TestChatGPTParser_Parse_InvalidJSON(t *testing.T) {
	p := &ChatGPTParser{}
	_, err := p.Parse(strings.NewReader(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
