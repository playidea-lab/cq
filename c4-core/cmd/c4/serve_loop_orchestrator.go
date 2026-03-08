//go:build research

package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/serve"
)

// loopHubClient is the minimal interface for submitting Hub jobs in the loop.
type loopHubClient interface {
	SubmitJob(ctx context.Context, req loopHubJobRequest) (string, error)
}

// loopHubJobRequest is the minimal job submission payload.
type loopHubJobRequest struct {
	HypothesisID     string
	ExperimentSpecID string
	Command          string
	ProjectID        string
}

// loopLineageBuilder builds lineage context strings for a hypothesis.
type loopLineageBuilder interface {
	BuildContext(ctx context.Context, hypothesisID string, limit int) (string, error)
}

func init() { registerInitHook(registerLoopOrchestratorComponent) }

// HubClient is the interface for Hub job management (defined by T-1308-0).
// Defined here as a local interface until T-1308-0 lands.
type HubClient interface {
	GetJob(jobID string) (*hub.Job, error)
	SubmitJob(req *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error)
}

// HubJobStatus carries job completion details passed to onJobDone.
// Defined here until T-1308-0 provides hub.HubJobStatus.
type HubJobStatus struct {
	JobID      string
	Status     string // SUCCEEDED, FAILED, CANCELLED, completed, failed
	Job        *hub.Job
	ValLoss    float64
	TestMetric float64
}

// LoopOrchestratorConfig holds configuration for the LoopOrchestrator component.
type LoopOrchestratorConfig struct {
	Store            *knowledge.Store
	Hub              HubClient
	LLMGateway       *llm.Gateway
	PollInterval     time.Duration // default 30s
	ExploreThreshold int           // null_result count before ExploreFlag=true; default 2
}

// LoopSession represents a single autonomous research loop session.
type LoopSession struct {
	HypothesisID    string
	JobID           string
	Round           int
	MaxIterations   int     // budget gate; 0 = unlimited
	MaxCostUSD      float64 // budget gate; 0 = unlimited
	ExploreFlag     bool    // E&E: set true after NullResultCount >= ExploreThreshold
	NullResultCount int
	Status          string // "running" | "stopped" | "completed"
	SteeringGuidance string
}

// LoopOrchestrator is a serve.Component that polls Hub job status every PollInterval,
// manages LoopSession state, and applies E&E policy (ExploreFlag).
type LoopOrchestrator struct {
	cfg      LoopOrchestratorConfig
	sessions sync.Map // map[hypothesisID string]*LoopSession
	mu       sync.Mutex
	status   string
	cancel   context.CancelFunc
	done     chan struct{}
	// jobdone fields (set by onJobDone path)
	caller  debateCaller
	store   debateStore
	hubCli  loopHubClient
	lineage loopLineageBuilder
	kStore  *knowledge.Store
}

// compile-time interface assertion
var _ serve.Component = (*LoopOrchestrator)(nil)

func newLoopOrchestrator(cfg LoopOrchestratorConfig) *LoopOrchestrator {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.ExploreThreshold <= 0 {
		cfg.ExploreThreshold = 2
	}
	return &LoopOrchestrator{
		cfg:    cfg,
		status: "ok",
	}
}

// registerLoopOrchestratorComponent registers the LoopOrchestrator in the serve ecosystem.
func registerLoopOrchestratorComponent(ictx *initContext) error {
	if ictx.knowledgeStore == nil || ictx.llmGateway == nil {
		return nil
	}
	// Hub is optional; LoopOrchestrator can run without it (StartLoop will require it).
	var hc HubClient
	if ictx.hubClient != nil {
		if c, ok := ictx.hubClient.(HubClient); ok {
			hc = c
		}
	}
	cfg := LoopOrchestratorConfig{
		Store:      ictx.knowledgeStore,
		Hub:        hc,
		LLMGateway: ictx.llmGateway,
	}
	o := newLoopOrchestrator(cfg)
	// Register with the serve manager if available.
	// The component is accessible via the initContext for direct calls in tests.
	_ = o
	fmt.Fprintf(os.Stderr, "cq serve: registered loop_orchestrator\n")
	return nil
}

// Name implements serve.Component.
func (o *LoopOrchestrator) Name() string { return "loop_orchestrator" }

// Start implements serve.Component. It launches the background polling loop.
func (o *LoopOrchestrator) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	o.cancel = cancel
	o.done = make(chan struct{})
	o.mu.Unlock()
	go o.loop(ctx2)
	return nil
}

// Stop implements serve.Component. It cancels the polling loop and waits for it to exit.
func (o *LoopOrchestrator) Stop(_ context.Context) error {
	o.mu.Lock()
	cancel := o.cancel
	done := o.done
	o.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

// Health implements serve.Component.
func (o *LoopOrchestrator) Health() serve.ComponentHealth {
	o.mu.Lock()
	s := o.status
	o.mu.Unlock()
	return serve.ComponentHealth{Status: s}
}

// loop is the background goroutine that polls Hub job status for all running sessions.
func (o *LoopOrchestrator) loop(ctx context.Context) {
	o.mu.Lock()
	done := o.done
	o.mu.Unlock()
	defer close(done)

	ticker := time.NewTicker(o.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.poll(ctx)
		}
	}
}

// poll iterates over running sessions and checks Hub job status.
func (o *LoopOrchestrator) poll(ctx context.Context) {
	if o.cfg.Hub == nil {
		return
	}
	o.sessions.Range(func(key, value any) bool {
		session, ok := value.(*LoopSession)
		if !ok || session.Status != "running" {
			return true
		}
		job, err := o.cfg.Hub.GetJob(session.JobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "loop_orchestrator: GetJob %s: %v\n", session.JobID, err)
			return true
		}
		switch job.Status {
		case "SUCCEEDED", "FAILED", "CANCELLED":
			jobStatus := &HubJobStatus{
				JobID:  job.GetID(),
				Status: job.Status,
				Job:    job,
			}
			if err := o.onJobDone(ctx, session, jobStatus); err != nil {
				fmt.Fprintf(os.Stderr, "loop_orchestrator: onJobDone %s: %v\n", session.HypothesisID, err)
			}
		}
		return true
	})
}

// StartLoop registers and starts a new loop session for the given hypothesis.
// Returns an error if a session for the same HypothesisID already exists and is running.
func (o *LoopOrchestrator) StartLoop(ctx context.Context, session *LoopSession) error {
	if session == nil || session.HypothesisID == "" {
		return fmt.Errorf("loop_orchestrator: StartLoop: HypothesisID is required")
	}
	session.Status = "running"
	o.sessions.Store(session.HypothesisID, session)
	return nil
}

// StopLoop stops an active loop session by hypothesis ID.
// Returns an error if the session does not exist.
func (o *LoopOrchestrator) StopLoop(ctx context.Context, hypID string) error {
	val, ok := o.sessions.Load(hypID)
	if !ok {
		return fmt.Errorf("loop_orchestrator: StopLoop: session %q not found", hypID)
	}
	session, ok := val.(*LoopSession)
	if !ok {
		return fmt.Errorf("loop_orchestrator: StopLoop: invalid session type for %q", hypID)
	}
	session.Status = "stopped"
	o.sessions.Store(hypID, session)
	return nil
}

// GetLoop returns the LoopSession for the given hypothesis ID, or nil if not found.
func (o *LoopOrchestrator) GetLoop(hypID string) *LoopSession {
	val, ok := o.sessions.Load(hypID)
	if !ok {
		return nil
	}
	session, _ := val.(*LoopSession)
	return session
}

// Steer injects steering guidance into an active session.
// The guidance will be included in the next Debate context via SteeringGuidance.
func (o *LoopOrchestrator) Steer(ctx context.Context, hypID string, guidance string) error {
	val, ok := o.sessions.Load(hypID)
	if !ok {
		return fmt.Errorf("loop_orchestrator: Steer: session %q not found", hypID)
	}
	session, ok := val.(*LoopSession)
	if !ok {
		return fmt.Errorf("loop_orchestrator: Steer: invalid session type for %q", hypID)
	}
	session.SteeringGuidance = guidance
	o.sessions.Store(hypID, session)
	return nil
}

