package skilleval

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
)

// SkillOptimizer orchestrates the autonomous skill optimization loop:
// baseline → mutation → eval → keep/discard → repeat.
type SkillOptimizer struct {
	ProjectDir string
	LLM        *llm.Gateway
	KStore     *knowledge.Store
}

// OptimizeOpts controls the optimization loop parameters.
type OptimizeOpts struct {
	RunsPerExperiment int     // Number of simulated runs per experiment round.
	BudgetCap         int     // Maximum number of mutation experiments before stopping.
	TargetPassRate    float64 // Stop early if pass rate >= this for 3 consecutive rounds.
}

// OptimizeResult summarizes the optimization outcome.
type OptimizeResult struct {
	Baseline    float64 `json:"baseline"`
	FinalScore  float64 `json:"final_score"`
	Experiments int     `json:"experiments"`
	KeptCount   int     `json:"kept_count"`
}

const simulateSystemTmpl = `You are executing this skill:
%s

Input: %s

Produce the output as if you were the skill. Output only the result — no explanations.`

// simulateRun asks the LLM to produce output as if executing the skill.
func (o *SkillOptimizer) simulateRun(ctx context.Context, skillContent, testInput string) (string, error) {
	ref := o.LLM.Resolve("sonnet", "")

	llmCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	systemPrompt := fmt.Sprintf(simulateSystemTmpl, skillContent, testInput)

	resp, err := o.LLM.Chat(llmCtx, "sonnet", &llm.ChatRequest{
		Model:       ref.Model,
		System:      systemPrompt,
		Messages:    []llm.Message{{Role: "user", Content: "Execute the skill and produce the output."}},
		MaxTokens:   1500,
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("simulate run: %w", err)
	}

	return resp.Content, nil
}

// measurePassRate runs N simulations and scores each output against evals,
// returning the overall pass rate and the collected EvalResult details from all runs.
func (o *SkillOptimizer) measurePassRate(ctx context.Context, skillContent string, evals []BinaryEval, n int) (float64, []EvalResult, error) {
	if n <= 0 {
		n = 1
	}

	totalPassed := 0
	totalEvals := 0
	var allDetails []EvalResult

	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}

		testInput := fmt.Sprintf("test input %d", i+1)
		output, err := o.simulateRun(ctx, skillContent, testInput)
		if err != nil {
			continue
		}

		passed, total, details, err := ScoreOutput(ctx, o.LLM, output, evals)
		if err != nil {
			continue
		}

		totalPassed += passed
		totalEvals += total
		allDetails = append(allDetails, details...)
	}

	if totalEvals == 0 {
		return 0, allDetails, nil
	}

	return float64(totalPassed) / float64(totalEvals), allDetails, nil
}

// Run executes the full optimization loop for a skill.
func (o *SkillOptimizer) Run(ctx context.Context, skillName string, evals []BinaryEval, opts OptimizeOpts) (*OptimizeResult, error) {
	if opts.RunsPerExperiment <= 0 {
		opts.RunsPerExperiment = 3
	}
	if opts.BudgetCap <= 0 {
		opts.BudgetCap = 10
	}
	if opts.TargetPassRate <= 0 {
		opts.TargetPassRate = 0.9
	}

	// 1. Read SKILL.md
	skillPath := SkillMDPath(o.ProjectDir, skillName)
	original, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	skillContent := string(original)

	// Backup as SKILL.md.baseline
	backupPath := skillPath + ".baseline"
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return nil, fmt.Errorf("write baseline backup: %w", err)
	}

	// 2. Measure baseline
	baselineRate, baselineDetails, err := o.measurePassRate(ctx, skillContent, evals, opts.RunsPerExperiment)
	if err != nil {
		return nil, fmt.Errorf("baseline measurement: %w", err)
	}

	bestPassRate := baselineRate
	bestContent := skillContent
	keptCount := 0
	consecutiveTarget := 0

	result := &OptimizeResult{
		Baseline: baselineRate,
	}

	// 3. Optimization loop
	for exp := 0; exp < opts.BudgetCap; exp++ {
		if ctx.Err() != nil {
			break
		}

		// a. Analyze failures from last round
		var detailsToAnalyze []EvalResult
		if exp == 0 {
			detailsToAnalyze = baselineDetails
		} else {
			// Re-measure current best to get fresh details for analysis
			_, freshDetails, measureErr := o.measurePassRate(ctx, bestContent, evals, opts.RunsPerExperiment)
			if measureErr != nil {
				break
			}
			detailsToAnalyze = freshDetails
		}

		failureAnalysis := AnalyzeFailures(detailsToAnalyze)

		// b. Propose mutation
		mutated, description, mutErr := ProposeMutation(ctx, o.LLM, bestContent, failureAnalysis)
		if mutErr != nil {
			break
		}

		// c. Write mutated SKILL.md
		if err := os.WriteFile(skillPath, []byte(mutated), 0o644); err != nil {
			break
		}

		// d. Measure mutated pass rate
		mutatedRate, _, scoreErr := o.measurePassRate(ctx, mutated, evals, opts.RunsPerExperiment)
		if scoreErr != nil {
			// Restore best on error
			_ = os.WriteFile(skillPath, []byte(bestContent), 0o644)
			break
		}

		result.Experiments = exp + 1

		// e/f. Keep or discard
		status := "discard"
		if mutatedRate > bestPassRate {
			bestPassRate = mutatedRate
			bestContent = mutated
			keptCount++
			status = "keep"
		} else {
			// Restore best content
			_ = os.WriteFile(skillPath, []byte(bestContent), 0o644)
		}

		// g. Record experiment in knowledge store
		if o.KStore != nil {
			expID := fmt.Sprintf("skill-opt-%s-%d", skillName, exp+1)
			_, _ = o.KStore.Create("experiment", map[string]any{
				"title":  fmt.Sprintf("Skill optimization: %s exp %d", skillName, exp+1),
				"domain": "skilleval",
				"tags":   []string{"skill-optimization", skillName},
			}, fmt.Sprintf("experiment_id: %s\nscore: %.4f\nstatus: %s\ndescription: %s",
				expID, mutatedRate, status, description))
		}

		// h. Convergence check
		if mutatedRate >= opts.TargetPassRate {
			consecutiveTarget++
			if consecutiveTarget >= 3 {
				break
			}
		} else {
			consecutiveTarget = 0
		}
	}

	// 4. Final SKILL.md = bestContent
	_ = os.WriteFile(skillPath, []byte(bestContent), 0o644)

	result.FinalScore = bestPassRate
	result.KeptCount = keptCount

	return result, nil
}
