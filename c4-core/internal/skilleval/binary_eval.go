package skilleval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

// BinaryEval defines a single yes/no quality criterion for evaluating skill output.
type BinaryEval struct {
	Name     string
	Question string
	Pass     string
	Fail     string
}

// EvalResult holds the outcome of one BinaryEval applied to a skill output.
type EvalResult struct {
	Name      string
	Passed    bool
	Reasoning string
}

const binaryEvalSystemTmpl = "Answer YES or NO only. Question: %s\nPass condition: %s\nFail condition: %s\n\nOutput to evaluate:\n%s"

// ScoreOutput evaluates a skill output against a list of BinaryEval criteria using an LLM.
// It calls the "haiku" route once per eval and returns the count of passed evals,
// the total number of evals, and per-eval details.
func ScoreOutput(ctx context.Context, gw *llm.Gateway, output string, evals []BinaryEval) (passed int, total int, details []EvalResult, err error) {
	total = len(evals)
	details = make([]EvalResult, 0, total)

	ref := gw.Resolve("haiku", "")

	for _, ev := range evals {
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}

		systemPrompt := fmt.Sprintf(binaryEvalSystemTmpl, ev.Question, ev.Pass, ev.Fail, output)

		llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		resp, callErr := gw.Chat(llmCtx, "haiku", &llm.ChatRequest{
			Model:       ref.Model,
			System:      systemPrompt,
			Messages:    []llm.Message{{Role: "user", Content: "Evaluate the output above."}},
			MaxTokens:   10,
			Temperature: 0.0,
		})
		cancel()

		result := EvalResult{Name: ev.Name}
		if callErr != nil {
			result.Reasoning = fmt.Sprintf("LLM error: %v", callErr)
			details = append(details, result)
			continue
		}

		answer := strings.TrimSpace(resp.Content)
		result.Reasoning = answer
		if strings.HasPrefix(strings.ToUpper(answer), "YES") {
			result.Passed = true
			passed++
		}

		details = append(details, result)
	}

	return passed, total, details, nil
}
