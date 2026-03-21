//go:build !c1_messenger

package messengerhandler

import "github.com/changmin/c4-core/internal/mcp"

// C1Handler is a placeholder type when c1_messenger build tag is disabled.
type C1Handler struct{}

// NewC1Handler returns nil when c1_messenger is disabled.
func NewC1Handler(_, _ string, _ any, _ string) *C1Handler { return nil }

// RegisterC1Handlers is a no-op stub when c1_messenger build tag is disabled.
func RegisterC1Handlers(_ *mcp.Registry, _ *C1Handler) {}

// UpdatePresence is a no-op when c1_messenger is disabled.
func (h *C1Handler) UpdatePresence(_, _, _, _ string) error { return nil }

// EnsureMember is a no-op when c1_messenger is disabled.
func (h *C1Handler) EnsureMember(_, _, _ string) (string, error) { return "", nil }

// cqMentionRow is a stub type for @cq mention polling.
type cqMentionRow struct {
	ID         string
	Content    string
	SenderName string
	CreatedAt  string
}

// PollCqMentions returns empty when c1_messenger is disabled.
func (h *C1Handler) PollCqMentions(_ string, _ int) ([]cqMentionRow, error) { return nil, nil }

// ClaimMessage returns false when c1_messenger is disabled.
func (h *C1Handler) ClaimMessage(_, _ string) (bool, error) { return false, nil }

// Search is a no-op when c1_messenger is disabled.
func (h *C1Handler) Search(_, _, _ string, _ int) (map[string]any, error) { return nil, nil }

// CheckMentions is a no-op when c1_messenger is disabled.
func (h *C1Handler) CheckMentions(_ string) ([]map[string]any, error) { return nil, nil }

// GetBriefing is a no-op when c1_messenger is disabled.
func (h *C1Handler) GetBriefing() (map[string]any, error) { return nil, nil }

// SendMessage is a no-op when c1_messenger is disabled.
func (h *C1Handler) SendMessage(_, _, _ string, _ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

// ContextKeeper is a placeholder type when c1_messenger is disabled.
type ContextKeeper struct {
	C1 *C1Handler
}

// NewContextKeeper returns nil when c1_messenger is disabled.
func NewContextKeeper(_ *C1Handler, _ any) *ContextKeeper { return nil }

// EnsureSystemChannels is a no-op when c1_messenger is disabled.
func (k *ContextKeeper) EnsureSystemChannels() error { return nil }

// AutoPost is a no-op when c1_messenger is disabled.
func (k *ContextKeeper) AutoPost(_, _ string) error { return nil }

// UpdateChannelSummary is a no-op when c1_messenger is disabled.
func (k *ContextKeeper) UpdateChannelSummary(_ string) error { return nil }

// EnsureChannel is a no-op when c1_messenger is disabled.
func (k *ContextKeeper) EnsureChannel(_, _, _ string) (string, error) { return "", nil }
