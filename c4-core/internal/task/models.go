// Package task implements the C4 task store with dependency resolution,
// Review-as-Task workflow, and version management.
package task

import (
	"fmt"
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
)

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

// ParseTaskID normalizes a task ID and extracts components.
func ParseTaskID(id string) (normalized string, baseID string, version int, taskType Type) {
	parts := strings.SplitN(id, "-", 3)
	if len(parts) < 2 {
		return id, id, 0, TypeImplementation
	}

	prefix := parts[0]
	switch prefix {
	case "R":
		taskType = TypeReview
	case "CP":
		taskType = TypeCheckpoint
	default:
		taskType = TypeImplementation
	}

	baseID = parts[1]
	version = 0

	if len(parts) == 3 {
		fmt.Sscanf(parts[2], "%d", &version)
		normalized = id
	} else {
		normalized = fmt.Sprintf("%s-%s-0", prefix, baseID)
	}

	return normalized, baseID, version, taskType
}

// ReviewID generates the review task ID for a given implementation task.
func ReviewID(baseID string, version int) string {
	return fmt.Sprintf("R-%s-%d", baseID, version)
}

// NextVersionID generates the next version of a task.
func NextVersionID(prefix string, baseID string, version int) string {
	return fmt.Sprintf("%s-%s-%d", prefix, baseID, version+1)
}
