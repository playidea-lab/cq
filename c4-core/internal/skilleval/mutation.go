package skilleval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

const mutationSystemPrompt = `You are a skill prompt optimizer. Analyze the failure pattern and propose exactly ONE targeted change to the skill. Return the full modified skill content. Change only what is necessary to fix the most common failure. Do NOT rewrite the entire skill.`

const mutationUserTmpl = `Skill content:
%s

Failure analysis:
%s

Propose ONE targeted change to fix the most common failure. Return the full modified skill content only — no explanations, no markdown fences.`

// AnalyzeFailures summarizes failure patterns from a slice of EvalResult.
// It returns a human-readable text listing which evals failed and why.
func AnalyzeFailures(results []EvalResult) string {
	if len(results) == 0 {
		return "No eval results to analyze."
	}

	var failed []EvalResult
	for _, r := range results {
		if !r.Passed {
			failed = append(failed, r)
		}
	}

	if len(failed) == 0 {
		return fmt.Sprintf("All %d evals passed. No failures detected.", len(results))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d of %d evals failed:\n", len(failed), len(results))
	for _, r := range failed {
		fmt.Fprintf(&sb, "- [FAIL] %s: %s\n", r.Name, r.Reasoning)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ProposeMutation calls the LLM to propose exactly one targeted change to the
// skill content based on the failure analysis. It returns the full mutated skill
// content and a one-line description of the change.
func ProposeMutation(ctx context.Context, gw *llm.Gateway, skillContent string, failureAnalysis string) (mutated string, description string, err error) {
	ref := gw.Resolve("sonnet", "")

	llmCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	userMsg := fmt.Sprintf(mutationUserTmpl, skillContent, failureAnalysis)

	resp, err := gw.Chat(llmCtx, "sonnet", &llm.ChatRequest{
		Model:       ref.Model,
		System:      mutationSystemPrompt,
		Messages:    []llm.Message{{Role: "user", Content: userMsg}},
		MaxTokens:   2000,
		Temperature: 0.3,
	})
	if err != nil {
		return "", "", fmt.Errorf("mutation LLM: %w", err)
	}

	mutated = strings.TrimSpace(resp.Content)
	if mutated == "" {
		return "", "", errors.New("LLM returned empty mutation")
	}

	// Derive a one-line description by diffing first changed line.
	description = deriveDescription(skillContent, mutated)

	return mutated, description, nil
}

// deriveDescription produces a short one-line summary of the mutation by
// finding the first line that differs between original and mutated content.
func deriveDescription(original, mutated string) string {
	origLines := strings.Split(original, "\n")
	mutLines := strings.Split(mutated, "\n")

	for i := 0; i < len(origLines) && i < len(mutLines); i++ {
		if origLines[i] != mutLines[i] {
			return fmt.Sprintf("changed line %d: %q", i+1, strings.TrimSpace(mutLines[i]))
		}
	}

	if len(mutLines) > len(origLines) {
		return fmt.Sprintf("added %d line(s) to skill", len(mutLines)-len(origLines))
	}
	if len(origLines) > len(mutLines) {
		return fmt.Sprintf("removed %d line(s) from skill", len(origLines)-len(mutLines))
	}

	return "applied targeted mutation to skill"
}
