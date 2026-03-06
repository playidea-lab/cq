package skilleval

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

const generateEvalPrompt = `You are an expert at writing skill trigger evaluation test cases.

Given a skill description (SKILL.md content), generate an EVAL.md file with 10 test cases:
- 5 prompts that SHOULD trigger the skill (mark with [x])
- 5 prompts that should NOT trigger the skill (mark with [ ])

The prompts should be realistic user messages that an AI assistant would receive.

Skill name: %s
SKILL.md content:
%s

Output ONLY the EVAL.md content in this exact format:
# %s
> <one-line description of what the skill does>

## trigger_tests
- [x] <prompt that should trigger>
- [x] <prompt that should trigger>
- [x] <prompt that should trigger>
- [x] <prompt that should trigger>
- [x] <prompt that should trigger>
- [ ] <prompt that should NOT trigger>
- [ ] <prompt that should NOT trigger>
- [ ] <prompt that should NOT trigger>
- [ ] <prompt that should NOT trigger>
- [ ] <prompt that should NOT trigger>`

// GenerateEvalMD reads the SKILL.md for the given skill, calls the LLM to generate
// test cases, and writes the result to EVAL.md. Returns the path to the written file.
func GenerateEvalMD(ctx context.Context, gateway *llm.Gateway, projectRoot, skillName string) (string, error) {
	skillPath := SkillMDPath(projectRoot, skillName)
	skillContent, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("reading SKILL.md for %q: %w", skillName, err)
	}

	prompt := fmt.Sprintf(generateEvalPrompt, skillName, string(skillContent), skillName)

	ref := gateway.Resolve("scout", "")
	llmCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := gateway.Chat(llmCtx, "scout", &llm.ChatRequest{
		Model:       ref.Model,
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens:   1000,
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("LLM generate eval for %q: %w", skillName, err)
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return "", fmt.Errorf("LLM returned empty content for eval generation")
	}

	evalPath := EvalMDPath(projectRoot, skillName)
	if err := os.WriteFile(evalPath, []byte(content+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("writing EVAL.md: %w", err)
	}

	return evalPath, nil
}
