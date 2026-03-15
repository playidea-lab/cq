//go:build research

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ebpb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/changmin/c4-core/internal/hub"
)

// mockEventBusSubscriber implements EventBusSubscriber for tests.
type mockEventBusSubscriber struct {
	mu     sync.Mutex
	ch     chan *ebpb.Event
	subErr error
}

func newMockEBSubscriber() *mockEventBusSubscriber {
	return &mockEventBusSubscriber{ch: make(chan *ebpb.Event, 16)}
}

func (m *mockEventBusSubscriber) Subscribe(_ context.Context, _ string, _ string) (<-chan *ebpb.Event, error) {
	if m.subErr != nil {
		return nil, m.subErr
	}
	return m.ch, nil
}

func (m *mockEventBusSubscriber) send(evType, jobID, status string) {
	payload, _ := json.Marshal(map[string]any{"job_id": jobID, "status": status})
	m.ch <- &ebpb.Event{Type: evType, Data: payload}
}

// newTestOrchestratorWithEB returns an orchestrator wired with a mock EB subscriber
// and all debate components required by onJobDone.
// The mock LLM returns "escalate" verdict → onJobDone sets Status="stopped" and returns
// immediately, which avoids gate waits and hypothesis creation in unit tests.
func newTestOrchestratorWithEB(t *testing.T, pollInterval time.Duration) (*LoopOrchestrator, *mockEventBusSubscriber) {
	t.Helper()
	ebSub := newMockEBSubscriber()
	kStore := mustNewHypothesisStore(t)
	// "escalate" verdict: onJobDone submits reasoning job (or no-op if HubCli nil)
	// and writes Phase=CONFERENCE. Session remains "running" but Phase changes.
	mock := &mockDebateLLM{responses: []string{
		"DIRECTION: stop\nRATIONALE: test\nNEXT_HYPOTHESIS: none",
		"CHALLENGE: quality\nALTERNATIVE: stop\nVERDICT: escalate",
		`{"verdict":"escalate","next_hypothesis_draft":""}`,
	}}
	store := &testDebateStore{s: kStore}
	hubCli := &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, _ LoopHubJobRequest) (string, error) {
			return "job-next-001", nil
		},
	}
	lineage := &mockLoopLineageBuilder{
		buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) {
			return "", nil
		},
	}
	stateWriter := NewStateYAMLWriter(t.TempDir())
	o := &LoopOrchestrator{
		cfg: Config{
			Store:            kStore,
			Hub:              newMockHubClient(),
			PollInterval:     pollInterval,
			ExploreThreshold: 2,
		},
		Caller:  mock,
		Store:   store,
		HubCli:  hubCli,
		Lineage: lineage,
		KStore:  kStore,
		State:   stateWriter,
		status:  "ok",
		EBSub:   ebSub,
	}
	return o, ebSub
}

// TestLoopOrchestrator_EventBus_WakesOnJobComplete verifies that a hub.job.completed
// event triggers onJobDone without waiting for a poll tick.
func TestLoopOrchestrator_EventBus_WakesOnJobComplete(t *testing.T) {
	// Very long poll interval so poll never fires during the test.
	o, ebSub := newTestOrchestratorWithEB(t, 10*time.Minute)

	// Create a hypothesis document so runDebate can find it.
	hypID := mustCreateHyp(t, o.KStore)

	// Register a session with a known job ID.
	session := &LoopSession{
		HypothesisID: hypID,
		JobID:        "job-eb-001",
		Status:       "running",
	}
	if err := o.StartLoop(context.Background(), session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer o.Stop(context.Background()) //nolint:errcheck

	// Send the event (before any poll tick fires).
	ebSub.send("hub.job.completed", "job-eb-001", "succeeded")

	// Wait for onJobDone to process: escalate writes Phase=CONFERENCE to state.yaml.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ls, _ := o.State.ReadState()
		if ls.Phase == PhaseConference {
			return // success: onJobDone was triggered by EventBus event
		}
		time.Sleep(10 * time.Millisecond)
	}
	ls, _ := o.State.ReadState()
	t.Errorf("state.yaml Phase = %q after EventBus event, want %q", ls.Phase, PhaseConference)
}

// TestLoopOrchestrator_EventBus_FallbackPollStillWorks verifies that when
// subscribeToHub returns an error, the poll loop continues functioning.
func TestLoopOrchestrator_EventBus_FallbackPollStillWorks(t *testing.T) {
	o, _ := newTestOrchestratorWithEB(t, 20*time.Millisecond)

	// Make the EB subscriber always fail.
	o.EBSub = &errorEBSubscriber{}

	// Create a hypothesis document so runDebate can find it.
	hypID := mustCreateHyp(t, o.KStore)

	session := &LoopSession{
		HypothesisID: hypID,
		JobID:        "job-fallback-001",
		Status:       "running",
	}
	if err := o.StartLoop(context.Background(), session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Pre-populate hub mock so GetJob returns SUCCEEDED.
	mhc := o.cfg.Hub.(*testMockHubClient)
	mhc.jobs["job-fallback-001"] = &hub.Job{ID: "job-fallback-001", Status: "SUCCEEDED"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer o.Stop(context.Background()) //nolint:errcheck

	// Poll should eventually fire and process the completed job (writes Phase=CONFERENCE).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ls, _ := o.State.ReadState()
		if ls.Phase == PhaseConference {
			return // success: poll fallback worked
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("poll fallback did not process completed job when EventBus subscription failed")
}

// TestLoopOrchestrator_EventBus_NoDuplicateOnJobDone verifies that concurrent
// EventBus events and poll ticks do not cause onJobDone to be called more than once
// for the same job. Runs with -race.
func TestLoopOrchestrator_EventBus_NoDuplicateOnJobDone(t *testing.T) {
	// Use short poll interval so poll races with EB events.
	o, ebSub := newTestOrchestratorWithEB(t, 5*time.Millisecond)

	// Replace the debate caller with one that counts calls atomically.
	var callerInvocations int64
	o.Caller = &countingDebateLLM{
		inner:   o.Caller.(*mockDebateLLM),
		counter: &callerInvocations,
	}

	// Create a hypothesis document so runDebate can find it.
	hypID := mustCreateHyp(t, o.KStore)

	// Pre-populate hub mock so poll() sees the job as completed.
	o.cfg.Hub.(*testMockHubClient).jobs["job-dup-001"] = &hub.Job{ID: "job-dup-001", Status: "SUCCEEDED"}

	session := &LoopSession{
		HypothesisID: hypID,
		JobID:        "job-dup-001",
		Status:       "running",
	}
	if err := o.StartLoop(context.Background(), session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer o.Stop(context.Background()) //nolint:errcheck

	// Flood with duplicate EB events to stress-test dedup while poll also runs.
	for i := 0; i < 10; i++ {
		ebSub.send("hub.job.completed", "job-dup-001", "succeeded")
	}

	// Wait for onJobDone to process (writes Phase=CONFERENCE).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ls, _ := o.State.ReadState()
		if ls.Phase == PhaseConference {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Allow any in-flight goroutines to complete.
	time.Sleep(50 * time.Millisecond)

	// The debate caller (first call in onJobDone) should be invoked exactly once.
	// If dedup fails, it would be called multiple times.
	calls := atomic.LoadInt64(&callerInvocations)
	if calls > 3 {
		// 3 calls = 1 onJobDone (3 LLM calls per debate: Optimizer, Skeptic, Synthesis)
		// More than 3 means onJobDone ran more than once.
		t.Errorf("debate caller invoked %d times; expected <= 3 (1 debate = 3 LLM calls, dedup may have failed)", calls)
	}
}

// errorEBSubscriber always returns an error from Subscribe.
type errorEBSubscriber struct{}

func (e *errorEBSubscriber) Subscribe(_ context.Context, _ string, _ string) (<-chan *ebpb.Event, error) {
	return nil, errTestEBUnavailable
}

var errTestEBUnavailable = fmt.Errorf("test: eventbus unavailable")

// countingDebateLLM wraps mockDebateLLM and counts Call() invocations atomically.
type countingDebateLLM struct {
	inner   *mockDebateLLM
	counter *int64
}

func (c *countingDebateLLM) Call(ctx context.Context, system, user string) (string, error) {
	atomic.AddInt64(c.counter, 1)
	return c.inner.Call(ctx, system, user)
}
