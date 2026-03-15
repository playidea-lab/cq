//go:build research

package orchestrator

import (
	"context"
	"testing"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
)

// mustNewKnowledgeStore creates a temp knowledge store for tests.
func mustNewKnowledgeStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mustNewHypothesisStore is an alias for mustNewKnowledgeStore for readability.
func mustNewHypothesisStore(t *testing.T) *knowledge.Store {
	t.Helper()
	return mustNewKnowledgeStore(t)
}

// mustCreateHyp creates a TypeHypothesis doc and returns its ID.
func mustCreateHyp(t *testing.T, kStore *knowledge.Store) string {
	t.Helper()
	id, err := kStore.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "test hypothesis",
		"status": "approved",
	}, "test body")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}
	return id
}

// mockDebateLLM is a test double for DebateCaller.
type mockDebateLLM struct {
	responses []string
	idx       int
}

func (m *mockDebateLLM) Call(_ context.Context, _, _ string) (string, error) {
	r := m.responses[m.idx%len(m.responses)]
	m.idx++
	return r, nil
}

// testDebateStore wraps *knowledge.Store to implement DebateStore.
type testDebateStore struct{ s *knowledge.Store }

func (ts *testDebateStore) Get(id string) (*knowledge.Document, error) { return ts.s.Get(id) }
func (ts *testDebateStore) Create(dt knowledge.DocumentType, meta map[string]any, body string) (string, error) {
	return ts.s.Create(dt, meta, body)
}

// mockLoopHubClient implements LoopHubClient for tests.
type mockLoopHubClient struct {
	submitJobFunc func(ctx context.Context, req LoopHubJobRequest) (string, error)
}

func (m *mockLoopHubClient) SubmitJob(ctx context.Context, req LoopHubJobRequest) (string, error) {
	return m.submitJobFunc(ctx, req)
}

// mockLoopLineageBuilder implements LoopLineageBuilder for tests.
type mockLoopLineageBuilder struct {
	buildContextFunc func(ctx context.Context, hypothesisID string, limit int) (string, error)
}

func (m *mockLoopLineageBuilder) BuildContext(ctx context.Context, hypothesisID string, limit int) (string, error) {
	return m.buildContextFunc(ctx, hypothesisID, limit)
}

// testMockHubClient implements HubClient for tests.
type testMockHubClient struct {
	jobs map[string]*hub.Job
}

func newMockHubClient() *testMockHubClient {
	return &testMockHubClient{jobs: make(map[string]*hub.Job)}
}

func (m *testMockHubClient) GetJob(jobID string) (*hub.Job, error) {
	if j, ok := m.jobs[jobID]; ok {
		return j, nil
	}
	return &hub.Job{ID: jobID, Status: "RUNNING"}, nil
}

func (m *testMockHubClient) SubmitJob(req *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error) {
	id := "job-new-001"
	m.jobs[id] = &hub.Job{ID: id, Status: "QUEUED"}
	return &hub.JobSubmitResponse{JobID: id, Status: "QUEUED"}, nil
}

// newTestOrchestrator creates a LoopOrchestrator with mock internals.
func newTestOrchestrator(t *testing.T, llmResponses []string) (*LoopOrchestrator, *knowledge.Store) {
	t.Helper()
	kStore := mustNewHypothesisStore(t)
	mock := &mockDebateLLM{responses: llmResponses}
	store := &testDebateStore{s: kStore}
	hubCli := &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, _ LoopHubJobRequest) (string, error) {
			return "job-new-001", nil
		},
	}
	lineage := &mockLoopLineageBuilder{
		buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) {
			return "", nil
		},
	}
	o := &LoopOrchestrator{
		cfg:     Config{ExploreThreshold: 2},
		Caller:  mock,
		Store:   store,
		HubCli:  hubCli,
		Lineage: lineage,
		KStore:  kStore,
	}
	return o, kStore
}
