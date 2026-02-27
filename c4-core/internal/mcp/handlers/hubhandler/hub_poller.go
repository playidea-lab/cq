//go:build c5_hub

package hubhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
)

// HubPoller periodically polls Hub for job status changes and publishes events.
type HubPoller struct {
	client    *hub.Client
	pub       eventbus.Publisher
	interval  time.Duration
	projectID string
	maxJobs   int // max jobs to fetch per poll (default 200)

	mu       sync.Mutex
	lastSeen map[string]string // job ID → last known status
}

// NewHubPoller creates a new HubPoller. interval defaults to 30s if <= 0.
func NewHubPoller(client *hub.Client, pub eventbus.Publisher, interval time.Duration, opts ...HubPollerOption) *HubPoller {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	p := &HubPoller{
		client:   client,
		pub:      pub,
		interval: interval,
		maxJobs:  200,
		lastSeen: make(map[string]string),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// HubPollerOption is a functional option for HubPoller.
type HubPollerOption func(*HubPoller)

// WithMaxJobs sets the maximum number of jobs fetched per poll. Default is 200.
func WithMaxJobs(n int) HubPollerOption {
	return func(p *HubPoller) {
		if n > 0 {
			p.maxJobs = n
		}
	}
}

// SetProjectID sets the project ID used when publishing events.
func (p *HubPoller) SetProjectID(projectID string) {
	p.projectID = projectID
}

// Start begins polling in a goroutine. Stops when ctx is cancelled.
func (p *HubPoller) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll()
			}
		}
	}()
}

// poll fetches RUNNING jobs and detects terminal-state transitions.
func (p *HubPoller) poll() {
	jobs, err := p.client.ListJobs("RUNNING", p.maxJobs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: hub_poller: list running jobs: %v\n", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Build current running set
	currentRunning := make(map[string]bool)
	for _, j := range jobs {
		id := j.GetID()
		if id == "" {
			continue
		}
		currentRunning[id] = true
		if _, known := p.lastSeen[id]; !known {
			p.lastSeen[id] = j.Status
		}
	}

	// Detect jobs that were running but are now gone → fetch their final status
	for id, prevStatus := range p.lastSeen {
		if prevStatus != "RUNNING" {
			continue
		}
		if currentRunning[id] {
			continue
		}
		// Job was running but is no longer in RUNNING list — fetch final status
		finalJob, err := p.client.GetJob(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cq: hub_poller: get job %s: %v\n", id, err)
			continue
		}
		newStatus := finalJob.Status
		p.publishTransition(finalJob, prevStatus, newStatus)
		// Delete terminal entries from lastSeen to prevent unbounded growth.
		// SUCCEEDED/FAILED jobs will never reappear in the RUNNING list.
		if newStatus == "SUCCEEDED" || newStatus == "FAILED" {
			delete(p.lastSeen, id)
		} else {
			p.lastSeen[id] = newStatus
		}
	}
}

func (p *HubPoller) publishTransition(job *hub.Job, prevStatus, newStatus string) {
	id := job.GetID()
	var evType string
	switch newStatus {
	case "SUCCEEDED":
		evType = "hub.job.completed"
	case "FAILED":
		evType = "hub.job.failed"
	default:
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"job_id":      id,
		"name":        job.Name,
		"status":      newStatus,
		"prev_status": prevStatus,
	})
	p.pub.PublishAsync(evType, "c4.hub", payload, p.projectID)
}
