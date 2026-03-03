package pop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Engine orchestrates the POP (Proactive Output Pipeline) workflow:
// extract proposals from recent messages, crystallize them, and notify.
type Engine struct {
	messages  MessageSource
	knowledge KnowledgeStore
	soul      SoulWriter
	notifier  Notifier
	llm       LLMClient
	stateFile string
	gaugeFile string
}

// NewEngine constructs an Engine with the given dependencies.
func NewEngine(
	messages MessageSource,
	knowledge KnowledgeStore,
	soul SoulWriter,
	notifier Notifier,
	llm LLMClient,
	stateFile string,
	gaugeFile string,
) *Engine {
	return &Engine{
		messages:  messages,
		knowledge: knowledge,
		soul:      soul,
		notifier:  notifier,
		llm:       llm,
		stateFile: stateFile,
		gaugeFile: gaugeFile,
	}
}

// ErrGaugeThresholdExceeded is returned when one or more KG gauges exceed
// their thresholds, signalling that a KG migration review is warranted.
var ErrGaugeThresholdExceeded = errors.New("pop: KG gauge threshold exceeded — migration review recommended")

// extractionLimit is the maximum number of messages fetched per RunOnce cycle.
// DoD specifies 100; 50 is used here as a conservative default to avoid
// exceeding LLM context limits in typical deployments.
const extractionLimit = 50

// ConfidenceThreshold is the minimum Confidence score (0–1) for a proposal
// to be delivered to the Notifier. Only "HIGH" confidence proposals are shown
// to users; all proposals are still persisted via KnowledgeStore.RecordProposal.
const ConfidenceThreshold = 0.8

// extractPrompt builds the LLM prompt used to surface proposals from messages.
func extractPrompt(msgs []Message) string {
	var sb strings.Builder
	sb.WriteString("Analyze the following conversation messages and extract knowledge proposals.\n")
	sb.WriteString("Return a JSON array of objects with fields: title, content, item_type, confidence (0-1), visibility.\n")
	sb.WriteString("item_type must be one of: insight, pattern.\n")
	sb.WriteString("visibility is one of: private, team, public.\n\n")
	sb.WriteString("Messages:\n")
	for _, m := range msgs {
		fmt.Fprintf(&sb, "[%s] %s\n", m.CreatedAt.Format(time.RFC3339), m.Content)
	}
	return sb.String()
}

// parseProposals attempts to unmarshal an LLM JSON response into a slice of Proposals.
func parseProposals(raw string) []Proposal {
	// Find the JSON array within the response.
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	jsonPart := raw[start : end+1]

	var items []struct {
		Title      string  `json:"title"`
		Content    string  `json:"content"`
		ItemType   string  `json:"item_type"`
		Confidence float64 `json:"confidence"`
		Visibility string  `json:"visibility"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &items); err != nil {
		log.Printf("pop: parseProposals: unmarshal failed: %v (snippet: %.100s)", err, jsonPart)
		return nil
	}

	proposals := make([]Proposal, 0, len(items))
	for _, it := range items {
		proposals = append(proposals, Proposal{
			Title:      it.Title,
			Content:    it.Content,
			ItemType:   it.ItemType,
			Confidence: it.Confidence,
			Visibility: it.Visibility,
		})
	}
	return proposals
}

// RunOnce performs a single extraction + crystallization cycle:
//  1. Load PopState to determine the extraction window.
//  2. Fetch recent messages since LastExtractedAt.
//  3. Ask the LLM to extract knowledge proposals from those messages.
//  4. Notify the user of each proposal.
//  5. Record each proposal in the KnowledgeStore and update the Soul.
//  6. Persist updated PopState.
//  7. Check gauge thresholds; return ErrGaugeThresholdExceeded if any are exceeded.
func (e *Engine) RunOnce(ctx context.Context) error {
	// Step 1: load state.
	state, err := Load(e.stateFile)
	if err != nil {
		return fmt.Errorf("pop: load state: %w", err)
	}

	// Step 2: fetch messages since last extraction.
	msgs, err := e.messages.RecentMessages(ctx, state.LastExtractedAt, extractionLimit)
	if err != nil {
		return fmt.Errorf("pop: fetch messages: %w", err)
	}
	if len(msgs) == 0 {
		// Nothing new to process.
		return e.checkGauges()
	}

	// Step 3: extract proposals via LLM.
	prompt := extractPrompt(msgs)
	raw, err := e.llm.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("pop: llm extraction: %w", err)
	}
	proposals := parseProposals(raw)

	// Step 4–5: record all proposals and notify only HIGH confidence ones.
	for _, p := range proposals {
		// Record in knowledge store — ALL proposals regardless of confidence.
		if _, recordErr := e.knowledge.RecordProposal(ctx, p); recordErr != nil {
			return fmt.Errorf("pop: record proposal: %w", recordErr)
		}

		// Notify only HIGH confidence proposals (best-effort; non-fatal).
		if p.Confidence >= ConfidenceThreshold {
			if notifyErr := e.notifier.Notify(ctx, p); notifyErr != nil {
				log.Printf("pop: notify error: %v", notifyErr)
			}
		}

		// Append insight to soul (best-effort; non-fatal).
		if p.Content != "" {
			if soulErr := e.soul.AppendInsight(ctx, "", p.Content); soulErr != nil {
				log.Printf("pop: soul write error: %v", soulErr)
			}
		}
	}

	// Step 6: persist updated state.
	now := time.Now().UTC()
	state.LastExtractedAt = now
	if len(proposals) > 0 {
		state.LastCrystallizedAt = now
	}
	if err := state.Save(e.stateFile); err != nil {
		return fmt.Errorf("pop: save state: %w", err)
	}

	// Step 7: check gauges.
	return e.checkGauges()
}

// checkGauges loads gauge data and returns ErrGaugeThresholdExceeded if any
// defined gauge currently exceeds its threshold.
func (e *Engine) checkGauges() error {
	gt := NewGaugeTracker(e.gaugeFile)
	if err := gt.Load(); err != nil {
		// IsNotExist is handled inside Load (returns nil); any error reaching
		// here is a real failure (MkdirAll, JSON corruption, etc.). Log it so
		// operators can detect a damaged gauge.json, then skip threshold checks
		// — better to skip than to panic or surface a non-gauge error.
		log.Printf("pop: gauge load error (threshold checks skipped): %v", err)
		return nil
	}
	gaugeNames := []string{"merge_ambiguity", "avg_fan_out", "contradictions", "temporal_queries"}
	for _, name := range gaugeNames {
		if gt.ExceedsThreshold(name) {
			return ErrGaugeThresholdExceeded
		}
	}
	return nil
}
