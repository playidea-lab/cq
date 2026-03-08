package secrets

import "context"

// CloudSyncer is an optional interface for syncing secrets to/from a remote store.
// Set SetCloud on Store to enable cloud sync; nil means local-only mode.
type CloudSyncer interface {
	Set(ctx context.Context, projectID, key, value string) error
	Get(ctx context.Context, projectID, key string) (string, error)
	ListKeys(ctx context.Context, projectID string) ([]string, error)
	Delete(ctx context.Context, projectID, key string) error
}
