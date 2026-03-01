// Package conversation provides platform-agnostic conversation history storage.
// It supports both an in-memory TTL-based store (MemoryStore) and a Supabase-backed
// persistent store (SupabaseStore).
package conversation

import "context"

// Message is a single chat turn with a role and content.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Channel describes a c1_channels row for bot/event channels.
type Channel struct {
	TenantID    string // defaults to "default"
	ProjectID   string // optional; empty for system-level bot channels
	Name        string // unique within (tenant_id, platform) for bot/event channels
	ChannelType string // "bot", "event", "general", "session", "dm", etc.
	Platform    string // "" (native CQ), "dooray", "discord", "slack"
}

// Participant describes a c1_members row for external-platform identities.
type Participant struct {
	TenantID    string
	ProjectID   string // optional
	MemberType  string // "user", "agent", "system"
	ExternalID  string
	DisplayName string
	Platform    string // "" (native), "dooray", "session"
	PlatformID  string // external platform identifier (e.g. Dooray user ID)
}

// Store is the interface for reading and writing conversation history.
// Implementations must be safe for concurrent use.
type Store interface {
	// Get returns the most recent limit messages for the given channel,
	// ordered oldest-first (suitable for LLM context).
	// channelID is the c1_channels UUID returned by EnsureChannel.
	Get(ctx context.Context, channelID string, limit int) ([]Message, error)

	// Append adds msgs to the history for the given channel.
	// channelID is the c1_channels UUID returned by EnsureChannel.
	// platform and projectID are metadata stored alongside the messages.
	Append(ctx context.Context, channelID, platform, projectID string, msgs []Message) error

	// EnsureChannel creates or retrieves a channel, returning its string ID.
	// Bot/event channels are keyed by (tenant_id, platform, name).
	EnsureChannel(ctx context.Context, ch Channel) (string, error)

	// ListChannels returns channels for the given tenant and optional project.
	// Pass projectID="" to list all channels for the tenant.
	ListChannels(ctx context.Context, tenantID, projectID string) ([]Channel, error)

	// EnsureParticipant creates or retrieves a member, returning its string ID.
	// External-platform members are keyed by (tenant_id, platform, platform_id).
	EnsureParticipant(ctx context.Context, p Participant) (string, error)

	// Cleanup removes expired or stale entries.
	Cleanup()
}
