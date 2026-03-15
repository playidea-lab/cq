package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// TestDeprecatedSkillsAreRemoved verifies that deprecated skills have been
// deleted and no longer exist in the skills/ directory.
func TestDeprecatedSkillsAreRemoved(t *testing.T) {
	root := archtest.FindRoot(t)
	skillsDir := filepath.Join(root, "../.claude/skills")

	removed := []string{
		"c4-polish",
		"c4-refine",
		"c2-paper-review",
	}

	for _, skill := range removed {
		path := filepath.Join(skillsDir, skill, "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			t.Errorf("deprecated skill %q still exists; it should be deleted", skill)
		}
	}
}

// TestFinishSkillsHaveKnowledgeGate verifies that finish-phase skills record
// outcomes via c4_knowledge_record or c4_experiment_record. This ensures that
// completed work feeds the knowledge base for future sessions.
func TestFinishSkillsHaveKnowledgeGate(t *testing.T) {
	root := archtest.FindRoot(t)
	skillsDir := filepath.Join(root, "../.claude/skills")

	finishSkills := []string{
		"c4-finish",
		"c4-review",
		"research-loop",
	}

	requiredSymbols := []string{
		"c4_knowledge_record",
		"c4_experiment_record",
	}

	for _, skill := range finishSkills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			path := filepath.Join(skillsDir, skill, "SKILL.md")
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				t.Skipf("skill %q not found (skipping)", skill)
			}
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)
			for _, sym := range requiredSymbols {
				if strings.Contains(content, sym) {
					return // found at least one — pass
				}
			}
			t.Errorf("finish skill %q SKILL.md missing knowledge gate; must contain one of: %v",
				skill, requiredSymbols)
		})
	}
}

// TestPlanSkillsHaveKnowledgeRead verifies that plan-phase skills read past
// patterns via c4_knowledge_search or c4_pattern_suggest. This ensures plans
// leverage accumulated knowledge before starting new work.
func TestPlanSkillsHaveKnowledgeRead(t *testing.T) {
	root := archtest.FindRoot(t)
	skillsDir := filepath.Join(root, "../.claude/skills")

	planSkills := []string{
		"c4-plan",
	}

	requiredSymbols := []string{
		"c4_knowledge_search",
		"c4_pattern_suggest",
	}

	for _, skill := range planSkills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			path := filepath.Join(skillsDir, skill, "SKILL.md")
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				t.Skipf("skill %q not found (skipping)", skill)
			}
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)
			for _, sym := range requiredSymbols {
				if strings.Contains(content, sym) {
					return // found at least one — pass
				}
			}
			t.Errorf("plan skill %q SKILL.md missing knowledge read gate; must contain one of: %v",
				skill, requiredSymbols)
		})
	}
}
