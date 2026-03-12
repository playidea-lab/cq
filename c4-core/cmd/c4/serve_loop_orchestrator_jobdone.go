//go:build research

package main

import (
	"context"
	"errors"
	"fmt"
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
func (o *LoopOrchestrator) onJobDone(ctx context.Context, session *LoopSession, jobStatus *HubJobStatus) error {
	if o.caller == nil || o.store == nil {
		return errors.New("loop_orchestrator: debate caller/store not wired")
	}

	// Copy-on-write snapshot: all reads and writes use s; the original pointer
	// is never mutated after this point.
	s := *session

	exploreThreshold := o.cfg.ExploreThreshold
	if exploreThreshold <= 0 {
		exploreThreshold = 2
	}

	// Build lineage context for the debate.
	lineageContext := ""
	if o.lineage != nil {
		lc, err := o.lineage.BuildContext(ctx, s.HypothesisID, 5)
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

	// Determine trigger reason from job status (status is normalized to lowercase by poll).
	triggerReason := "dod_success"
	if jobStatus != nil && (jobStatus.Status == "failed" || jobStatus.Status == "cancelled") {
		triggerReason = "dod_null"
	}

	// Run the Optimizer→Skeptic→Synthesis debate.
	result, err := runDebate(ctx, o.caller, o.store, s.HypothesisID, triggerReason, extraContext, lineageContext)
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
	if o.notify != nil {
		jobID := ""
		if jobStatus != nil {
			jobID = jobStatus.JobID
		}
		o.notify.Emit(ctx, EventDebateComplete,
			"Research Loop: Debate Complete",
			fmt.Sprintf("Round %d debate complete (job: %s)", s.Round, jobID))
	}

	// Persist gate_wait state and enter gate.
	// Snapshot gate under mu to avoid data race with Stop().
	o.mu.Lock()
	gate := o.gate
	o.mu.Unlock()
	if o.state != nil && gate != nil {
		gateDur := gate.Duration()
		deadline := time.Now().Add(gateDur)
		jobID := ""
		if jobStatus != nil {
			jobID = jobStatus.JobID
		}
		_ = o.state.WriteState(LoopState{
			State:               "gate_wait",
			LoopCount:           s.Round,
			CurrentHypothesisID: s.HypothesisID,
			LastJobID:           jobID,
			GateDeadline:        &deadline,
		})
		if o.notify != nil {
			o.notify.Emit(ctx, EventGateEntered,
				"Research Loop: Gate Entered",
				fmt.Sprintf("Round %d entering %v gate", s.Round, gateDur))
		}
		// Block until gate elapses or is released.
		<-gate.EnterGate(ctx)
		// Restore running state after gate.
		_ = o.state.WriteState(LoopState{
			State:               "running",
			LoopCount:           s.Round,
			CurrentHypothesisID: s.HypothesisID,
			LastJobID:           jobID,
		})
		if o.notify != nil {
			o.notify.Emit(ctx, EventAutoContinued,
				"Research Loop: Auto-Continued",
				fmt.Sprintf("Round %d gate elapsed, auto-continuing", s.Round))
		}
	}

	// Handle verdict.
	switch verdict {
	case "approved":
		// Extract next hypothesis from draft.
		draft := extractDraft(nextHypDraft)
		if draft == "" {
			// extractDraft failure → treat as null_result.
			// Round is intentionally not advanced; ExploreFlag will force exploration after threshold.
			s.NullResultCount++
			if s.NullResultCount >= exploreThreshold {
				s.ExploreFlag = true
			}
		} else if o.kStore == nil || o.hubCli == nil {
			// kStore/hubCli not wired — degrade gracefully.
			// TODO: wire kStore and hubCli to enable hypothesis creation and job submission.
			s.NullResultCount++
			if s.NullResultCount >= exploreThreshold {
				s.ExploreFlag = true
			}
		} else {
			// Create new TypeHypothesis document.
			newHypID, err := o.kStore.Create(knowledge.TypeHypothesis, map[string]any{
				"title":                draft,
				"status":               "approved",
				"parent_hypothesis_id": s.HypothesisID,
			}, draft)
			if err != nil {
				return fmt.Errorf("create hypothesis: %w", err)
			}

			// Submit new Hub job.
			newJobID, err := o.hubCli.SubmitJob(ctx, loopHubJobRequest{
				HypothesisID: newHypID,
				Command:      "cq research run",
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
			o.sessions.Delete(oldHypID)
		}

	case "null_result":
		s.NullResultCount++
		if s.NullResultCount >= exploreThreshold {
			s.ExploreFlag = true
		}

	case "escalate":
		// Early return: budget gate does not apply to escalated sessions.
		s.Status = "stopped"
		o.sessions.Store(s.HypothesisID, &s)
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
	o.sessions.Store(s.HypothesisID, &s)

	return nil
}

// extractDraft extracts the first line of the next hypothesis draft.
// Returns empty string if draft is empty or whitespace-only.
func extractDraft(draft string) string {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return ""
	}
	if nl := strings.Index(draft, "\n"); nl >= 0 {
		return strings.TrimSpace(draft[:nl])
	}
	return draft
}
