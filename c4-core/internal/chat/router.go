package chat

import (
	"context"
	"fmt"
	"time"
)

// Router is a bridge that posts messages to a fixed c1_messages channel.
// It wraps a Client and is used by MCP handlers (c4_notify, c4_mail_send)
// to mirror messages into the shared chat channel.
//
// Router is safe for concurrent use. Post is a best-effort operation:
// errors are returned but never fatal to the calling handler.
type Router struct {
	client    *Client
	channelID string
}

// NewRouter returns a Router that posts to the given channelID via client.
// channelID must be a non-empty Supabase UUID for the target c1_messages channel.
func NewRouter(client *Client, channelID string) (*Router, error) {
	if client == nil {
		return nil, fmt.Errorf("chat: router: client is required")
	}
	if channelID == "" {
		return nil, fmt.Errorf("chat: router: channelID is required")
	}
	return &Router{client: client, channelID: channelID}, nil
}

// Post inserts content into c1_messages with the given senderType.
// senderType should be "agent", "user", or "system".
// Uses a 10-second timeout. Returns an error on failure (non-fatal to callers).
func (r *Router) Post(senderType, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.client.SendMessage(ctx, r.channelID, content, senderType)
}
