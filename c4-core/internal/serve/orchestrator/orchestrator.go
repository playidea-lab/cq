//go:build research

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
	ebpb "github.com/changmin/c4-core/internal/eventbus/pb"
)

// EventBusSubscriber is the minimal EventBus interface needed by LoopOrchestrator.
// *eventbus.Client satisfies this interface.
type EventBusSubscriber interface {
	Subscribe(ctx context.Context, pattern string, projectID string) (<-chan *ebpb.Event, error)
}

// LoopHubClient is the minimal interface for submitting Hub jobs in the loop.
type LoopHubClient interface {
	SubmitJob(ctx context.Context, req LoopHubJobRequest) (string, error)
}

// LoopHubJobRequest is the minimal job submission payload.
type LoopHubJobRequest struct {
	HypothesisID     string
	ExperimentSpecID string
	Command          string
	ProjectID        string
	Capability       string
	Params           map[string]any
}

// LoopLineageBuilder builds lineage context strings for a hypothesis.
type LoopLineageBuilder interface {
	BuildContext(ctx context.Context, hypothesisID string, limit int) (string, error)
}

// DebateCaller is the interface for LLM calls in the debate flow.
type DebateCaller interface {
	Call(ctx context.Context, system, user string) (string, error)
}

// DebateStore abstracts knowledge.Store for testability.
type DebateStore interface {
	Get(id string) (*knowledge.Document, error)
	Create(docType knowledge.DocumentType, metadata map[string]any, body string) (string, error)
}

// HubClient is the interface for Hub job management.
type HubClient interface {
	GetJob(jobID string) (*hub.Job, error)
	SubmitJob(req *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error)
}

// HubJobStatus carries job completion details passed to onJobDone.
type HubJobStatus struct {
	JobID      string
	Status     string // SUCCEEDED, FAILED, CANCELLED, completed, failed
	Job        *hub.Job
	ValLoss    float64
	TestMetric float64
}

// Config holds configuration for the LoopOrchestrator component.
type Config struct {
	Store            *knowledge.Store
	Hub              HubClient
	PollInterval     time.Duration // default 30s
	ExploreThreshold int           // null_result count before ExploreFlag=true; default 2
}

// LoopSession represents a single autonomous research loop session.
type LoopSession struct {
	HypothesisID     string
	JobID            string
	Round            int
	MaxIterations    int     // budget gate; 0 = unlimited
	MaxCostUSD       float64 // budget gate; 0 = unlimited
	ExploreFlag      bool    // E&E: set true after NullResultCount >= ExploreThreshold
	NullResultCount  int
	Status           string // "running" | "stopped" | "completed"
	SteeringGuidance string
	// Convergence tracking fields.
	MaxPatience          int     // max rounds without improvement; 0 = no convergence check
	ConvergenceThreshold float64 // minimum metric improvement to reset patience; default 0.5
	PatienceCount        int     // rounds without sufficient improvement
	BestMetric           float64 // best metric seen so far
	MetricLowerIsBetter  bool    // direction for metric comparison
}

// LoopSpecPipeline holds the dependencies for generateAndReview in onJobDone.
// When nil, spec generation is skipped and the job is submitted without an ExperimentSpecID.
type LoopSpecPipeline struct {
	Caller DebateCaller
	KStore DebateStore
}

// LoopOrchestrator is a serve.Component that polls Hub job status every PollInterval,
// manages LoopSession state, and applies E&E policy (ExploreFlag).
type LoopOrchestrator struct {
	cfg      Config
	Sessions sync.Map // map[hypothesisID string]*LoopSession
	mu       sync.Mutex
	status   string
	cancel   context.CancelFunc
	done     chan struct{}
	// integrated components (T-RLOOP-4-0/4-1)
	Gate   *GateController  // optional; released on Stop() to unblock onJobDone gate waits
	State  *StateYAMLWriter
	Notify *NotifyBridge
	// jobdone fields (set by onJobDone path)
	Caller       DebateCaller
	Store        DebateStore
	HubCli       LoopHubClient
	Lineage      LoopLineageBuilder
	KStore       *knowledge.Store
	SpecPipeline *LoopSpecPipeline // optional; when set, runs generateAndReview before Hub job submit
	// EventBus subscription (optional; nil disables push-based wake, poll fallback continues)
	EBSub EventBusSubscriber
}

// loopHubClientAdapter adapts HubClient to the LoopHubClient interface.
type loopHubClientAdapter struct{ hc HubClient }

func (a *loopHubClientAdapter) SubmitJob(ctx context.Context, req LoopHubJobRequest) (string, error) {
	resp, err := a.hc.SubmitJob(&hub.JobSubmitRequest{
		Command:    req.Command,
		ProjectID:  req.ProjectID,
		Capability: req.Capability,
		Params:     req.Params,
		Env: map[string]string{
			"C4_HYPOTHESIS_ID":      req.HypothesisID,
			"C4_EXPERIMENT_SPEC_ID": req.ExperimentSpecID,
		},
	})
	if err != nil {
		return "", err
	}
	return resp.JobID, nil
}

// NewHubClientAdapter creates a LoopHubClient from a HubClient.
func NewHubClientAdapter(hc HubClient) LoopHubClient {
	return &loopHubClientAdapter{hc: hc}
}

// compile-time interface assertion
var _ serve.Component = (*LoopOrchestrator)(nil)

// New creates a new LoopOrchestrator with the given config.
func New(cfg Config) *LoopOrchestrator {
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

// SetupComponents wires integrated sub-components. Called by the registration layer
// after New(), before Start().
func (o *LoopOrchestrator) SetupComponents(gate *GateController, state *StateYAMLWriter, notify *NotifyBridge) {
	o.Gate = gate
	o.State = state
	o.Notify = notify
}

// Name implements serve.Component.
func (o *LoopOrchestrator) Name() string { return "loop_orchestrator" }

// Start implements serve.Component. It launches the background polling loop.
// On start, it reads persisted state and resumes a gate if one was in progress.
func (o *LoopOrchestrator) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	o.cancel = cancel
	o.done = make(chan struct{})
	o.mu.Unlock()

	// Resume: if persisted state shows gate_wait, re-enter gate with remaining duration.
	if o.State != nil && o.Gate != nil {
		if s, err := o.State.ReadState(); err == nil && s.State == "gate_wait" && s.GateDeadline != nil {
			remaining := time.Until(*s.GateDeadline)
			var logMsg string
			o.mu.Lock()
			if remaining > 0 {
				o.Gate = NewGateController(remaining)
				logMsg = "research loop: resuming gate"
			} else {
				// Deadline already passed — release immediately.
				o.Gate = NewGateController(0)
				logMsg = "research loop: gate deadline expired on resume, auto-continuing"
			}
			o.mu.Unlock()
			slog.InfoContext(ctx, logMsg, "remaining", remaining)
		}
	}

	go o.loop(ctx2)
	if o.EBSub != nil {
		go o.subscribeToHub(ctx2)
	}
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
	if o.Gate != nil {
		o.Gate.Release("stop") // unblock any in-progress gate wait in onJobDone
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
// Job completion handling is deferred outside sessions.Range to avoid
// blocking the sync.Map during the gate wait (up to 24h).
//
// The done slice stores value copies of sessions captured at poll time.
// onJobDone operates on these poll-time snapshots; concurrent Steer/StopLoop
// updates to the map are not visible to the current poll cycle but will be
// picked up on the next round (by design — consistent per-round view).
func (o *LoopOrchestrator) poll(ctx context.Context) {
	if o.cfg.Hub == nil {
		return
	}
	type doneEntry struct {
		session   LoopSession // value copy at poll time; independent of sync.Map pointer
		jobStatus *HubJobStatus
	}
	var done []doneEntry
	o.Sessions.Range(func(key, value any) bool {
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
		case "SUCCEEDED", "FAILED", "CANCELLED",
			"succeeded", "failed", "cancelled":
			done = append(done, doneEntry{
				session: *session, // value copy: detached from map, safe after Range exits
				// Normalize status to lowercase for consistent comparison in onJobDone.
				jobStatus: &HubJobStatus{JobID: job.GetID(), Status: strings.ToLower(job.Status), Job: job},
			})
		}
		return true
	})
	for i := range done {
		if err := o.onJobDone(ctx, &done[i].session, done[i].jobStatus); err != nil {
			fmt.Fprintf(os.Stderr, "loop_orchestrator: onJobDone %s: %v\n", done[i].session.HypothesisID, err)
		}
	}
}

// subscribeToHub subscribes to hub.job.completed and hub.job.failed EventBus events.
// On event arrival it resolves the matching session and calls onJobDone immediately,
// eliminating the 30s poll delay. poll() remains as fallback.
//
// subscribeToHub returns on ctx cancellation or Subscribe error; errors are logged
// to stderr and the poll fallback continues unaffected (graceful degradation).
func (o *LoopOrchestrator) subscribeToHub(ctx context.Context) error {
	ch, err := o.EBSub.Subscribe(ctx, "hub.job.*", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loop_orchestrator: subscribeToHub: %v (poll fallback continues)\n", err)
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			evCopy := ev
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "loop_orchestrator: subscribeToHub: panic: %v\n", r)
					}
				}()
				o.handleHubEvent(ctx, evCopy)
			}()
		}
	}
}

// handleHubEvent processes a hub.job.completed / hub.job.failed event.
// It locates the session by JobID, performs an atomic LoadOrStore to prevent
// duplicate onJobDone calls when poll() fires concurrently, then calls onJobDone.
func (o *LoopOrchestrator) handleHubEvent(ctx context.Context, ev *ebpb.Event) {
	evType := ev.GetType()
	if evType != "hub.job.completed" && evType != "hub.job.failed" {
		return
	}

	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(ev.GetData(), &payload); err != nil || payload.JobID == "" {
		return
	}

	// Find the running session whose JobID matches.
	var matched *LoopSession
	o.Sessions.Range(func(_, value any) bool {
		s, ok := value.(*LoopSession)
		if ok && s.Status == "running" && s.JobID == payload.JobID {
			matched = s
			return false // stop iteration
		}
		return true
	})
	if matched == nil {
		return
	}

	// Atomic duplicate prevention: mark this job as being processed by EB path.
	// Use a synthetic "eb:jobID" key to check-and-store atomically.
	dedupKey := "eb:" + payload.JobID
	if _, loaded := o.Sessions.LoadOrStore(dedupKey, struct{}{}); loaded {
		return // already processed by another goroutine
	}
	// Clean up dedup key after processing.
	defer o.Sessions.Delete(dedupKey)

	status := "succeeded"
	if evType == "hub.job.failed" {
		status = "failed"
	}
	js := &HubJobStatus{JobID: payload.JobID, Status: status}
	session := *matched // value copy (consistent with poll() pattern)
	if err := o.onJobDone(ctx, &session, js); err != nil {
		fmt.Fprintf(os.Stderr, "loop_orchestrator: handleHubEvent onJobDone %s: %v\n", matched.HypothesisID, err)
	}
}

// StartLoop registers and starts a new loop session for the given hypothesis.
// Returns an error if a session for the same HypothesisID already exists and is running.
func (o *LoopOrchestrator) StartLoop(ctx context.Context, session *LoopSession) error {
	if session == nil || session.HypothesisID == "" {
		return fmt.Errorf("loop_orchestrator: StartLoop: HypothesisID is required")
	}
	session.Status = "running"
	// Initialize BestMetric to worst possible value based on metric direction.
	if session.MetricLowerIsBetter {
		session.BestMetric = math.Inf(1)
	} else {
		session.BestMetric = math.Inf(-1)
	}
	actual, loaded := o.Sessions.LoadOrStore(session.HypothesisID, session)
	if loaded {
		if existing, ok := actual.(*LoopSession); ok && existing.Status == "running" {
			return fmt.Errorf("loop_orchestrator: StartLoop: session %q already running", session.HypothesisID)
		}
		// stopped/completed session: overwrite atomically.
		o.Sessions.Store(session.HypothesisID, session)
	}
	return nil
}

// StopLoop stops an active loop session by hypothesis ID.
// Returns an error if the session does not exist.
func (o *LoopOrchestrator) StopLoop(ctx context.Context, hypID string) error {
	val, ok := o.Sessions.Load(hypID)
	if !ok {
		return fmt.Errorf("loop_orchestrator: StopLoop: session %q not found", hypID)
	}
	existing, ok := val.(*LoopSession)
	if !ok {
		return fmt.Errorf("loop_orchestrator: StopLoop: invalid session type for %q", hypID)
	}
	// Copy-on-write: mutate a copy then atomically replace to avoid data races
	// with concurrent onJobDone / Steer goroutines.
	updated := *existing
	updated.Status = "stopped"
	o.Sessions.Store(hypID, &updated)
	return nil
}

// GetLoop returns the LoopSession for the given hypothesis ID, or nil if not found.
func (o *LoopOrchestrator) GetLoop(hypID string) *LoopSession {
	val, ok := o.Sessions.Load(hypID)
	if !ok {
		return nil
	}
	session, _ := val.(*LoopSession)
	return session
}

// ReleaseGate releases the active gate immediately (human-intervene).
// Used by c4_research_intervene with action="continue".
func (o *LoopOrchestrator) ReleaseGate(reason string) {
	if o.Gate != nil {
		o.Gate.Release(reason)
	}
}

// Steer injects steering guidance into an active session.
// The guidance will be included in the next Debate context via SteeringGuidance.
func (o *LoopOrchestrator) Steer(ctx context.Context, hypID string, guidance string) error {
	val, ok := o.Sessions.Load(hypID)
	if !ok {
		return fmt.Errorf("loop_orchestrator: Steer: session %q not found", hypID)
	}
	existing, ok := val.(*LoopSession)
	if !ok {
		return fmt.Errorf("loop_orchestrator: Steer: invalid session type for %q", hypID)
	}
	// Copy-on-write: mutate a copy then atomically replace to avoid data races
	// with concurrent onJobDone / StopLoop goroutines.
	updated := *existing
	updated.SteeringGuidance = guidance
	o.Sessions.Store(hypID, &updated)
	return nil
}

// RegisterComponent registers the LoopOrchestrator with a serve.Manager and prints a message.
func (o *LoopOrchestrator) RegisterComponent(mgr *serve.Manager, projectDir string) {
	mgr.Register(o)
	fmt.Fprintf(os.Stderr, "cq serve: registered loop_orchestrator\n")
	_ = projectDir
}

// WireDebateComponents sets up the debate and hub client dependencies.
func (o *LoopOrchestrator) WireDebateComponents(caller DebateCaller, store DebateStore, kStore *knowledge.Store, hubCli LoopHubClient) {
	o.Caller = caller
	o.Store = store
	o.KStore = kStore
	o.HubCli = hubCli
}

// SetSpecPipeline configures the spec pipeline for experiment spec generation.
func (o *LoopOrchestrator) SetSpecPipeline(caller DebateCaller, store DebateStore) {
	o.SpecPipeline = &LoopSpecPipeline{
		Caller: caller,
		KStore: store,
	}
}

// SetupGate configures the gate with project directory for state persistence.
func SetupGate(gateDur time.Duration) *GateController {
	return NewGateController(gateDur)
}

// SetupState creates a StateYAMLWriter for the given project directory.
func SetupState(projectDir string) *StateYAMLWriter {
	return NewStateYAMLWriter(filepath.Join(projectDir, ".c9"))
}
