package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// seenEntry records when a completed job was first observed.
type seenEntry struct {
	CompletedAt time.Time `json:"completed_at"`
}

// C1Notifier is an optional interface for sending C1 Messenger notifications.
// Pass nil to disable notifications.
type C1Notifier interface {
	SendMessage(ctx context.Context, msg string) error
}

// knowledgeHubPollerConfig holds configuration for knowledgeHubPoller.
type knowledgeHubPollerConfig struct {
	HubURL       string
	APIKey       string
	APIPrefix    string
	Store        *knowledge.Store
	SeenPath     string
	PollInterval time.Duration
	C1           C1Notifier // optional; nil = disabled
}

// knowledgeHubPoller polls C5 Hub for completed jobs and records
// stdout KEY=VALUE metrics as knowledge.TypeExperiment documents.
// It implements serve.Component.
type knowledgeHubPoller struct {
	cfg    knowledgeHubPollerConfig
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status string // "ok" | "degraded" | "error"
	detail string
}

// compile-time interface assertion
var _ serve.Component = (*knowledgeHubPoller)(nil)

func newKnowledgeHubPoller(cfg knowledgeHubPollerConfig) *knowledgeHubPoller {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &knowledgeHubPoller{cfg: cfg, status: "ok"}
}

func (p *knowledgeHubPoller) Name() string { return "hub-knowledge-poller" }

func (p *knowledgeHubPoller) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.loop(ctx2)
	return nil
}

func (p *knowledgeHubPoller) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
		if p.done != nil {
			<-p.done
		}
	}
	return nil
}

func (p *knowledgeHubPoller) Health() serve.ComponentHealth {
	p.mu.Lock()
	defer p.mu.Unlock()
	return serve.ComponentHealth{Status: p.status, Detail: p.detail}
}

func (p *knowledgeHubPoller) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *knowledgeHubPoller) poll(ctx context.Context) {
	client := hub.NewClient(hub.HubConfig{
		URL:       p.cfg.HubURL,
		APIPrefix: p.cfg.APIPrefix,
		APIKey:    p.cfg.APIKey,
	})

	jobs, err := client.ListJobsCtx(ctx, "completed")
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

		logsResp, err := client.GetJobLogsCtx(ctx, id, 0, 1000)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hub-knowledge-poller: get logs for job %s: %v\n", id, err)
			continue
		}

		metrics := parseHubMetrics(logsResp.Lines)

		title := job.Name
		if title == "" {
			title = id
		}
		var parts []string
		for k, v := range metrics {
			parts = append(parts, k+"="+v)
		}
		body := fmt.Sprintf("job: %s\nstatus: %s\n", id, job.Status)
		if len(parts) > 0 {
			body += "metrics: " + strings.Join(parts, ", ") + "\n"
		}
		if len(job.Tags) > 0 {
			body += "tags: " + strings.Join(job.Tags, ", ") + "\n"
		}

		meta := map[string]any{
			"title":  title,
			"domain": "experiment",
		}
		if len(job.Tags) > 0 {
			meta["tags"] = job.Tags
		}

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

		seenIDs[id] = seenEntry{CompletedAt: time.Now()}
	}

	// TTL cleanup: remove entries older than 30 days.
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for id, entry := range seenIDs {
		if entry.CompletedAt.Before(cutoff) {
			delete(seenIDs, id)
		}
	}

	if saveErr := p.saveSeenIDs(seenIDs); saveErr != nil {
		fmt.Fprintf(os.Stderr, "hub-knowledge-poller: save seen IDs: %v\n", saveErr)
	}

	p.mu.Lock()
	p.status = "ok"
	p.detail = ""
	p.mu.Unlock()
}

// parseHubMetrics parses KEY=VALUE lines from job stdout logs.
// Lines without '=' are ignored. For duplicate keys, the last value wins.
func parseHubMetrics(lines []string) map[string]string {
	result := make(map[string]string)
	for _, line := range lines {
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			result[key] = val
		}
	}
	return result
}

func (p *knowledgeHubPoller) loadSeenIDs() map[string]seenEntry {
	data, err := os.ReadFile(p.cfg.SeenPath)
	if err != nil {
		return make(map[string]seenEntry)
	}
	var m map[string]seenEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]seenEntry)
	}
	return m
}

func (p *knowledgeHubPoller) saveSeenIDs(m map[string]seenEntry) error {
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
