package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/mailbox"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterMailHandlers registers c4_mail_* MCP tools.
func RegisterMailHandlers(reg *mcp.Registry, ms *mailbox.MailStore) {
	// c4_mail_send — send a message to a session
	reg.Register(mcp.ToolSchema{
		Name:        "c4_mail_send",
		Description: "Send a local (in-process) message to a CQ session. The message is stored unread until c4_mail_read is called.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to":      map[string]any{"type": "string", "description": "Recipient session name"},
				"subject": map[string]any{"type": "string", "description": "Message subject (optional)"},
				"body":    map[string]any{"type": "string", "description": "Message body"},
				"from":    map[string]any{"type": "string", "description": "Sender name; defaults to CQ_SESSION_NAME env var"},
			},
			"required": []string{"to", "body"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			To      string `json:"to"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			From    string `json:"from"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		// Resolve defaults before validating.
		if args.From == "" {
			args.From = os.Getenv("CQ_SESSION_NAME")
		}
		if args.To == "" {
			return nil, fmt.Errorf("to is required")
		}
		if args.From == "*" {
			return nil, fmt.Errorf("from cannot be '*'")
		}
		if args.Body == "" {
			return nil, fmt.Errorf("body is required")
		}
		// Send returns (id, created_at) from a single clock read so the response
		// matches what is stored in the DB (no second Read() call needed).
		// project_id is reserved for future project-scoped mailbox filtering.
		id, createdAt, err := ms.Send(args.From, args.To, args.Subject, args.Body, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":         id,
			"created_at": createdAt,
		}, nil
	})

	// c4_mail_ls — list messages
	reg.Register(mcp.ToolSchema{
		Name:        "c4_mail_ls",
		Description: "List mail messages. Optionally filter by session (to_addr) and/or unread-only.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session":     map[string]any{"type": "string", "description": "Filter by recipient session (defaults to CQ_SESSION_NAME)"},
				"unread_only": map[string]any{"type": "boolean", "description": "If true, return only unread messages"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Session    string `json:"session"`
			UnreadOnly bool   `json:"unread_only"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Session == "" {
			args.Session = os.Getenv("CQ_SESSION_NAME")
		}
		msgs, err := ms.List(args.Session, args.UnreadOnly)
		if err != nil {
			return nil, err
		}
		type item struct {
			ID        int64  `json:"id"`
			From      string `json:"from"`
			To        string `json:"to"`
			Subject   string `json:"subject"`
			CreatedAt string `json:"created_at"`
			ReadAt    string `json:"read_at"`
		}
		out := make([]item, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, item{
				ID:        m.ID,
				From:      m.From,
				To:        m.To,
				Subject:   m.Subject,
				CreatedAt: m.CreatedAt,
				ReadAt:    m.ReadAt,
			})
		}
		return out, nil
	})

	// c4_mail_read — read (and mark as read) a message by ID
	reg.Register(mcp.ToolSchema{
		Name:        "c4_mail_read",
		Description: "Read a mail message by ID. Marks the message as read (sets read_at).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "Message ID"},
			},
			"required": []string{"id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		msg, err := ms.Read(args.ID)
		if errors.Is(err, mailbox.ErrNotFound) {
			return nil, fmt.Errorf("message %d not found", args.ID)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":         msg.ID,
			"from":       msg.From,
			"to":         msg.To,
			"subject":    msg.Subject,
			"body":       msg.Body,
			"created_at": msg.CreatedAt,
			"read_at":    msg.ReadAt,
		}, nil
	})

	// c4_mail_rm — delete a message by ID
	reg.Register(mcp.ToolSchema{
		Name:        "c4_mail_rm",
		Description: "Delete a mail message by ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "Message ID to delete"},
			},
			"required": []string{"id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if err := ms.Delete(args.ID); errors.Is(err, mailbox.ErrNotFound) {
			return nil, fmt.Errorf("message %d not found", args.ID)
		} else if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true}, nil
	})
}
