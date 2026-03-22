// Package source provides MessageSource implementations for the POP engine.
package source

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/pop"
	"github.com/changmin/c4-core/internal/store"
)

// ReviewSource reads completed review tasks from the task store and surfaces
// approve/reject judgment patterns for the POP engine to learn from.
// Only R-tasks (IDs starting with "R-") with status "done" are considered.
type ReviewSource struct {
	store store.Store
}

// NewReviewSource constructs a ReviewSource backed by the given Store.
func NewReviewSource(s store.Store) *ReviewSource {
	return &ReviewSource{store: s}
}

// RecentMessages implements pop.MessageSource.
// It returns one pop.Message per completed review task whose updated_at is
// after the given time. The message content describes the judgment decision
// (APPROVE or REJECT) and the reason extracted from review_decision_evidence.
func (r *ReviewSource) RecentMessages(_ context.Context, after time.Time, limit int) ([]pop.Message, error) {
	tasks, _, err := r.store.ListTasks(store.TaskFilter{Status: "done", Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("review source: list tasks: %w", err)
	}

	var msgs []pop.Message
	for _, t := range tasks {
		if !strings.HasPrefix(t.ID, "R-") {
			continue
		}

		// Parse updated_at (falls back to created_at) as RFC3339 to respect after filter.
		ts := parseTaskTime(t.UpdatedAt, t.CreatedAt)
		if !after.IsZero() && !ts.After(after) {
			continue
		}

		content := formatReviewPattern(t)
		if content == "" {
			continue
		}

		msgs = append(msgs, pop.Message{
			ID:        "review-" + t.ID,
			Content:   content,
			CreatedAt: ts,
		})

		if limit > 0 && len(msgs) >= limit {
			break
		}
	}
	return msgs, nil
}

// formatReviewPattern returns a human-readable judgment pattern for one review task.
// APPROVE: review_decision_evidence is empty (no changes requested).
// REJECT:  review_decision_evidence contains the reviewer's comments.
func formatReviewPattern(t store.Task) string {
	if t.ReviewDecisionEvidence == "" {
		// Approved — no changes were requested.
		return fmt.Sprintf("APPROVE: task=%s title=%q", t.ID, t.Title)
	}
	// Rejected — evidence holds the request-changes reason.
	return fmt.Sprintf("REJECT: task=%s title=%q reason=%s", t.ID, t.Title, t.ReviewDecisionEvidence)
}

// parseTaskTime tries to parse the preferred time string; falls back to the
// secondary string, and finally to the zero time if both fail.
func parseTaskTime(preferred, fallback string) time.Time {
	if t, err := time.Parse(time.RFC3339, preferred); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, fallback); err == nil {
		return t
	}
	return time.Time{}
}
