// Package task implements the C4 task store with dependency resolution,
// Review-as-Task workflow, and version management.
package task

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Status represents a task's lifecycle state.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusBlocked    Status = "blocked"
)

// Type represents the kind of task.
type Type string

const (
	TypeImplementation Type = "IMPLEMENTATION"
	TypeReview         Type = "REVIEW"
	TypeCheckpoint     Type = "CHECKPOINT"
	TypeRefine         Type = "REFINE"
)

var baseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Task represents a C4 task with Review-as-Task versioning.
type Task struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Scope        string   `json:"scope,omitempty"`
	Priority     int      `json:"priority"`
	DoD          string   `json:"dod"`
	Dependencies []string `json:"dependencies"`
	Status       Status   `json:"status"`
	AssignedTo   string   `json:"assigned_to,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	CommitSHA    string   `json:"commit_sha,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Model        string   `json:"model"`

	// Review-as-Task fields
	Type        Type   `json:"type"`
	BaseID      string `json:"base_id,omitempty"`
	Version     int    `json:"version"`
	ParentID    string `json:"parent_id,omitempty"`
	CompletedBy string `json:"completed_by,omitempty"`

	// Optimistic locking (Supabase row_version)
	RowVersion int `json:"row_version,omitempty"`

	// Timestamps
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// ValidateTaskID validates task ID grammar.
//
// Grammar:
//   - Implementation: T-<base>-<version> (legacy T-<base> is accepted and normalized to -0)
//   - Review:         R-<base>-<version> (legacy R-<base> is accepted and normalized to -0)
//   - Refine:         RF-<base>-<version> (iterative review-fix loop task)
//   - Repair:         RPR-<base>-<version> (legacy RPR-<base> is accepted and normalized to -0)
//   - Checkpoint:     CP-<base>
//
// base: [A-Za-z0-9][A-Za-z0-9_-]*
// version: non-negative integer
func ValidateTaskID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("task_id is required")
	}

	switch {
	case strings.HasPrefix(id, "CP-"):
		baseID := strings.TrimPrefix(id, "CP-")
		if !baseIDPattern.MatchString(baseID) {
			return fmt.Errorf("invalid task_id format: %s", id)
		}
		return nil
	case strings.HasPrefix(id, "T-"):
		return validateVersionedTaskIDBody(strings.TrimPrefix(id, "T-"), id)
	case strings.HasPrefix(id, "RPR-"):
		return validateVersionedTaskIDBody(strings.TrimPrefix(id, "RPR-"), id)
	case strings.HasPrefix(id, "RF-"):
		return validateVersionedTaskIDBody(strings.TrimPrefix(id, "RF-"), id)
	case strings.HasPrefix(id, "R-"):
		return validateVersionedTaskIDBody(strings.TrimPrefix(id, "R-"), id)
	default:
		return fmt.Errorf("invalid task_id format: %s", id)
	}
}

func validateVersionedTaskIDBody(body string, original string) error {
	if body == "" {
		return fmt.Errorf("invalid task_id format: %s", original)
	}

	// Preferred format: <base>-<version> (right-most '-' splits version)
	if idx := strings.LastIndex(body, "-"); idx > 0 {
		basePart := body[:idx]
		versionPart := body[idx+1:]
		if isDigits(versionPart) {
			if !baseIDPattern.MatchString(basePart) {
				return fmt.Errorf("invalid task_id format: %s", original)
			}
			return nil
		}
	}

	// Legacy format without version suffix.
	if baseIDPattern.MatchString(body) {
		return nil
	}
	return fmt.Errorf("invalid task_id format: %s", original)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ParseTaskID normalizes a task ID and extracts components.
func ParseTaskID(id string) (normalized string, baseID string, version int, taskType Type) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", 0, TypeImplementation
	}

	switch {
	case strings.HasPrefix(id, "CP-"):
		baseID = strings.TrimPrefix(id, "CP-")
		if baseID == "" {
			return id, id, 0, TypeCheckpoint
		}
		return "CP-" + baseID, baseID, 0, TypeCheckpoint
	case strings.HasPrefix(id, "RF-"):
		return parseVersionedTaskID(id, "RF", TypeRefine)
	case strings.HasPrefix(id, "RPR-"):
		return parseVersionedTaskID(id, "RPR", TypeImplementation)
	case strings.HasPrefix(id, "R-"):
		return parseVersionedTaskID(id, "R", TypeReview)
	case strings.HasPrefix(id, "T-"):
		return parseVersionedTaskID(id, "T", TypeImplementation)
	default:
		return id, id, 0, TypeImplementation
	}
}

func parseVersionedTaskID(id string, prefix string, taskType Type) (normalized string, baseID string, version int, parsedType Type) {
	parsedType = taskType
	body := strings.TrimPrefix(id, prefix+"-")
	if body == "" {
		return id, id, 0, parsedType
	}

	baseID = body
	version = 0
	if idx := strings.LastIndex(body, "-"); idx > 0 {
		basePart := body[:idx]
		versionPart := body[idx+1:]
		if isDigits(versionPart) {
			if v, err := strconv.Atoi(versionPart); err == nil {
				baseID = basePart
				version = v
			}
		}
	}
	return fmt.Sprintf("%s-%s-%d", prefix, baseID, version), baseID, version, parsedType
}

// ReviewID generates the review task ID for a given implementation task.
func ReviewID(baseID string, version int) string {
	return fmt.Sprintf("R-%s-%d", baseID, version)
}

// NextVersionID generates the next version of a task.
func NextVersionID(prefix string, baseID string, version int) string {
	return fmt.Sprintf("%s-%s-%d", prefix, baseID, version+1)
}
