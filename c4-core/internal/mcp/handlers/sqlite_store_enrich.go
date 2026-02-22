package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/task"
)

// enrichWithConfig applies config-based branch and worktree assignment.
func (s *SQLiteStore) enrichWithConfig(assignment *TaskAssignment, workerID string) {
	if s.config == nil {
		return
	}

	cfg := s.config.GetConfig()
	if cfg.WorkBranchPrefix != "" {
		assignment.Branch = cfg.WorkBranchPrefix + assignment.TaskID
	}

	if cfg.Worktree.Enabled && s.projectRoot != "" {
		wtPath := filepath.Join(s.projectRoot, ".c4", "worktrees", workerID)
		assignment.WorktreePath = wtPath
		// Auto-create worktree (best-effort, skip if already exists)
		if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
			branch := assignment.Branch
			if branch == "" {
				branch = "c4-" + assignment.TaskID
			}
			if _, wtErr := runGit(s.projectRoot, "worktree", "add", wtPath, "-b", branch); wtErr != nil {
				fmt.Fprintf(os.Stderr, "c4: warning: failed to create worktree %s: %v\n", wtPath, wtErr)
			}
		}
	}
}

// enrichWithSoulContext injects soul context (best-effort).
func (s *SQLiteStore) enrichWithSoulContext(assignment *TaskAssignment) {
	if s.projectRoot != "" {
		s.injectSoulContext(assignment)
	}
}

// enrichWithLighthouse injects lighthouse spec for T-LH- tasks.
func (s *SQLiteStore) enrichWithLighthouse(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "T-LH-") {
		return
	}

	// Extract lighthouse name: T-LH-{name}-{ver}
	parts := strings.TrimPrefix(assignment.TaskID, "T-LH-")
	if idx := strings.LastIndex(parts, "-"); idx > 0 {
		lhName := parts[:idx]
		lh, lhErr := s.getLighthouse(lhName)
		if lhErr == nil {
			assignment.LighthouseSpec = &LighthouseContext{
				Name:        lh.Name,
				Spec:        lh.Spec,
				InputSchema: lh.InputSchema,
				Description: lh.Description,
			}
		} else {
			fmt.Fprintf(os.Stderr, "c4: warning: task %s has T-LH- prefix but lighthouse '%s' not found\n", assignment.TaskID, lhName)
		}
	}
}

// enrichWithReviewContext injects parent T's review context for R- tasks.
func (s *SQLiteStore) enrichWithReviewContext(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "R-") {
		return
	}

	_, baseID, ver, _ := task.ParseTaskID(assignment.TaskID)
	parentID := fmt.Sprintf("T-%s-%d", baseID, ver)
	var commitSHA, filesChangedCol, legacyBranchFiles, handoff string
	if err := s.db.QueryRow("SELECT commit_sha, files_changed, branch, handoff FROM c4_tasks WHERE task_id=?", parentID).Scan(&commitSHA, &filesChangedCol, &legacyBranchFiles, &handoff); err != nil {
		if err != sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "c4: assign-task: review context for %s: %v\n", parentID, err)
		}
	}
	// Prefer the dedicated files_changed column, then handoff JSON, then legacy branch field.
	filesChanged := filesChangedCol
	if filesChanged == "" {
		filesChanged = extractFilesChangedFromHandoff(handoff)
	}
	if filesChanged == "" {
		// Backward compatibility for legacy rows that stored files in branch.
		filesChanged = legacyBranchFiles
	}
	// Extract evidence from parent T handoff
	var evidence []HandoffEvidence
	if handoff != "" {
		if ho := parseHandoff(handoff); len(ho.Evidence) > 0 {
			evidence = ho.Evidence
		}
	}

	if commitSHA != "" || filesChanged != "" || len(evidence) > 0 {
		rc := &ReviewContext{
			ParentTaskID: parentID,
			CommitSHA:    commitSHA,
			FilesChanged: filesChanged,
		}
		if len(evidence) > 0 {
			rc.Evidence = evidence
		}
		assignment.ReviewContext = rc
	}
}

// enrichWithKnowledge injects relevant knowledge context (past patterns, insights, experiments).
func (s *SQLiteStore) enrichWithKnowledge(assignment *TaskAssignment) {
	if s.knowledgeSearch == nil {
		return
	}

	query := assignment.Title
	if assignment.Domain != "" {
		query = assignment.Domain + " " + query
	}

	results, err := s.knowledgeSearch.Search(query, 3, nil)
	if err != nil || len(results) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("## Relevant Knowledge (auto-injected)\n\n")
	for i, r := range results {
		fmt.Fprintf(&b, "### %d. [%s] %s\n", i+1, r.Type, r.Title)
		if r.Domain != "" {
			fmt.Fprintf(&b, "- Domain: %s\n", r.Domain)
		}
		// Fetch body summary (first 200 chars) for actionable context
		if s.knowledgeReader != nil {
			if body, err := s.knowledgeReader.GetBody(r.ID); err == nil && body != "" {
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				fmt.Fprintf(&b, "- Summary: %s\n", body)
			}
		}
		b.WriteString("\n")
	}
	assignment.KnowledgeContext = b.String()
}

// extractFilesChangedFromHandoff is a helper used by enrichWithReviewContext.
func extractFilesChangedFromHandoff(handoff string) string {
	if strings.TrimSpace(handoff) == "" {
		return ""
	}
	var payload struct {
		Type         string   `json:"type"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(handoff), &payload); err != nil {
		return ""
	}
	if payload.Type != "" && payload.Type != "direct_report" {
		return ""
	}
	if len(payload.FilesChanged) == 0 {
		return ""
	}
	return strings.Join(payload.FilesChanged, ",")
}

