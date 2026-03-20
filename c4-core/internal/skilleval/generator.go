package skilleval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

// evalGuide is embedded from the Autoresearch binary eval writing guide.
const evalGuide = `## Binary Eval Writing Guide

Every eval must be a yes/no question. Not a scale. Not a vibe check. Binary.

Good evals are:
- Specific and checkable by an agent without ambiguity
- Testing something the user actually cares about (not arbitrary constraints)
- Distinct from each other (no overlapping coverage)
- Not gameable — the skill can't pass without actually improving

Bad evals:
- "Is the output good?" (too vague)
- "Rate quality 1-10" (scale, not binary)
- "Would a human find this engaging?" (unmeasurable by an agent)

The 3-question test before finalizing an eval:
1. Could two different agents score the same output and agree?
2. Could a skill game this eval without actually improving?
3. Does this eval test something the user actually cares about?

Keep to 3-5 evals maximum — more than 6 and the skill optimizes for tests instead of quality.`

const generateEvalPrompt = `You are an expert at writing skill evaluation test cases.

Given a skill description (SKILL.md content), generate an EVAL.md file with:
1. 10 trigger test cases:
   - 5 prompts that SHOULD trigger the skill (mark with [x])
   - 5 prompts that should NOT trigger the skill (mark with [ ])
2. 3-5 output quality evals that assess the skill's core output quality using binary yes/no questions.

The prompts should be realistic user messages that an AI assistant would receive.

%s

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
- [ ] <prompt that should NOT trigger>

## Output Quality Evals
EVAL 1: <eval name> / Question: <binary yes/no question about output quality> / Pass: <what a passing output looks like> / Fail: <what a failing output looks like>
EVAL 2: <eval name> / Question: <binary yes/no question about output quality> / Pass: <what a passing output looks like> / Fail: <what a failing output looks like>
EVAL 3: <eval name> / Question: <binary yes/no question about output quality> / Pass: <what a passing output looks like> / Fail: <what a failing output looks like>`

// GenerateEvalMD reads the SKILL.md for the given skill, calls the LLM to generate
// test cases, and writes the result to EVAL.md. Returns the path to the written file.
func GenerateEvalMD(ctx context.Context, gateway *llm.Gateway, projectRoot, skillName string) (string, error) {
	skillPath := SkillMDPath(projectRoot, skillName)
	skillContent, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("reading SKILL.md for %q: %w", skillName, err)
	}

	prompt := fmt.Sprintf(generateEvalPrompt, evalGuide, skillName, string(skillContent), skillName)

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

// GenerateAllEvalMD generates EVAL.md for every skill that has a SKILL.md
// but no EVAL.md. Returns lists of generated skill names and failed skill names.
func GenerateAllEvalMD(ctx context.Context, gateway *llm.Gateway, projectRoot string) (generated []string, failed []string) {
	skillsDir := filepath.Join(projectRoot, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		evalPath := EvalMDPath(projectRoot, name)
		if _, statErr := os.Stat(evalPath); statErr == nil {
			// Already exists — skip.
			continue
		}
		if _, genErr := GenerateEvalMD(ctx, gateway, projectRoot, name); genErr != nil {
			failed = append(failed, name)
		} else {
			generated = append(generated, name)
		}
	}
	return generated, failed
}
