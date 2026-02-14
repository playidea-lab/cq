package handlers

import (
	"fmt"
	"strings"
)

// ReviewLens defines a single review focus area.
type ReviewLens struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CheckItems  []string `json:"check_items"`
	Mandatory   bool     `json:"mandatory"`   // true = always run
	MinFiles    int      `json:"min_files"`    // 0 = always, N = only when N+ files changed
	Level       string   `json:"level"`        // "task" or "checkpoint"
}

// TaskReviewLenses defines the 8 tactical lenses for R-task reviews.
var TaskReviewLenses = []ReviewLens{
	{
		ID:          "purpose",
		Name:        "Purpose Alignment",
		Description: "Does the implementation match the DoD?",
		CheckItems: []string{
			"All DoD items are addressed",
			"No scope creep beyond requirements",
			"Edge cases from DoD are handled",
		},
		Mandatory: true,
		Level:     "task",
	},
	{
		ID:          "security",
		Name:        "Security & Bugs",
		Description: "Path traversal, injection, data leaks, error handling",
		CheckItems: []string{
			"Input validation at system boundaries",
			"No path traversal or injection vectors",
			"Sensitive data not logged or exposed",
			"Error messages don't leak internals",
		},
		Mandatory: true,
		Level:     "task",
	},
	{
		ID:          "regression",
		Name:        "Regression Check",
		Description: "Could changes break existing behavior?",
		CheckItems: []string{
			"Existing tests still pass",
			"Changed interfaces are backward-compatible or callers updated",
			"No silent behavior changes in shared functions",
		},
		Mandatory: true,
		Level:     "task",
	},
	{
		ID:          "side-effects",
		Name:        "Side-Effect Analysis",
		Description: "Unintended impacts on other systems or state",
		CheckItems: []string{
			"State changes are intentional and documented",
			"Concurrent access is safe (no races)",
			"Resource cleanup on error paths (files, connections)",
		},
		Mandatory: true,
		Level:     "task",
	},
	{
		ID:          "quality",
		Name:        "Code Quality",
		Description: "Naming, structure, readability, idiomatic patterns",
		CheckItems: []string{
			"Naming is clear and consistent with codebase",
			"Functions have single responsibility",
			"Error handling follows project patterns",
		},
		Mandatory: true,
		Level:     "task",
	},
	{
		ID:          "size-audit",
		Name:        "Size Audit",
		Description: "Large functions or files that should be split",
		CheckItems: []string{
			"No functions over 100 lines",
			"No files over 500 lines (new files)",
			"Complex logic extracted into helpers",
		},
		Mandatory: false,
		MinFiles:  5,
		Level:     "task",
	},
	{
		ID:          "dry-check",
		Name:        "DRY Check",
		Description: "Duplicated logic across files",
		CheckItems: []string{
			"No copy-pasted logic blocks",
			"Shared patterns extracted (only if used 3+ times)",
		},
		Mandatory: false,
		MinFiles:  5,
		Level:     "task",
	},
	{
		ID:          "dead-code",
		Name:        "Dead Code Cleanup",
		Description: "Unused imports, functions, variables",
		CheckItems: []string{
			"No orphaned imports or variables",
			"Removed code is fully deleted (no commented-out blocks)",
		},
		Mandatory: false,
		MinFiles:  5,
		Level:     "task",
	},
}

// CheckpointReviewLenses defines the 4 strategic lenses for checkpoint reviews.
var CheckpointReviewLenses = []ReviewLens{
	{
		ID:          "holistic",
		Name:        "Holistic Review",
		Description: "All changes together — coherent architecture?",
		CheckItems: []string{
			"Changes form a coherent whole",
			"Architecture decisions are consistent",
			"No conflicting patterns introduced",
		},
		Mandatory: true,
		Level:     "checkpoint",
	},
	{
		ID:          "user-flow",
		Name:        "User Flow Validation",
		Description: "End-to-end functionality works from user perspective",
		CheckItems: []string{
			"Primary user flows are tested",
			"Error states have user-friendly handling",
			"Performance is acceptable",
		},
		Mandatory: true,
		Level:     "checkpoint",
	},
	{
		ID:          "cascade",
		Name:        "Cascade Review",
		Description: "Issues from earlier reviews fully resolved",
		CheckItems: []string{
			"All REQUEST_CHANGES items addressed",
			"Fixes didn't introduce new issues",
			"Previous review feedback incorporated",
		},
		Mandatory: true,
		Level:     "checkpoint",
	},
	{
		ID:          "ship-ready",
		Name:        "Ship-Ready Gate",
		Description: "Production-worthy? Monitoring? Rollback plan?",
		CheckItems: []string{
			"Tests pass (lint + unit + integration)",
			"No TODO/FIXME left in changed code",
			"Rollback is possible if issues found",
		},
		Mandatory: true,
		Level:     "checkpoint",
	},
}

// BuildReviewDoD generates a review checklist to append to an R-task's DoD.
// filesChanged is used to determine which conditional lenses to include.
func BuildReviewDoD(parentTaskID, originalDoD string, filesChanged int) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Review: %s\n\n", parentTaskID))

	// Mandatory lenses
	b.WriteString("### Mandatory Lenses\n")
	for _, lens := range TaskReviewLenses {
		if !lens.Mandatory {
			continue
		}
		b.WriteString(fmt.Sprintf("\n**[%s] %s**: %s\n", lens.ID, lens.Name, lens.Description))
		for _, item := range lens.CheckItems {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", item))
		}
	}

	// Conditional lenses (only when threshold met)
	var conditionalLenses []ReviewLens
	for _, lens := range TaskReviewLenses {
		if !lens.Mandatory && lens.MinFiles > 0 && filesChanged >= lens.MinFiles {
			conditionalLenses = append(conditionalLenses, lens)
		}
	}
	if len(conditionalLenses) > 0 {
		b.WriteString(fmt.Sprintf("\n### Conditional Lenses (%d+ files changed)\n", conditionalLenses[0].MinFiles))
		for _, lens := range conditionalLenses {
			b.WriteString(fmt.Sprintf("\n**[%s] %s**: %s\n", lens.ID, lens.Name, lens.Description))
			for _, item := range lens.CheckItems {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", item))
			}
		}
	}

	// Original DoD
	b.WriteString("\n### Original DoD\n")
	b.WriteString(originalDoD)

	return b.String()
}

// BuildCheckpointReviewPrompt generates the strategic review checklist for checkpoints.
func BuildCheckpointReviewPrompt() string {
	var b strings.Builder

	b.WriteString("## Strategic Review Lenses\n\n")
	for _, lens := range CheckpointReviewLenses {
		b.WriteString(fmt.Sprintf("### [%s] %s\n", lens.ID, lens.Name))
		b.WriteString(fmt.Sprintf("%s\n", lens.Description))
		for _, item := range lens.CheckItems {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", item))
		}
		b.WriteString("\n")
	}

	return b.String()
}
