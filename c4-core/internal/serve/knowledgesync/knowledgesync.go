package knowledgesync

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

const syncInterval = 5 * time.Minute

// CloudSyncer is the subset of knowledge.CloudSyncer used by this component.
type CloudSyncer = knowledge.CloudSyncer

// KnowledgeSyncComponent pulls cloud knowledge documents to the local store
// on Start and then periodically every 5 minutes.
// Errors are logged only — they never crash serve.
type KnowledgeSyncComponent struct {
	store  *knowledge.Store
	cloud  CloudSyncer
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status string
	detail string
}

// compile-time interface assertion
var _ serve.Component = (*KnowledgeSyncComponent)(nil)

// New creates a new KnowledgeSyncComponent.
func New(store *knowledge.Store, cloud CloudSyncer) *KnowledgeSyncComponent {
	return &KnowledgeSyncComponent{
		store:  store,
		cloud:  cloud,
		status: "ok",
	}
}

func (c *KnowledgeSyncComponent) Name() string { return "knowledge-cloud-sync" }

// Start performs an initial Pull and launches the periodic sync goroutine.
func (c *KnowledgeSyncComponent) Start(ctx context.Context) error {
	// Initial pull on startup (errors logged, not fatal).
	c.pull()

	ctx2, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	go c.loop(ctx2)
	return nil
}

// Stop cancels the sync goroutine and waits for it to exit.
func (c *KnowledgeSyncComponent) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
		if c.done != nil {
			<-c.done
		}
	}
	return nil
}

// Health returns the current health status.
func (c *KnowledgeSyncComponent) Health() serve.ComponentHealth {
	c.mu.Lock()
	defer c.mu.Unlock()
	return serve.ComponentHealth{Status: c.status, Detail: c.detail}
}

func (c *KnowledgeSyncComponent) loop(ctx context.Context) {
	defer close(c.done)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.pull()
		}
	}
}

func (c *KnowledgeSyncComponent) pull() {
	result, err := knowledge.Pull(c.store, c.cloud, "", 50, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge-cloud-sync: pull failed: %v\n", err)
		c.mu.Lock()
		c.status = "degraded"
		c.detail = err.Error()
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	c.status = "ok"
	c.detail = fmt.Sprintf("pulled=%d updated=%d skipped=%d deleted=%d", result.Pulled, result.Updated, result.Skipped, result.Deleted)
	c.mu.Unlock()
}
