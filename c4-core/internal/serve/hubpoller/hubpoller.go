package hubpoller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// hubPollerLogLimit is the max log lines fetched per job.
// HasMore is intentionally ignored: we capture the first N lines of stdout
// (enough for KEY=VALUE metric lines) and skip pagination to keep polling simple.
const hubPollerLogLimit = 1000

// SeenEntry records when a completed job was first observed.
type SeenEntry struct {
	CompletedAt time.Time `json:"completed_at"`
}

// C1Notifier is an optional interface for sending C1 Messenger notifications.
// Pass nil to disable notifications.
type C1Notifier interface {
	SendMessage(ctx context.Context, msg string) error
}

// Config holds configuration for KnowledgeHubPoller.
type Config struct {
	HubURL       string
	APIKey       string
	APIKeyEnv    string
	APIPrefix    string
	SupabaseURL  string
	SupabaseKey  string
	Store        *knowledge.Store
	SeenPath     string
	PollInterval time.Duration
	C1           C1Notifier // optional; nil = disabled
}

// KnowledgeHubPoller polls C5 Hub for completed jobs and records
// stdout KEY=VALUE metrics as knowledge.TypeExperiment documents.
// It implements serve.Component.
type KnowledgeHubPoller struct {
	cfg    Config
	client *hub.Client // created once in New; reused across polls
	wake   chan struct{} // optional; signals immediate poll (set via SetWakeChannel)
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status string // "ok" | "degraded" | "error"
	detail string
}

// compile-time interface assertion
var _ serve.Component = (*KnowledgeHubPoller)(nil)

func New(cfg Config) *KnowledgeHubPoller {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	client := hub.NewClient(hub.HubConfig{
		URL:         cfg.HubURL,
		APIPrefix:   cfg.APIPrefix,
		APIKey:      cfg.APIKey,
		APIKeyEnv:   cfg.APIKeyEnv,
		SupabaseURL: cfg.SupabaseURL,
		SupabaseKey: cfg.SupabaseKey,
	})
	return &KnowledgeHubPoller{cfg: cfg, client: client, status: "ok"}
}

func (p *KnowledgeHubPoller) Name() string { return "hub-knowledge-poller" }

func (p *KnowledgeHubPoller) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.loop(ctx2)
	return nil
}

func (p *KnowledgeHubPoller) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
		if p.done != nil {
			<-p.done
		}
	}
	return nil
}

func (p *KnowledgeHubPoller) Health() serve.ComponentHealth {
	p.mu.Lock()
	defer p.mu.Unlock()
	return serve.ComponentHealth{Status: p.status, Detail: p.detail}
}

// SetWakeChannel sets an optional channel that triggers an immediate poll when
// signalled, without waiting for the next ticker tick.
// A nil wake channel leaves the ticker-only behavior intact (Go spec: receive on
// nil channel blocks forever, so the case is never selected).
// Must be called before Start.
func (p *KnowledgeHubPoller) SetWakeChannel(ch chan struct{}) {
	p.wake = ch
}

func (p *KnowledgeHubPoller) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		case <-p.wake:
			p.poll(ctx)
		}
	}
}

func (p *KnowledgeHubPoller) poll(ctx context.Context) {
	jobs, err := p.client.ListJobsCtx(ctx, "completed", 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub-knowledge-poller: list completed jobs: %v\n", err)
		p.mu.Lock()
		p.status = "degraded"
		p.detail = err.Error()
		p.mu.Unlock()
		return
	}

	seenIDs := p.loadSeenIDs()

	for _, job := range jobs {
		id := job.GetID()
		if id == "" {
			continue
		}
		if _, seen := seenIDs[id]; seen {
			continue
		}

		logsResp, err := p.client.GetJobLogsCtx(ctx, id, 0, hubPollerLogLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hub-knowledge-poller: get logs for job %s: %v\n", id, err)
			continue
		}

		metrics := ParseHubMetrics(logsResp.Lines)

		title := job.Name
		if title == "" {
			title = id
		}
		var parts []string
		for k, v := range metrics {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts) // deterministic order for reproducible knowledge bodies
		body := fmt.Sprintf("job: %s\nstatus: %s\n", id, job.Status)
		if len(parts) > 0 {
			body += "metrics: " + strings.Join(parts, ", ") + "\n"
		}
		if len(job.Tags) > 0 {
			body += "tags: " + string(job.Tags) + "\n"
		}

		meta := map[string]any{
			"title":  title,
			"domain": "experiment",
		}
		if len(job.Tags) > 0 {
			meta["tags"] = job.Tags
		}

		// Store.Create is idempotent per-seenIDs guard: a job is only processed
		// once (seenIDs[id] set after success), so duplicate documents are not
		// created even if the process restarts between Create and saveSeenIDs.
		_, createErr := p.cfg.Store.Create(knowledge.TypeExperiment, meta, body)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "hub-knowledge-poller: create knowledge for job %s: %v\n", id, createErr)
			continue
		}

		// Notify C1 Messenger if configured.
		if p.cfg.C1 != nil {
			msg := fmt.Sprintf("hub-poller: recorded experiment for job %s (%s)", id, title)
			if notifyErr := p.cfg.C1.SendMessage(ctx, msg); notifyErr != nil {
				fmt.Fprintf(os.Stderr, "hub-knowledge-poller: c1 notify for job %s: %v\n", id, notifyErr)
			}
		}

		seenIDs[id] = SeenEntry{CompletedAt: time.Now()}
	}

	// TTL cleanup: remove entries older than 30 days.
	CleanupSeenIDs(seenIDs, time.Now().Add(-30*24*time.Hour))

	if saveErr := p.saveSeenIDs(seenIDs); saveErr != nil {
		fmt.Fprintf(os.Stderr, "hub-knowledge-poller: save seen IDs: %v\n", saveErr)
	}

	p.mu.Lock()
	p.status = "ok"
	p.detail = ""
	p.mu.Unlock()
}

// ParseHubMetrics parses KEY=VALUE lines from job stdout logs.
// Lines without '=' are ignored. For duplicate keys, the last value wins.
// The C5 ExperimentWrapper uses @key=value protocol in stdout; the leading
// '@' is stripped so knowledge records use clean key names.
func ParseHubMetrics(lines []string) map[string]string {
	result := make(map[string]string)
	for _, line := range lines {
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		key = strings.TrimLeft(key, "@")
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			result[key] = val
		}
	}
	return result
}

func (p *KnowledgeHubPoller) loadSeenIDs() map[string]SeenEntry {
	data, err := os.ReadFile(p.cfg.SeenPath)
	if err != nil {
		return make(map[string]SeenEntry)
	}
	var m map[string]SeenEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]SeenEntry)
	}
	return m
}

// CleanupSeenIDs removes entries whose CompletedAt is before cutoff (in-place).
func CleanupSeenIDs(m map[string]SeenEntry, cutoff time.Time) {
	for id, entry := range m {
		if entry.CompletedAt.Before(cutoff) {
			delete(m, id)
		}
	}
}

func (p *KnowledgeHubPoller) saveSeenIDs(m map[string]SeenEntry) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.cfg.SeenPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "hub_poller_seen_*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(data); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return werr
	}
	if serr := tmp.Sync(); serr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return serr
	}
	tmp.Close()
	return os.Rename(tmpName, p.cfg.SeenPath)
}
