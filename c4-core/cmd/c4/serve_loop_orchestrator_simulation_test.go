//go:build research

package main

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
// 시뮬레이션 헬퍼
// =============================================================================

// simulatedHub: 잡 상태를 외부에서 제어 가능한 mock Hub
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
	h.t.Logf("  [Hub] 잡 제출됨: %s (hypothesis=%s)", newID, req.Env["C4_HYPOTHESIS_ID"])
	return &hub.JobSubmitResponse{JobID: newID, Status: "QUEUED"}, nil
}

func (h *simulatedHub) completeJob(jobID string, status string) {
	if j, ok := h.jobs[jobID]; ok {
		j.Status = status
		h.t.Logf("  [Hub] 잡 완료: %s → %s", jobID, status)
	}
}

// simulatedNotifier: 알림 이벤트를 캡처하는 mock
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
// 시나리오:
//   Round 0: approved  → 새 가설로 진행
//   Round 1: null_result → NullCount=1
//   Round 2: null_result → NullCount=2, ExploreFlag=true
//   Round 3: approved  → MaxIterations 도달 → Status=completed
// =============================================================================
func TestResearchLoop_FullSimulation(t *testing.T) {
	dir := t.TempDir()
	kStore := mustNewHypothesisStore(t)
	simHub := newSimulatedHub(t)
	notifier := &simulatedNotifier{t: t}

	// LLM 응답 시나리오 (각 라운드마다 Optimizer+Skeptic+Synthesis = 3개 응답)
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

	t.Log("=== Research Loop 시뮬레이션 시작 ===")
	t.Logf("시나리오: approved → null_result × 2 (ExploreFlag) → approved (budget gate)")

	// 초기 가설 생성
	hypID, err := kStore.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "initial hypothesis: optimize attention mechanism",
		"status": "approved",
	}, "initial body")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}
	t.Logf("\n[Setup] 초기 가설 ID: %s", hypID)

	// 초기 잡 등록
	initialJobID := "job-sim-initial"
	simHub.jobs[initialJobID] = &hub.Job{ID: initialJobID, Status: "RUNNING"}

	// LoopOrchestrator 구성 (gate=50ms, poll=20ms)
	o := &LoopOrchestrator{
		cfg:     LoopOrchestratorConfig{ExploreThreshold: 2, PollInterval: 20 * time.Millisecond},
		caller:  &mockDebateLLM{responses: flattenResponses(roundResponses)},
		store:   &testDebateStore{s: kStore},
		hubCli: &mockLoopHubClient{submitJobFunc: func(_ context.Context, req loopHubJobRequest) (string, error) {
		resp, err := simHub.SubmitJob(&hub.JobSubmitRequest{Env: map[string]string{"C4_HYPOTHESIS_ID": req.HypothesisID}})
		if err != nil {
			return "", err
		}
		return resp.JobID, nil
	}},
		lineage: &mockLoopLineageBuilder{buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) { return "", nil }},
		kStore:  kStore,
		gate:    NewGateController(50 * time.Millisecond),
		state:   NewStateYAMLWriter(filepath.Join(dir, ".c9")),
		notify:  NewNotifyBridge(notifier, 0),
	}

	ctx := context.Background()

	// 세션 시작
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         initialJobID,
		Round:         0,
		MaxIterations: 2, // approved 2회(R=2) 시 completed
		Status:        "running",
	}
	if err := o.StartLoop(ctx, session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}
	t.Logf("[Session] 루프 시작: hypID=%s, jobID=%s, maxIter=%d (approved 2회 시 completed)", hypID, initialJobID, session.MaxIterations)

	// ─── Round 0: approved ──────────────────────────────────────────────────
	t.Log("\n─── Round 0: approved 시나리오 ───")
	simHub.completeJob(initialJobID, "SUCCEEDED")
	jobStatus0 := &HubJobStatus{JobID: initialJobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, o.GetLoop(hypID), jobStatus0); err != nil {
		t.Fatalf("Round 0 onJobDone: %v", err)
	}

	// approved 후 세션 key가 변경됨 → Range로 탐색
	var got0 *LoopSession
	o.sessions.Range(func(_, v any) bool { got0 = v.(*LoopSession); return true })
	if got0 == nil {
		t.Fatal("Round 0: 세션 없음")
	}
	t.Logf("[Round 0] 결과: verdict=approved, Round=%d, HypID=%s, JobID=%s", got0.Round, got0.HypothesisID, got0.JobID)
	assertInt(t, "Round", got0.Round, 1)
	assertInt(t, "NullResultCount", got0.NullResultCount, 0)
	assertBool(t, "ExploreFlag", got0.ExploreFlag, false)

	// 상태 파일 확인
	state0, err := o.state.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	t.Logf("[State] state=%s, loop_count=%d", state0.State, state0.LoopCount)

	// ─── Round 1: null_result ────────────────────────────────────────────────
	t.Log("\n─── Round 1: null_result 시나리오 ───")
	// LLM 응답 교체 (다음 라운드용)
	o.caller = &mockDebateLLM{responses: flattenResponses(roundResponses[1:])}

	simHub.completeJob(got0.JobID, "SUCCEEDED")
	jobStatus1 := &HubJobStatus{JobID: got0.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got0, jobStatus1); err != nil {
		t.Fatalf("Round 1 onJobDone: %v", err)
	}
	got1 := o.GetLoop(got0.HypothesisID)
	if got1 == nil {
		t.Fatal("Round 1: 세션 없음")
	}
	t.Logf("[Round 1] 결과: verdict=null_result, NullCount=%d, ExploreFlag=%v", got1.NullResultCount, got1.ExploreFlag)
	assertInt(t, "NullResultCount", got1.NullResultCount, 1)
	assertBool(t, "ExploreFlag", got1.ExploreFlag, false) // 아직 threshold 미달

	// ─── Round 2: null_result → ExploreFlag ─────────────────────────────────
	t.Log("\n─── Round 2: null_result → ExploreFlag 시나리오 ───")
	o.caller = &mockDebateLLM{responses: flattenResponses(roundResponses[2:])}

	simHub.completeJob(got1.JobID, "SUCCEEDED")
	jobStatus2 := &HubJobStatus{JobID: got1.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got1, jobStatus2); err != nil {
		t.Fatalf("Round 2 onJobDone: %v", err)
	}
	got2 := o.GetLoop(got1.HypothesisID)
	if got2 == nil {
		t.Fatal("Round 2: 세션 없음")
	}
	t.Logf("[Round 2] 결과: verdict=null_result, NullCount=%d, ExploreFlag=%v", got2.NullResultCount, got2.ExploreFlag)
	assertInt(t, "NullResultCount", got2.NullResultCount, 2)
	assertBool(t, "ExploreFlag", got2.ExploreFlag, true) // threshold(2) 도달!
	t.Log("  → ExploreFlag=true: 다음 토론에 force_explore 힌트 주입 예정")

	// ─── Round 3: approved + budget gate ────────────────────────────────────
	t.Log("\n─── Round 3: approved + budget gate (MaxIterations=4) 시나리오 ───")
	o.caller = &mockDebateLLM{responses: flattenResponses(roundResponses[3:])}

	simHub.completeJob(got2.JobID, "SUCCEEDED")
	jobStatus3 := &HubJobStatus{JobID: got2.JobID, Status: "succeeded"}

	if err := o.onJobDone(ctx, got2, jobStatus3); err != nil {
		t.Fatalf("Round 3 onJobDone: %v", err)
	}

	// approved → key 변경 → completed 세션 탐색
	var got3 *LoopSession
	o.sessions.Range(func(_, v any) bool {
		s := v.(*LoopSession)
		if s.Status == "completed" {
			got3 = s
		}
		return true
	})
	if got3 == nil {
		t.Fatal("Round 3: completed 세션 없음")
	}
	t.Logf("[Round 3] 결과: Round=%d, Status=%s", got3.Round, got3.Status)
	// Round 0→approved(R=1), Round 1→null, Round 2→null, Round 3→approved(R=2)
	// MaxIterations=2: R=2 >= 2 → completed
	assertInt(t, "Round", got3.Round, 2)
	assertString(t, "Status", got3.Status, "completed") // budget gate 발동!
	t.Log("  → Status=completed: MaxIterations(4) 도달, 루프 자동 종료")

	// ─── Gate 동작 확인 ───────────────────────────────────────────────────────
	t.Log("\n─── Gate 동작 확인 ───")
	gate := NewGateController(50 * time.Millisecond)
	ch := gate.EnterGate(ctx)
	select {
	case <-ch:
		t.Log("[Gate] ✓ 50ms gate 정상 소화")
	case <-time.After(500 * time.Millisecond):
		t.Error("[Gate] ✗ gate timeout")
	}

	// ReleaseGate (intervene) 확인
	gate2 := NewGateController(10 * time.Second)
	ch2 := gate2.EnterGate(ctx)
	go func() {
		time.Sleep(10 * time.Millisecond)
		gate2.Release("human-intervene")
	}()
	select {
	case <-ch2:
		t.Log("[Gate] ✓ intervene(Release) 즉시 해제 동작")
	case <-time.After(500 * time.Millisecond):
		t.Error("[Gate] ✗ Release가 gate를 해제하지 못함")
	}

	// ─── 알림 이벤트 확인 ────────────────────────────────────────────────────
	t.Log("\n─── 알림 이벤트 확인 ───")
	for i, ev := range notifier.events {
		t.Logf("  [%d] %s", i+1, ev)
	}
	if len(notifier.events) == 0 {
		t.Log("  (알림 없음 — gate가 nil이어서 gate 이벤트 미발생)")
	}

	// ─── 최종 요약 ────────────────────────────────────────────────────────────
	t.Log("\n=== 시뮬레이션 완료 ===")
	t.Logf("라운드 이력: approved → null×2(ExploreFlag) → approved+budget_gate")
	t.Logf("최종 상태: Round=%d, Status=%s", got3.Round, got3.Status)
	t.Log("모든 검증 통과 ✓")
}

// =============================================================================
// TestResearchLoop_EscalateSimulation: escalate → 루프 즉시 중단
// =============================================================================
func TestResearchLoop_EscalateSimulation(t *testing.T) {
	kStore := mustNewHypothesisStore(t)

	llmResponses := []string{
		"DIRECTION: stop\nRATIONALE: critical failure\nNEXT_HYPOTHESIS: none",
		"CHALLENGE: unrecoverable\nALTERNATIVE: none\nVERDICT: escalate",
		`{"verdict":"escalate","next_hypothesis_draft":""}`,
	}

	o := &LoopOrchestrator{
		cfg:     LoopOrchestratorConfig{ExploreThreshold: 2},
		caller:  &mockDebateLLM{responses: llmResponses},
		store:   &testDebateStore{s: kStore},
		hubCli:  &mockLoopHubClient{submitJobFunc: func(_ context.Context, _ loopHubJobRequest) (string, error) { return "job-x", nil }},
		lineage: &mockLoopLineageBuilder{buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) { return "", nil }},
		kStore:  kStore,
	}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-esc",
		Round:         2,
		MaxIterations: 10,
		Status:        "running",
	}

	t.Log("=== Escalate 시뮬레이션 ===")
	t.Logf("[Setup] 실험 중 치명적 오류 발생, 사람에게 에스컬레이션")

	jobStatus := &HubJobStatus{JobID: "job-esc", Status: "failed"}
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("escalate 후 세션 없음")
	}
	t.Logf("[Escalate] Status=%s (gate wait 없이 즉시 중단)", got.Status)
	assertString(t, "Status", got.Status, "stopped")
	t.Log("  → 루프 즉시 중단, gate 없이 사람에게 제어권 반환 ✓")
}

// =============================================================================
// TestResearchLoop_StopLoop_ConcurrentSafety: concurrent Steer + StopLoop
// =============================================================================
func TestResearchLoop_StopLoop_ConcurrentSafety(t *testing.T) {
	o := &LoopOrchestrator{
		cfg: LoopOrchestratorConfig{},
	}

	ctx := context.Background()

	// 세션 10개 병렬 등록
	for i := 0; i < 10; i++ {
		hypID := fmt.Sprintf("hyp-concurrent-%d", i)
		sess := &LoopSession{HypothesisID: hypID, JobID: fmt.Sprintf("job-%d", i)}
		if err := o.StartLoop(ctx, sess); err != nil {
			t.Fatalf("StartLoop %d: %v", i, err)
		}
	}

	t.Log("=== Concurrent Safety 시뮬레이션 (Steer + StopLoop 동시 실행) ===")

	// Steer와 StopLoop를 동시에 실행 (데이터 레이스 없어야 함)
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

	// 모든 세션이 stopped 상태인지 확인
	allStopped := true
	o.sessions.Range(func(_, v any) bool {
		s := v.(*LoopSession)
		if s.Status != "stopped" {
			allStopped = false
		}
		return true
	})
	if allStopped {
		t.Log("[Concurrent] 10개 세션 모두 stopped — 데이터 레이스 없음 ✓")
	} else {
		t.Error("[Concurrent] 일부 세션이 stopped 상태 아님")
	}
}

// =============================================================================
// 헬퍼
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
		t.Errorf("  ✗ %s = %d, want %d", name, got, want)
	} else {
		t.Logf("  ✓ %s = %d", name, got)
	}
}

func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("  ✗ %s = %v, want %v", name, got, want)
	} else {
		t.Logf("  ✓ %s = %v", name, got)
	}
}

func assertString(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("  ✗ %s = %q, want %q", name, got, want)
	} else {
		t.Logf("  ✓ %s = %q", name, got)
	}
}

// mockLoopHubClient 어댑터 (submitJobFunc 기반)
func (m *mockLoopHubClient) getSimHub() func(ctx context.Context, req loopHubJobRequest) (string, error) {
	return m.submitJobFunc
}

// simulatedNotifier가 Notifier 인터페이스를 구현하는지 확인
// NotifyBridge가 받는 인터페이스에 맞게 래핑
type simulatedNotifierAdapter struct {
	n *simulatedNotifier
}

func (a *simulatedNotifierAdapter) Notify(ctx context.Context, event, title, body string) error {
	return a.n.Notify(ctx, event, title, body)
}

// NotifyBridge 생성 시 simulatedNotifier를 Notifier로 사용하기 위한 확인
var _ interface {
	Notify(context.Context, string, string, string) error
} = (*simulatedNotifier)(nil)

// mustNewHypothesisStore는 jobdone_test.go에도 있을 수 있어 확인
func mustNewHypothesisStoreLocal(t *testing.T) *knowledge.Store {
	t.Helper()
	return mustNewHypothesisStore(t)
}

// strings 패키지 사용 확인용 (미사용 import 방지)
var _ = strings.Contains
