// Package skilleval provides skill trigger accuracy evaluation.
package skilleval

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TriggerTest is a single test case for skill trigger evaluation.
type TriggerTest struct {
	Prompt        string `json:"prompt"`
	ShouldTrigger bool   `json:"should_trigger"`
}

// EvalSpec describes a skill's evaluation configuration.
type EvalSpec struct {
	SkillName   string        `json:"skill_name"`
	Description string        `json:"description"`
	Tests       []TriggerTest `json:"tests"`
}

// ParseEvalMD parses an EVAL.md file from the given path and returns an EvalSpec.
// Format expected:
//
//	# skill-name
//	> description line
//
//	## trigger_tests
//	- [x] prompt that should trigger
//	- [ ] prompt that should NOT trigger
func ParseEvalMD(path string) (*EvalSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open EVAL.md: %w", err)
	}
	defer f.Close()

	spec := &EvalSpec{}
	inTriggerSection := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Title
		if strings.HasPrefix(trimmed, "# ") && spec.SkillName == "" {
			spec.SkillName = strings.TrimPrefix(trimmed, "# ")
			continue
		}

		// Description (blockquote)
		if strings.HasPrefix(trimmed, "> ") && spec.Description == "" {
			spec.Description = strings.TrimPrefix(trimmed, "> ")
			continue
		}

		// Section header
		if strings.HasPrefix(trimmed, "## ") {
			section := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			inTriggerSection = strings.Contains(section, "trigger")
			continue
		}

		if !inTriggerSection {
			continue
		}

		// Parse test cases: - [x] / - [X] (checked) or - [ ] (unchecked)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "- [x] ") {
			prompt := trimmed[len("- [x] "):]
			spec.Tests = append(spec.Tests, TriggerTest{Prompt: prompt, ShouldTrigger: true})
			continue
		}
		if strings.HasPrefix(trimmed, "- [ ] ") {
			prompt := strings.TrimPrefix(trimmed, "- [ ] ")
			spec.Tests = append(spec.Tests, TriggerTest{Prompt: prompt, ShouldTrigger: false})
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading EVAL.md: %w", err)
	}

	return spec, nil
}

// EvalMDPath returns the canonical EVAL.md path for a skill name given a project root.
func EvalMDPath(projectRoot, skillName string) string {
	return filepath.Join(projectRoot, ".claude", "skills", skillName, "EVAL.md")
}

// SkillMDPath returns the SKILL.md path for a skill name given a project root.
func SkillMDPath(projectRoot, skillName string) string {
	return filepath.Join(projectRoot, ".claude", "skills", skillName, "SKILL.md")
}
