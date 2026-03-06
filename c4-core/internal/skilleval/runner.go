package skilleval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

const judgeSystemPrompt = `You are a skill trigger classifier. Given a skill description and a user prompt, determine if the skill should be triggered. Respond with JSON only: {"should_trigger": true/false, "confidence": 0.0-1.0}`

const judgeUserTmpl = `Skill description: %s
User prompt: %s
Should this skill trigger? Respond with JSON: {"should_trigger": true/false, "confidence": 0.0-1.0}`

// judgeResponse is the expected JSON from the judge LLM.
type judgeResponse struct {
	ShouldTrigger bool    `json:"should_trigger"`
	Confidence    float64 `json:"confidence"`
}

// CaseResult holds per-test-case evaluation results.
type CaseResult struct {
	Prompt        string  `json:"prompt"`
	Expected      bool    `json:"expected"`
	Trials        []bool  `json:"trials"`      // raw per-trial outcomes
	PassAtK       bool    `json:"pass_at_k"`   // at least 1 correct in k trials
	PassK         bool    `json:"pass_k"`      // all k correct
	AvgConfidence float64 `json:"avg_confidence"`
	Correct       bool    `json:"correct"` // majority vote
}

// RunResult holds the aggregated evaluation result for a skill.
type RunResult struct {
	Skill           string       `json:"skill"`
	TriggerAccuracy float64      `json:"trigger_accuracy"` // majority-vote accuracy across all cases
	PassAtK         float64      `json:"pass_at_k"`        // fraction of cases with pass@k
	PassK           float64      `json:"pass_k"`           // fraction of cases with pass^k
	K               int          `json:"k"`
	TestCount       int          `json:"test_count"`
	ExpID           string       `json:"exp_id"`
	Cases           []CaseResult `json:"cases"`
}

// callJudge calls the judge LLM once for a single (description, prompt) pair.
// Uses the scout task type (maps to a fast/cheap model like haiku).
func callJudge(ctx context.Context, gateway *llm.Gateway, skillDesc, testPrompt string) (judgeResponse, error) {
	ref := gateway.Resolve("scout", "")
	llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	userMsg := fmt.Sprintf(judgeUserTmpl, skillDesc, testPrompt)

	resp, err := gateway.Chat(llmCtx, "scout", &llm.ChatRequest{
		Model:       ref.Model,
		System:      judgeSystemPrompt,
		Messages:    []llm.Message{{Role: "user", Content: userMsg}},
		MaxTokens:   100,
		Temperature: 0.0,
	})
	if err != nil {
		return judgeResponse{}, fmt.Errorf("judge LLM: %w", err)
	}

	// Extract JSON from response (may have surrounding text)
	raw := strings.TrimSpace(resp.Content)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var jr judgeResponse
	if err := json.Unmarshal([]byte(raw), &jr); err != nil {
		// Fallback: infer from text
		lower := strings.ToLower(resp.Content)
		if strings.Contains(lower, `"should_trigger": true`) || strings.Contains(lower, `"should_trigger":true`) {
			jr.ShouldTrigger = true
		}
		jr.Confidence = 0.5
	}

	return jr, nil
}

// RunEval evaluates the skill trigger accuracy using k LLM judge calls per test case.
// If EVAL.md does not exist and a gateway is provided, it auto-generates one first.
func RunEval(ctx context.Context, gateway *llm.Gateway, projectRoot, skillName string, k int) (*RunResult, error) {
	if k <= 0 {
		k = 5
	}

	evalPath := EvalMDPath(projectRoot, skillName)

	// Auto-generate EVAL.md if missing
	if _, statErr := os.Stat(evalPath); errors.Is(statErr, os.ErrNotExist) {
		if gateway == nil {
			return nil, fmt.Errorf("EVAL.md missing for skill %q and no LLM gateway available for auto-generation", skillName)
		}
		if _, genErr := GenerateEvalMD(ctx, gateway, projectRoot, skillName); genErr != nil {
			return nil, fmt.Errorf("auto-generate EVAL.md for %q: %w", skillName, genErr)
		}
	}

	spec, err := ParseEvalMD(evalPath)
	if err != nil {
		return nil, fmt.Errorf("parse EVAL.md: %w", err)
	}

	if len(spec.Tests) == 0 {
		return nil, fmt.Errorf("no trigger_tests found in EVAL.md for skill %q", skillName)
	}

	skillDesc := spec.Description
	if skillDesc == "" {
		skillDesc = spec.SkillName
	}

	var cases []CaseResult
	correctCount := 0
	passAtKCount := 0
	passKCount := 0

	for _, test := range spec.Tests {
		cr := CaseResult{
			Prompt:   test.Prompt,
			Expected: test.ShouldTrigger,
		}

		// Run k judge calls concurrently — trials are independent.
		type trialResult struct {
			resp judgeResponse
			ok   bool
		}
		results := make([]trialResult, k)
		var wg sync.WaitGroup
		wg.Add(k)
		for i := 0; i < k; i++ {
			i := i
			go func() {
				defer wg.Done()
				jr, callErr := callJudge(ctx, gateway, skillDesc, test.Prompt)
				results[i] = trialResult{resp: jr, ok: callErr == nil}
			}()
		}
		wg.Wait()

		trueCount := 0
		successCount := 0
		var totalConf float64
		for _, r := range results {
			if !r.ok {
				continue
			}
			cr.Trials = append(cr.Trials, r.resp.ShouldTrigger)
			successCount++
			if r.resp.ShouldTrigger {
				trueCount++
			}
			totalConf += r.resp.Confidence
		}

		// If all judge calls failed, this case is unevaluable.
		if successCount == 0 {
			cr.Correct = false
			cases = append(cases, cr)
			continue
		}

		cr.AvgConfidence = totalConf / float64(successCount)

		// Majority vote uses only successful calls.
		majority := trueCount > successCount/2
		cr.Correct = majority == test.ShouldTrigger

		// pass@k: at least one trial is correct
		for _, t := range cr.Trials {
			if t == test.ShouldTrigger {
				cr.PassAtK = true
				break
			}
		}

		// pass^k: all trials are correct
		cr.PassK = true
		for _, t := range cr.Trials {
			if t != test.ShouldTrigger {
				cr.PassK = false
				break
			}
		}

		if cr.Correct {
			correctCount++
		}
		if cr.PassAtK {
			passAtKCount++
		}
		if cr.PassK {
			passKCount++
		}

		cases = append(cases, cr)
	}

	n := len(cases)
	expID := fmt.Sprintf("skill-eval-%s-%s", skillName, time.Now().Format("20060102"))

	return &RunResult{
		Skill:           skillName,
		TriggerAccuracy: float64(correctCount) / float64(n),
		PassAtK:         float64(passAtKCount) / float64(n),
		PassK:           float64(passKCount) / float64(n),
		K:               k,
		TestCount:       n,
		ExpID:           expID,
		Cases:           cases,
	}, nil
}
