package harness

import (
	"context"

	"github.com/changmin/c4-core/internal/c1push"
)

// ChannelPusher is the interface JournalWatcher uses to push messages.
// It is satisfied by *c1push.Pusher and by mock implementations in tests.
type ChannelPusher interface {
	EnsureChannel(ctx context.Context, tenantID, projectID, name string, platform c1push.Platform) (string, error)
	AppendMessages(ctx context.Context, channelID string, msgs []c1push.PushMessage) error
}

// SyncPusher wraps a ChannelPusher with per-file channel ID caching and
// handles the EnsureChannel → AppendMessages flow for a single file push.
type SyncPusher struct {
	pusher       ChannelPusher
	channelCache map[string]string // filePath → channelID
	tenantID     string
}

// newSyncPusher creates a SyncPusher.
func newSyncPusher(pusher ChannelPusher, tenantID string) *SyncPusher {
	return &SyncPusher{
		pusher:       pusher,
		channelCache: make(map[string]string),
		tenantID:     tenantID,
	}
}

// Push ensures the channel for filePath exists and appends msgs to it.
func (s *SyncPusher) Push(ctx context.Context, filePath string, msgs []c1push.PushMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	channelID, err := s.ensureChannel(ctx, filePath)
	if err != nil {
		return err
	}
	if channelID == "" {
		return nil
	}
	return s.pusher.AppendMessages(ctx, channelID, msgs)
}

func (s *SyncPusher) ensureChannel(ctx context.Context, filePath string) (string, error) {
	if id, ok := s.channelCache[filePath]; ok {
		return id, nil
	}
	channelName := filePathToChannelName(filePath)
	id, err := s.pusher.EnsureChannel(ctx, s.tenantID, "", channelName, c1push.PlatformClaudeCode)
	if err != nil {
		return "", err
	}
	s.channelCache[filePath] = id
	return id, nil
}
