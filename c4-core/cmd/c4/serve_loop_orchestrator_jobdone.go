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
func (o *LoopOrchestrator) onJobDone(ctx context.Context, session *LoopSession, jobStatus *HubJobStatus) error {
	if o.caller == nil || o.store == nil {
		return errors.New("loop_orchestrator: debate caller/store not wired")
	}

	exploreThreshold := o.cfg.ExploreThreshold
	if exploreThreshold <= 0 {
		exploreThreshold = 2
	}

	// Build lineage context for the debate.
	lineageContext := ""
	if o.lineage != nil {
		lc, err := o.lineage.BuildContext(ctx, session.HypothesisID, 5)
		if err == nil {
			lineageContext = lc
		}
	}

	// Inject explore hint if flag is set.
	extraContext := session.SteeringGuidance
	if session.ExploreFlag {
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

	// Determine trigger reason from job status.
	triggerReason := "dod_success"
	if jobStatus != nil && jobStatus.Status == "failed" {
		triggerReason = "dod_null"
	}

	// Run the Optimizer→Skeptic→Synthesis debate.
	result, err := runDebate(ctx, o.caller, o.store, session.HypothesisID, triggerReason, extraContext, lineageContext)
	if err != nil {
		return fmt.Errorf("runDebate: %w", err)
	}

	// Emit debate_complete after debate finishes.
	if o.notify != nil {
		jobID := ""
		if jobStatus != nil {
			jobID = jobStatus.JobID
		}
		o.notify.Emit(ctx, EventDebateComplete,
			"Research Loop: Debate Complete",
			fmt.Sprintf("Round %d debate complete (job: %s)", session.Round, jobID))
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
			LoopCount:           session.Round,
			CurrentHypothesisID: session.HypothesisID,
			LastJobID:           jobID,
			GateDeadline:        &deadline,
		})
		if o.notify != nil {
			o.notify.Emit(ctx, EventGateEntered,
				"Research Loop: Gate Entered",
				fmt.Sprintf("Round %d entering %v gate", session.Round, gateDur))
		}
		// Block until gate elapses or is released.
		<-gate.EnterGate(ctx)
		// Restore running state after gate.
		_ = o.state.WriteState(LoopState{
			State:               "running",
			LoopCount:           session.Round,
			CurrentHypothesisID: session.HypothesisID,
			LastJobID:           jobID,
		})
		if o.notify != nil {
			o.notify.Emit(ctx, EventAutoContinued,
				"Research Loop: Auto-Continued",
				fmt.Sprintf("Round %d gate elapsed, auto-continuing", session.Round))
		}
	}


	m, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected debate result type")
	}

	verdict, _ := m["verdict"].(string)
	nextHypDraft, _ := m["next_hypothesis_draft"].(string)

	// Handle verdict.
	switch verdict {
	case "approved":
		// Extract next hypothesis from draft.
		draft := extractDraft(nextHypDraft)
		if draft == "" {
			// extractDraft failure → treat as null_result.
			session.NullResultCount++
			if session.NullResultCount >= exploreThreshold {
				session.ExploreFlag = true
			}
		} else if o.kStore == nil || o.hubCli == nil {
			// kStore/hubCli not wired — degrade gracefully.
			session.NullResultCount++
			if session.NullResultCount >= exploreThreshold {
				session.ExploreFlag = true
			}
		} else {
			// Create new TypeHypothesis document.
			newHypID, err := o.kStore.Create(knowledge.TypeHypothesis, map[string]any{
				"title":               draft,
				"status":              "approved",
				"parent_hypothesis_id": session.HypothesisID,
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

			// Advance session state.
			session.HypothesisID = newHypID
			session.JobID = newJobID
			session.Round++
			session.NullResultCount = 0
			session.ExploreFlag = false
		}

	case "null_result":
		session.NullResultCount++
		if session.NullResultCount >= exploreThreshold {
			session.ExploreFlag = true
		}

	case "escalate":
		session.Status = "stopped"
		return nil

	default:
		// Unknown verdict → treat as null_result.
		session.NullResultCount++
		if session.NullResultCount >= exploreThreshold {
			session.ExploreFlag = true
		}
	}

	// Budget gate: check iteration limit.
	if session.MaxIterations > 0 && session.Round >= session.MaxIterations {
		session.Status = "completed"
	}

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
