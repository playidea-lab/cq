//go:build research

package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
)

// =============================================================================
// simulation helpers
// =============================================================================

// simulatedHub: job status externally controlled mock Hub
type simulatedHub struct {
	jobs map[string]*hub.Job
	t    *testing.T
}

func newSimulatedHub(t *testing.T) *simulatedHub {
	t.Helper()
	return &simulatedHub{jobs: make(map[string]*hub.Job), t: t}
}

func (h *simulatedHub) GetJob(jobID string) (*hub.Job, error) {
	if j, ok := h.jobs[jobID]; ok {
		return j, nil
	}
	return &hub.Job{ID: jobID, Status: "RUNNING"}, nil
}

func (h *simulatedHub) SubmitJob(req *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error) {
	newID := fmt.Sprintf("job-sim-%d", len(h.jobs)+1)
	h.jobs[newID] = &hub.Job{ID: newID, Status: "RUNNING"}
	h.t.Logf("  [Hub] job submitted: %s (hypothesis=%s)", newID, req.Env["C4_HYPOTHESIS_ID"])
	return &hub.JobSubmitResponse{JobID: newID, Status: "QUEUED"}, nil
}

func (h *simulatedHub) completeJob(jobID string, status string) {
	if j, ok := h.jobs[jobID]; ok {
		j.Status = status
		h.t.Logf("  [Hub] job completed: %s -> %s", jobID, status)
	}
}

// simulatedNotifier: captures notification events
type simulatedNotifier struct {
	events []string
	t      *testing.T
}

func (n *simulatedNotifier) Notify(ctx context.Context, event, title, body string) error {
	msg := fmt.Sprintf("[%s] %s", event, body)
	n.events = append(n.events, msg)
	n.t.Logf("  [Notify] %s", msg)
	return nil
}

// =============================================================================
// TestResearchLoop_FullSimulation
// =============================================================================
func TestResearchLoop_FullSimulation(t *testing.T) {
	dir := t.TempDir()
	kStore := mustNewHypothesisStore(t)
	simHub := newSimulatedHub(t)
	notifier := &simulatedNotifier{t: t}

	// LLM response scenarios (each round: Optimizer+Skeptic+Synthesis = 3 responses)
	roundResponses := [][]string{
		// Round 0: approved
		{
			"DIRECTION: scale up\nRATIONALE: need more data\nNEXT_HYPOTHESIS: scale training data 10x",
			"CHALLENGE: compute cost\nALTERNATIVE: data quality\nVERDICT: approved",
			`{"verdict":"approved","next_hypothesis_draft":"scale training data 10x"}`,
		},
		// Round 1: null_result
		{
			"DIRECTION: pivot\nRATIONALE: low signal\nNEXT_HYPOTHESIS: try smaller model",
			"CHALLENGE: still noisy\nALTERNATIVE: different dataset\nVERDICT: null_result",
			`{"verdict":"null_result","next_hypothesis_draft":"try smaller model"}`,
		},
		// Round 2: null_result → ExploreFlag
		{
			"DIRECTION: explore\nRATIONALE: stuck\nNEXT_HYPOTHESIS: random search",
			"CHALLENGE: expensive\nALTERNATIVE: bayesian\nVERDICT: null_result",
			`{"verdict":"null_result","next_hypothesis_draft":"random search"}`,
		},
		// Round 3: approved + budget gate (MaxIterations=4)
		{
			"DIRECTION: finalize\nRATIONALE: good results\nNEXT_HYPOTHESIS: final hypothesis",
			"CHALLENGE: none\nALTERNATIVE: none\nVERDICT: approved",
			`{"verdict":"approved","next_hypothesis_draft":"final hypothesis"}`,
		},
	}

	t.Log("=== Research Loop Simulation Start ===")

	// Create initial hypothesis
	hypID, err := kStore.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "initial hypothesis: optimize attention mechanism",
		"status": "approved",
	}, "initial body")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}
	t.Logf("\n[Setup] Initial hypothesis ID: %s", hypID)

	// Register initial job
	initialJobID := "job-sim-initial"
	simHub.jobs[initialJobID] = &hub.Job{ID: initialJobID, Status: "RUNNING"}

	// LoopOrchestrator config (gate=50ms, poll=20ms)
	o := &LoopOrchestrator{
		cfg:    Config{ExploreThreshold: 2, PollInterval: 20 * time.Millisecond},
		Caller: &mockDebateLLM{responses: flattenResponses(roundResponses)},
		Store:  &testDebateStore{s: kStore},
		HubCli: &mockLoopHubClient{submitJobFunc: func(_ context.Context, req LoopHubJobRequest) (string, error) {
			resp, err := simHub.SubmitJob(&hub.JobSubmitRequest{Env: map[string]string{"C4_HYPOTHESIS_ID": req.HypothesisID}})
			if err != nil {
				return "", err
			}
			return resp.JobID, nil
		}},
		Lineage: &mockLoopLineageBuilder{buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) { return "", nil }},
		KStore:  kStore,
		Gate:    NewGateController(50 * time.Millisecond),
		State:   NewStateYAMLWriter(filepath.Join(dir, ".c9")),
		Notify:  NewNotifyBridge(notifier, 0),
	}

	ctx := context.Background()

	// Start session
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         initialJobID,
		Round:         0,
		MaxIterations: 2, // approved 2x(R=2) -> completed
		Status:        "running",
	}
	if err := o.StartLoop(ctx, session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// --- Round 0: approved ---
	t.Log("\n--- Round 0: approved ---")
	simHub.completeJob(initialJobID, "SUCCEEDED")
	jobStatus0 := &HubJobStatus{JobID: initialJobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, o.GetLoop(hypID), jobStatus0); err != nil {
		t.Fatalf("Round 0 onJobDone: %v", err)
	}

	var got0 *LoopSession
	o.Sessions.Range(func(_, v any) bool { got0 = v.(*LoopSession); return true })
	if got0 == nil {
		t.Fatal("Round 0: no session")
	}
	assertInt(t, "Round", got0.Round, 1)
	assertInt(t, "NullResultCount", got0.NullResultCount, 0)
	assertBool(t, "ExploreFlag", got0.ExploreFlag, false)

	// --- Round 1: null_result ---
	t.Log("\n--- Round 1: null_result ---")
	o.Caller = &mockDebateLLM{responses: flattenResponses(roundResponses[1:])}

	simHub.completeJob(got0.JobID, "SUCCEEDED")
	jobStatus1 := &HubJobStatus{JobID: got0.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got0, jobStatus1); err != nil {
		t.Fatalf("Round 1 onJobDone: %v", err)
	}
	got1 := o.GetLoop(got0.HypothesisID)
	if got1 == nil {
		t.Fatal("Round 1: no session")
	}
	assertInt(t, "NullResultCount", got1.NullResultCount, 1)
	assertBool(t, "ExploreFlag", got1.ExploreFlag, false)

	// --- Round 2: null_result -> ExploreFlag ---
	t.Log("\n--- Round 2: null_result -> ExploreFlag ---")
	o.Caller = &mockDebateLLM{responses: flattenResponses(roundResponses[2:])}

	simHub.completeJob(got1.JobID, "SUCCEEDED")
	jobStatus2 := &HubJobStatus{JobID: got1.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got1, jobStatus2); err != nil {
		t.Fatalf("Round 2 onJobDone: %v", err)
	}
	got2 := o.GetLoop(got1.HypothesisID)
	if got2 == nil {
		t.Fatal("Round 2: no session")
	}
	assertInt(t, "NullResultCount", got2.NullResultCount, 2)
	assertBool(t, "ExploreFlag", got2.ExploreFlag, true)

	// --- Round 3: approved + budget gate ---
	t.Log("\n--- Round 3: approved + budget gate ---")
	o.Caller = &mockDebateLLM{responses: flattenResponses(roundResponses[3:])}

	simHub.completeJob(got2.JobID, "SUCCEEDED")
	jobStatus3 := &HubJobStatus{JobID: got2.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got2, jobStatus3); err != nil {
		t.Fatalf("Round 3 onJobDone: %v", err)
	}

	var got3 *LoopSession
	o.Sessions.Range(func(_, v any) bool {
		s := v.(*LoopSession)
		if s.Status == "completed" {
			got3 = s
		}
		return true
	})
	if got3 == nil {
		t.Fatal("Round 3: no completed session")
	}
	assertInt(t, "Round", got3.Round, 2)
	assertString(t, "Status", got3.Status, "completed")

	t.Log("\n=== Simulation Complete ===")
}

// =============================================================================
// TestResearchLoop_EscalateSimulation
// =============================================================================
func TestResearchLoop_EscalateSimulation(t *testing.T) {
	kStore := mustNewHypothesisStore(t)

	llmResponses := []string{
		"DIRECTION: stop\nRATIONALE: critical failure\nNEXT_HYPOTHESIS: none",
		"CHALLENGE: unrecoverable\nALTERNATIVE: none\nVERDICT: escalate",
		`{"verdict":"escalate","next_hypothesis_draft":""}`,
	}

	o := &LoopOrchestrator{
		cfg:     Config{ExploreThreshold: 2},
		Caller:  &mockDebateLLM{responses: llmResponses},
		Store:   &testDebateStore{s: kStore},
		HubCli:  &mockLoopHubClient{submitJobFunc: func(_ context.Context, _ LoopHubJobRequest) (string, error) { return "job-x", nil }},
		Lineage: &mockLoopLineageBuilder{buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) { return "", nil }},
		KStore:  kStore,
	}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-esc",
		Round:         2,
		MaxIterations: 10,
		Status:        "running",
	}

	jobStatus := &HubJobStatus{JobID: "job-esc", Status: "failed"}
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("session not found after escalate")
	}
	// Escalate submits reasoning job and sets waiting_reasoning.
	assertString(t, "Status", got.Status, "waiting_reasoning")
}

// =============================================================================
// TestResearchLoop_StopLoop_ConcurrentSafety
// =============================================================================
func TestResearchLoop_StopLoop_ConcurrentSafety(t *testing.T) {
	o := &LoopOrchestrator{
		cfg: Config{},
	}

	ctx := context.Background()

	// Register 10 sessions in parallel
	for i := 0; i < 10; i++ {
		hypID := fmt.Sprintf("hyp-concurrent-%d", i)
		sess := &LoopSession{HypothesisID: hypID, JobID: fmt.Sprintf("job-%d", i)}
		if err := o.StartLoop(ctx, sess); err != nil {
			t.Fatalf("StartLoop %d: %v", i, err)
		}
	}

	// Run Steer and StopLoop concurrently (no data race expected)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			hypID := fmt.Sprintf("hyp-concurrent-%d", i)
			_ = o.Steer(ctx, hypID, fmt.Sprintf("guidance-%d", i))
		}
	}()
	for i := 0; i < 10; i++ {
		hypID := fmt.Sprintf("hyp-concurrent-%d", i)
		_ = o.StopLoop(ctx, hypID)
	}
	<-done

	allStopped := true
	o.Sessions.Range(func(_, v any) bool {
		s := v.(*LoopSession)
		if s.Status != "stopped" {
			allStopped = false
		}
		return true
	})
	if !allStopped {
		t.Error("some sessions are not stopped after concurrent Steer+StopLoop")
	}
}

// =============================================================================
// helpers
// =============================================================================

func flattenResponses(rounds [][]string) []string {
	var out []string
	for _, r := range rounds {
		out = append(out, r...)
	}
	return out
}

func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("  %s = %d, want %d", name, got, want)
	}
}

func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("  %s = %v, want %v", name, got, want)
	}
}

func assertString(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("  %s = %q, want %q", name, got, want)
	}
}

// Notifier interface check for simulatedNotifier
var _ Notifier = (*simulatedNotifier)(nil)

// unused import guard
var _ = strings.Contains
