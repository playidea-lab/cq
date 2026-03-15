//go:build research

package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
)

// onJobDone is called when a Hub job completes (status = "completed" or "failed").
// It triggers the Optimizer→Skeptic→Synthesis debate and handles the verdict.
// After debate, it writes gate_wait state, emits notifications, blocks on gate,
// then writes running state and proceeds to the next job.
//
// All mutations are performed on a local copy of the session (copy-on-write) to
// avoid data races with concurrent StopLoop / Steer goroutines.
// ReasoningResult is the structured result from a reasoning Hub job.
type ReasoningResult struct {
	NewHypothesisID string   `json:"new_hypothesis_id"`
	ExperimentSpecs []string `json:"experiment_specs"`
	NextAction      string   `json:"next_action"` // "submit_experiment" | "submit_implement" | "finish"
	FilesChanged    []string `json:"files_changed"`
	Summary         string   `json:"summary"`
}

func (o *LoopOrchestrator) onJobDone(ctx context.Context, session *LoopSession, jobStatus *HubJobStatus) error {
	// Copy-on-write snapshot: all reads and writes use s; the original pointer
	// is never mutated after this point.
	s := *session

	// Capability-based dispatch: reasoning jobs have a different completion path.
	if jobStatus != nil && jobStatus.Job != nil && jobStatus.Job.Capability == "reasoning" {
		return o.handleReasoningResult(ctx, &s, jobStatus)
	}

	if o.Caller == nil || o.Store == nil {
		return errors.New("loop_orchestrator: debate caller/store not wired")
	}

	exploreThreshold := o.cfg.ExploreThreshold
	if exploreThreshold <= 0 {
		exploreThreshold = 2
	}

	// Convergence check: run before debate to short-circuit if converged.
	if o.Convergence != nil && jobStatus != nil && jobStatus.Job != nil {
		metric := jobStatus.Job.BestMetric
		if metric != nil {
			cr := o.Convergence.Check(&s, *metric)
			if cr.Converged {
				s.Status = "completed"
				if o.State != nil {
					_ = o.State.WriteState(LoopState{Phase: PhaseFinish, LoopCount: s.Round, CurrentHypothesisID: s.HypothesisID})
				}
				if o.Notify != nil {
					o.Notify.Emit(ctx, "convergence_reached",
						"Research Loop: Converged",
						fmt.Sprintf("Round %d converged: %s (best=%.4f)", s.Round, cr.Reason, cr.BestMetric))
				}
				o.Sessions.Store(s.HypothesisID, &s)
				return nil
			}
		}
	}

	// Build lineage context for the debate.
	lineageContext := ""
	if o.Lineage != nil {
		lc, err := o.Lineage.BuildContext(ctx, s.HypothesisID, 5)
		if err == nil {
			lineageContext = lc
		}
	}

	// Inject explore hint if flag is set.
	extraContext := s.SteeringGuidance
	if s.ExploreFlag {
		if extraContext != "" {
			extraContext += "\nforce_explore: true"
		} else {
			extraContext = "force_explore: true"
		}
	}
	if lineageContext != "" {
		if extraContext != "" {
			extraContext += "\n" + lineageContext
		} else {
			extraContext = lineageContext
		}
	}

	// Inject experiment metrics from the latest TypeExperiment doc for this HypothesisID.
	if o.KStore != nil {
		if metricBlock := FetchExperimentMetrics(o.KStore, s.HypothesisID); metricBlock != "" {
			if extraContext != "" {
				extraContext += "\n" + metricBlock
			} else {
				extraContext = metricBlock
			}
		}
	}

	// Determine trigger reason from job status (status is normalized to lowercase by poll).
	triggerReason := "dod_success"
	if jobStatus != nil && (jobStatus.Status == "failed" || jobStatus.Status == "cancelled") {
		triggerReason = "dod_null"
	}

	// Run the Optimizer→Skeptic→Synthesis debate.
	result, err := RunDebate(ctx, o.Caller, o.Store, s.HypothesisID, triggerReason, extraContext, lineageContext)
	if err != nil {
		return fmt.Errorf("runDebate: %w", err)
	}
	// Validate result before entering the gate (may block up to 24h).
	if result == nil {
		return fmt.Errorf("runDebate returned nil result")
	}
	m, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected debate result type")
	}
	verdict, _ := m["verdict"].(string)
	nextHypDraft, _ := m["next_hypothesis_draft"].(string)

	// Emit debate_complete after debate finishes.
	if o.Notify != nil {
		jobID := ""
		if jobStatus != nil {
			jobID = jobStatus.JobID
		}
		o.Notify.Emit(ctx, EventDebateComplete,
			"Research Loop: Debate Complete",
			fmt.Sprintf("Round %d debate complete (job: %s)", s.Round, jobID))
	}

	// Persist gate_wait state and enter gate.
	// Snapshot gate under mu to avoid data race with Stop().
	o.mu.Lock()
	gate := o.Gate
	o.mu.Unlock()
	if o.State != nil && gate != nil {
		gateDur := gate.Duration()
		deadline := time.Now().Add(gateDur)
		jobID := ""
		if jobStatus != nil {
			jobID = jobStatus.JobID
		}
		_ = o.State.WriteState(LoopState{
			Phase:               PhaseGateWait,
			LoopCount:           s.Round,
			CurrentHypothesisID: s.HypothesisID,
			LastJobID:           jobID,
			GateDeadline:        &deadline,
		})
		if o.Notify != nil {
			o.Notify.Emit(ctx, EventGateEntered,
				"Research Loop: Gate Entered",
				fmt.Sprintf("Round %d entering %v gate", s.Round, gateDur))
		}
		// Block until gate elapses or is released.
		<-gate.EnterGate(ctx)
		// Restore running state after gate.
		_ = o.State.WriteState(LoopState{
			Phase:               PhaseRun,
			LoopCount:           s.Round,
			CurrentHypothesisID: s.HypothesisID,
			LastJobID:           jobID,
		})
		if o.Notify != nil {
			o.Notify.Emit(ctx, EventAutoContinued,
				"Research Loop: Auto-Continued",
				fmt.Sprintf("Round %d gate elapsed, auto-continuing", s.Round))
		}
	}

	// Handle verdict.
	switch verdict {
	case "approved":
		// Extract next hypothesis from draft.
		draft := ExtractDraft(nextHypDraft)
		if draft == "" {
			// extractDraft failure → treat as null_result.
			// Round is intentionally not advanced; ExploreFlag will force exploration after threshold.
			s.NullResultCount++
			if s.NullResultCount >= exploreThreshold {
				s.ExploreFlag = true
			}
		} else if o.KStore == nil || o.HubCli == nil {
			// kStore/hubCli not wired — degrade gracefully.
			// TODO: wire kStore and hubCli to enable hypothesis creation and job submission.
			s.NullResultCount++
			if s.NullResultCount >= exploreThreshold {
				s.ExploreFlag = true
			}
		} else {
			// Create new TypeHypothesis document.
			newHypID, err := o.KStore.Create(knowledge.TypeHypothesis, map[string]any{
				"title":                draft,
				"status":               "approved",
				"parent_hypothesis_id": s.HypothesisID,
			}, draft)
			if err != nil {
				return fmt.Errorf("create hypothesis: %w", err)
			}

			// Run SpecPipeline: generate and review an ExperimentSpec.
			var specDocID string
			if o.SpecPipeline != nil {
				_, sid, nullResult, specErr := GenerateAndReview(ctx, o.SpecPipeline.Caller, o.SpecPipeline.KStore, draft, s.Round)
				if specErr != nil || nullResult {
					// Spec failed: clean up orphaned hypothesis document.
					if o.KStore != nil {
						if _, delErr := o.KStore.Delete(newHypID); delErr != nil {
							fmt.Fprintf(os.Stderr, "warn: loop spec fail cleanup: %v\n", delErr)
						}
					}
					s.NullResultCount++
					if s.NullResultCount >= exploreThreshold {
						s.ExploreFlag = true
					}
					// Budget gate must also be checked here to prevent sessions from
					// getting permanently stuck in "running" when spec fails at limit.
					if s.MaxIterations > 0 && s.Round >= s.MaxIterations {
						s.Status = "completed"
					}
					o.Sessions.Store(s.HypothesisID, &s)
					return nil
				}
				specDocID = sid
			}

			// Submit new Hub job with the approved ExperimentSpec.
			newJobID, err := o.HubCli.SubmitJob(ctx, LoopHubJobRequest{
				HypothesisID:     newHypID,
				ExperimentSpecID: specDocID,
				Command:          "cq research run",
			})
			if err != nil {
				return fmt.Errorf("submit job: %w", err)
			}

			// Advance session state and re-register under the new HypothesisID.
			// sessions.Delete is safe here: both kStore.Create and hubCli.SubmitJob
			// succeeded above, so the old session can be removed atomically.
			oldHypID := s.HypothesisID
			s.HypothesisID = newHypID
			s.JobID = newJobID
			s.Round++
			s.NullResultCount = 0
			s.ExploreFlag = false
			o.Sessions.Delete(oldHypID)
		}

	case "null_result":
		s.NullResultCount++
		if s.NullResultCount >= exploreThreshold {
			s.ExploreFlag = true
		}

	case "escalate":
		// Submit a reasoning job for human/LLM-level discussion.
		if o.HubCli != nil {
			if err := o.submitReasoningJob(ctx, &s, "conference"); err != nil {
				fmt.Fprintf(os.Stderr, "loop_orchestrator: submitReasoningJob: %v\n", err)
			}
		}
		// Mark as waiting_reasoning so poll() skips this session
		// (Status != "running" guard in poll prevents re-processing).
		s.Status = "waiting_reasoning"
		if o.State != nil {
			_ = o.State.WriteState(LoopState{Phase: PhaseConference, LoopCount: s.Round, CurrentHypothesisID: s.HypothesisID})
		}
		o.Sessions.Store(s.HypothesisID, &s)
		return nil

	default:
		// Unknown verdict → treat as null_result.
		s.NullResultCount++
		if s.NullResultCount >= exploreThreshold {
			s.ExploreFlag = true
		}
	}

	// Budget gate: check iteration limit.
	if s.MaxIterations > 0 && s.Round >= s.MaxIterations {
		s.Status = "completed"
	}

	// Persist the updated session copy. For the approved-advance case,
	// s.HypothesisID was updated to the new key above, so this correctly
	// stores the final state (including any budget-gate completion) under the new key.
	o.Sessions.Store(s.HypothesisID, &s)

	return nil
}

// FetchExperimentMetrics queries the latest TypeExperiment document for the given
// hypothesisID, extracts val_loss and test_metric from the JSON body, and returns
// a formatted extraContext block. Returns "" if no matching doc is found or on error
// (fallback: debate proceeds without metric context).
func FetchExperimentMetrics(kStore *knowledge.Store, hypothesisID string) string {
	docs, err := kStore.List(string(knowledge.TypeExperiment), "experiment", 100)
	if err != nil {
		return ""
	}
	// Find the most-recently-created doc whose body JSON contains hypothesis_id==hypothesisID.
	// List returns docs ORDER BY updated_at DESC, so we take the first match.
	for _, meta := range docs {
		id, _ := meta["id"].(string)
		if id == "" {
			continue
		}
		doc, err := kStore.Get(id)
		if err != nil || doc == nil {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(doc.Body)), &fields); err != nil {
			continue
		}
		if hypID, _ := fields["hypothesis_id"].(string); hypID != hypothesisID {
			continue
		}
		// Found matching doc — build metric block.
		valLossStr := "N/A"
		if v, ok := fields["val_loss"]; ok && v != nil {
			if f, ok := v.(float64); ok && f != 0.0 {
				valLossStr = fmt.Sprintf("%g", f)
			}
		}
		testMetricStr := "N/A"
		if v, ok := fields["test_metric"]; ok && v != nil {
			if f, ok := v.(float64); ok {
				testMetricStr = fmt.Sprintf("%g", f)
			}
		}
		status, _ := fields["status"].(string)
		if status == "" {
			status = "unknown"
		}
		return fmt.Sprintf("experiment_result:\n  val_loss: %s\n  test_metric: %s\n  status: %s",
			valLossStr, testMetricStr, status)
	}
	return ""
}

// ExtractDraft extracts the first line of the next hypothesis draft.
// Returns empty string if draft is empty or whitespace-only.
func ExtractDraft(draft string) string {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return ""
	}
	if nl := strings.Index(draft, "\n"); nl >= 0 {
		return strings.TrimSpace(draft[:nl])
	}
	return draft
}

// RunDebate executes the Optimizer→Skeptic→Synthesis debate flow.
func RunDebate(ctx context.Context, caller DebateCaller, store DebateStore, hypID, triggerReason, extraContext, lineageContext string) (any, error) {
	if hypID == "" {
		return nil, fmt.Errorf("hypothesis_id required")
	}

	hypDoc, err := store.Get(hypID)
	if err != nil || hypDoc == nil {
		return nil, fmt.Errorf("hypothesis not found: %s", hypID)
	}

	userMsg := fmt.Sprintf("Hypothesis: %s\nTrigger: %s\nContext: %s\n", hypID, triggerReason, extraContext)
	if lineageContext != "" {
		userMsg += "\n" + lineageContext + "\n"
	}
	userMsg += "\nHypothesis body:\n" + hypDoc.Body

	optimizerSystem := `You are a research direction optimizer. Analyze the hypothesis and experimental results. Propose the most promising next research direction. Format: DIRECTION: [direction], RATIONALE: [rationale], NEXT_HYPOTHESIS: [draft hypothesis text]`
	skepticSystem := `You are a research hypothesis critic. Challenge the current hypothesis and proposed directions. Identify blind spots, alternative explanations, and exploration directions being ignored. Format: CHALLENGE: [main challenge], ALTERNATIVE: [alternative direction], VERDICT: [approved|null_result|escalate]`

	optimizerOut, err := caller.Call(ctx, optimizerSystem, userMsg)
	if err != nil {
		return nil, fmt.Errorf("optimizer: %w", err)
	}

	skepticOut, err := caller.Call(ctx, skepticSystem, userMsg)
	if err != nil {
		return nil, fmt.Errorf("skeptic: %w", err)
	}

	synthSystem := `You are a research synthesis expert. Given optimizer and skeptic perspectives, determine the final verdict and next hypothesis. Output JSON: {"verdict":"approved|null_result|escalate","next_hypothesis_draft":"...","experiment_spec_draft":"..."}`
	synthUser := fmt.Sprintf("Optimizer:\n%s\n\nSkeptic:\n%s", optimizerOut, skepticOut)
	synthOut, err := caller.Call(ctx, synthSystem, synthUser)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	// Determine verdict: try synth JSON first, fall back to skeptic text.
	verdict := "approved"
	var synthJSON struct {
		Verdict string `json:"verdict"`
	}
	if start := strings.Index(synthOut, "{"); start >= 0 {
		if err := json.Unmarshal([]byte(synthOut[start:]), &synthJSON); err == nil && synthJSON.Verdict != "" {
			verdict = synthJSON.Verdict
		}
	}
	if verdict == "approved" {
		lower := strings.ToLower(skepticOut)
		if strings.Contains(lower, "verdict: null_result") {
			verdict = "null_result"
		} else if strings.Contains(lower, "verdict: escalate") {
			verdict = "escalate"
		}
	}

	// Extract next_hypothesis_draft from optimizer.
	// LLM follows the fixed prompt format so "NEXT_HYPOTHESIS:" is uppercase ASCII.
	nextHypDraft := ""
	if idx := strings.Index(optimizerOut, "NEXT_HYPOTHESIS:"); idx >= 0 {
		nextHypDraft = strings.TrimSpace(optimizerOut[idx+16:])
		if nl := strings.Index(nextHypDraft, "\n"); nl >= 0 {
			nextHypDraft = nextHypDraft[:nl]
		}
	}

	debateBody := fmt.Sprintf("## Debate Record\n\nhypothesis_id: %s\ntrigger_reason: %s\n\n### Optimizer\n%s\n\n### Skeptic\n%s\n\n### Synthesis\n%s",
		hypID, triggerReason, optimizerOut, skepticOut, synthOut)

	debateDocID, err := store.Create(knowledge.TypeDebate, map[string]any{
		"title":          "Debate: " + hypID,
		"hypothesis_id":  hypID,
		"trigger_reason": triggerReason,
		"verdict":        verdict,
	}, debateBody)
	if err != nil {
		return nil, fmt.Errorf("create debate doc: %w", err)
	}

	return map[string]any{
		"debate_doc_id":         debateDocID,
		"verdict":               verdict,
		"next_hypothesis_draft": nextHypDraft,
		"experiment_spec_draft": synthOut,
	}, nil
}

// submitReasoningJob submits a reasoning job to Hub for LLM-level discussion.
func (o *LoopOrchestrator) submitReasoningJob(ctx context.Context, session *LoopSession, task string) error {
	if o.HubCli == nil {
		return errors.New("loop_orchestrator: HubCli not wired")
	}
	_, err := o.HubCli.SubmitJob(ctx, LoopHubJobRequest{
		HypothesisID: session.HypothesisID,
		Command:      "reasoning",
		Capability:   "reasoning",
		Params: map[string]any{
			"task":          task,
			"hypothesis_id": session.HypothesisID,
			"round":         session.Round,
		},
	})
	return err
}

// handleReasoningResult processes the completion of a reasoning Hub job.
// It parses the structured result and decides the next action.
func (o *LoopOrchestrator) handleReasoningResult(ctx context.Context, session *LoopSession, jobStatus *HubJobStatus) error {
	if jobStatus.Status == "failed" || jobStatus.Status == "cancelled" {
		// Reasoning job failed — log and keep session running for retry.
		fmt.Fprintf(os.Stderr, "loop_orchestrator: reasoning job %s %s\n", jobStatus.JobID, jobStatus.Status)
		o.Sessions.Store(session.HypothesisID, session)
		return nil
	}

	// Parse structured result from the job.
	var result ReasoningResult
	if jobStatus.Job != nil && jobStatus.Job.Result != nil {
		raw, err := json.Marshal(jobStatus.Job.Result)
		if err == nil {
			_ = json.Unmarshal(raw, &result)
		}
	}

	switch result.NextAction {
	case "submit_experiment":
		// Submit experiment job with the new hypothesis.
		if o.HubCli != nil && result.NewHypothesisID != "" {
			newJobID, err := o.HubCli.SubmitJob(ctx, LoopHubJobRequest{
				HypothesisID: result.NewHypothesisID,
				Command:      "cq research run",
			})
			if err != nil {
				return fmt.Errorf("submit experiment after reasoning: %w", err)
			}
			session.HypothesisID = result.NewHypothesisID
			session.JobID = newJobID
			session.Round++
		}
		if o.State != nil {
			_ = o.State.WriteState(LoopState{Phase: PhaseRun, LoopCount: session.Round, CurrentHypothesisID: session.HypothesisID})
		}

	case "submit_implement":
		// Need implementation before experiment — submit another reasoning job.
		if o.HubCli != nil {
			_ = o.submitReasoningJob(ctx, session, "implement")
		}
		if o.State != nil {
			_ = o.State.WriteState(LoopState{Phase: PhaseImplement, LoopCount: session.Round, CurrentHypothesisID: session.HypothesisID})
		}

	case "finish":
		session.Status = "completed"
		if o.State != nil {
			_ = o.State.WriteState(LoopState{Phase: PhaseFinish, LoopCount: session.Round, CurrentHypothesisID: session.HypothesisID})
		}

	default:
		// Unknown action — keep session running.
		fmt.Fprintf(os.Stderr, "loop_orchestrator: unknown reasoning next_action: %q\n", result.NextAction)
	}

	o.Sessions.Store(session.HypothesisID, session)
	return nil
}
