package pop

import (
	"context"
	"time"
)

// Proposal represents a knowledge item surfaced by the POP engine.
type Proposal struct {
	ID           string
	Title        string
	Content      string
	ItemType     string
	Confidence   float64
	Visibility   string
	SourceMsgIDs []string
}

// MessageSource provides access to recent conversation messages.
type MessageSource interface {
	// RecentMessages returns messages after the given time, up to limit.
	RecentMessages(ctx context.Context, after time.Time, limit int) ([]Message, error)
}

// Message is a single conversation message returned by MessageSource.
type Message struct {
	ID        string
	Content   string
	CreatedAt time.Time
}

// KnowledgeStore records proposals as knowledge entries.
type KnowledgeStore interface {
	// RecordProposal persists a proposal and returns its stored ID.
	RecordProposal(ctx context.Context, p Proposal) (string, error)
}

// SoulWriter updates the user's soul/persona based on extracted insights.
type SoulWriter interface {
	// AppendInsight appends a raw insight string to the soul.
	AppendInsight(ctx context.Context, userID, insight string) error
}

// Notifier delivers proposals to the user interface.
type Notifier interface {
	// Notify sends a proposal notification.
	Notify(ctx context.Context, p Proposal) error
}

// LLMClient invokes a language model for extraction and crystallization.
type LLMClient interface {
	// Complete sends a prompt and returns the completion.
	Complete(ctx context.Context, prompt string) (string, error)
}
