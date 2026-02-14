package handlers

import (
	"strings"
	"testing"
)

func TestBuildReviewDoD_MandatoryOnly(t *testing.T) {
	dod := BuildReviewDoD("T-001-0", "Implement login feature", 0)

	// Must include mandatory lenses
	for _, lens := range TaskReviewLenses {
		if !lens.Mandatory {
			continue
		}
		if !strings.Contains(dod, lens.Name) {
			t.Errorf("missing mandatory lens: %s", lens.Name)
		}
		for _, item := range lens.CheckItems {
			if !strings.Contains(dod, item) {
				t.Errorf("missing check item for %s: %s", lens.Name, item)
			}
		}
	}

	// Must NOT include conditional lenses (0 files)
	for _, lens := range TaskReviewLenses {
		if lens.Mandatory {
			continue
		}
		if strings.Contains(dod, lens.Name) {
			t.Errorf("conditional lens %s should not appear with 0 files", lens.Name)
		}
	}

	// Must include original DoD
	if !strings.Contains(dod, "Implement login feature") {
		t.Error("missing original DoD text")
	}

	// Must include parent task ID
	if !strings.Contains(dod, "T-001-0") {
		t.Error("missing parent task ID")
	}
}

func TestBuildReviewDoD_WithConditionalLenses(t *testing.T) {
	dod := BuildReviewDoD("T-002-0", "Big refactor", 7)

	// Must include mandatory lenses
	mandatoryCount := 0
	for _, lens := range TaskReviewLenses {
		if lens.Mandatory && strings.Contains(dod, lens.Name) {
			mandatoryCount++
		}
	}
	if mandatoryCount == 0 {
		t.Error("no mandatory lenses found")
	}

	// Must include conditional lenses (7 >= 5)
	conditionalFound := false
	for _, lens := range TaskReviewLenses {
		if !lens.Mandatory && lens.MinFiles > 0 && 7 >= lens.MinFiles {
			if strings.Contains(dod, lens.Name) {
				conditionalFound = true
			} else {
				t.Errorf("conditional lens %s should appear with 7 files", lens.Name)
			}
		}
	}
	if !conditionalFound {
		t.Error("no conditional lenses found despite 7 files")
	}

	// Must include "Conditional Lenses" header
	if !strings.Contains(dod, "Conditional Lenses") {
		t.Error("missing Conditional Lenses header")
	}
}

func TestBuildReviewDoD_BelowThreshold(t *testing.T) {
	dod := BuildReviewDoD("T-003-0", "Small fix", 3)

	// Conditional lenses should NOT appear (3 < 5)
	if strings.Contains(dod, "Conditional Lenses") {
		t.Error("conditional lenses header should not appear with 3 files")
	}
	if strings.Contains(dod, "Size Audit") {
		t.Error("Size Audit should not appear with 3 files")
	}
}

func TestBuildCheckpointReviewPrompt(t *testing.T) {
	prompt := BuildCheckpointReviewPrompt()

	// Must include all 4 checkpoint lenses
	for _, lens := range CheckpointReviewLenses {
		if !strings.Contains(prompt, lens.Name) {
			t.Errorf("missing checkpoint lens: %s", lens.Name)
		}
		for _, item := range lens.CheckItems {
			if !strings.Contains(prompt, item) {
				t.Errorf("missing check item for %s: %s", lens.Name, item)
			}
		}
	}

	// Must include strategic header
	if !strings.Contains(prompt, "Strategic Review Lenses") {
		t.Error("missing Strategic Review Lenses header")
	}
}

func TestTaskReviewLenses_Invariants(t *testing.T) {
	mandatoryCount := 0
	conditionalCount := 0

	for _, lens := range TaskReviewLenses {
		if lens.ID == "" {
			t.Error("lens has empty ID")
		}
		if lens.Name == "" {
			t.Errorf("lens %s has empty name", lens.ID)
		}
		if len(lens.CheckItems) == 0 {
			t.Errorf("lens %s has no check items", lens.ID)
		}
		if lens.Level != "task" {
			t.Errorf("lens %s level is %q, want 'task'", lens.ID, lens.Level)
		}
		if lens.Mandatory {
			mandatoryCount++
		} else {
			conditionalCount++
		}
	}

	if mandatoryCount != 5 {
		t.Errorf("mandatory lens count = %d, want 5", mandatoryCount)
	}
	if conditionalCount != 3 {
		t.Errorf("conditional lens count = %d, want 3", conditionalCount)
	}
}

func TestCheckpointReviewLenses_Invariants(t *testing.T) {
	if len(CheckpointReviewLenses) != 4 {
		t.Fatalf("checkpoint lens count = %d, want 4", len(CheckpointReviewLenses))
	}

	for _, lens := range CheckpointReviewLenses {
		if lens.ID == "" {
			t.Error("lens has empty ID")
		}
		if !lens.Mandatory {
			t.Errorf("checkpoint lens %s should be mandatory", lens.ID)
		}
		if lens.Level != "checkpoint" {
			t.Errorf("lens %s level is %q, want 'checkpoint'", lens.ID, lens.Level)
		}
		if len(lens.CheckItems) == 0 {
			t.Errorf("lens %s has no check items", lens.ID)
		}
	}
}

func TestBuildReviewDoD_ChecklistFormat(t *testing.T) {
	dod := BuildReviewDoD("T-010-0", "Test task", 10)

	// All check items should be markdown checkboxes
	lines := strings.Split(dod, "\n")
	checkboxCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			checkboxCount++
		}
	}

	// At minimum: 5 mandatory lenses × 2-4 items each + 3 conditional lenses × 2-3 items
	if checkboxCount < 10 {
		t.Errorf("too few checkboxes: %d (expected at least 10)", checkboxCount)
	}
}
