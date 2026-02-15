package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// GetStatus returns the current project status with task counts.
func (s *SQLiteStore) GetStatus() (*ProjectStatus, error) {
	status := &ProjectStatus{State: "INIT", ProjectName: s.projectID}

	// Read state
	var stateJSON string
	err := s.db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err == nil {
		var m map[string]any
		if jsonErr := json.Unmarshal([]byte(stateJSON), &m); jsonErr == nil {
			if st, ok := m["status"].(string); ok {
				status.State = st
			}
			if pn, ok := m["project_id"].(string); ok {
				status.ProjectName = pn
			}
		}
	}

	// Count tasks by status
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM c4_tasks GROUP BY status")
	if err != nil {
		return status, nil
	}
	defer rows.Close()

	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			continue
		}
		status.TotalTasks += count
		switch st {
		case "pending":
			status.PendingTasks = count
		case "in_progress":
			status.InProgress = count
		case "done":
			status.DoneTasks = count
		case "blocked":
			status.BlockedTasks = count
		}
	}

	// Calculate how many pending tasks are runnable now (all deps done).
	// This helps direct-mode operators pick tasks without extra DB queries.
	if err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM c4_tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM json_each(CASE WHEN t.dependencies IS NULL OR t.dependencies = '' THEN '[]' ELSE t.dependencies END) AS dep
			JOIN c4_tasks dt ON dt.task_id = dep.value
			WHERE dt.status != 'done'
		)`).
		Scan(&status.ReadyTasks); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-status: ready tasks count: %v\n", err)
	}
	status.BlockedByDeps = status.PendingTasks - status.ReadyTasks
	if status.BlockedByDeps < 0 {
		status.BlockedByDeps = 0
	}

	readyRows, err := s.db.Query(`
		SELECT t.task_id
		FROM c4_tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM json_each(CASE WHEN t.dependencies IS NULL OR t.dependencies = '' THEN '[]' ELSE t.dependencies END) AS dep
			JOIN c4_tasks dt ON dt.task_id = dep.value
			WHERE dt.status != 'done'
		)
		ORDER BY t.priority DESC, t.created_at ASC
		LIMIT 10`)
	if err == nil {
		defer readyRows.Close()
		for readyRows.Next() {
			var taskID string
			if scanErr := readyRows.Scan(&taskID); scanErr == nil {
				status.ReadyTaskIDs = append(status.ReadyTaskIDs, taskID)
			}
		}
	}

	// Lighthouse counts
	var lhStubs, lhImpl int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_lighthouses WHERE status='stub'").Scan(&lhStubs); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-status: lighthouse stubs count: %v\n", err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_lighthouses WHERE status='implemented'").Scan(&lhImpl); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-status: lighthouse implemented count: %v\n", err)
	}
	status.LighthouseStubs = lhStubs
	status.LighthouseImplemented = lhImpl

	// Orphan review count: R-tasks still pending/in_progress whose parent T-task is done
	var orphans int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM c4_tasks r
		WHERE r.task_id LIKE 'R-%' AND r.status IN ('pending','in_progress')
		AND EXISTS (
			SELECT 1 FROM c4_tasks t
			WHERE t.task_id = REPLACE(r.task_id, 'R-', 'T-') AND t.status = 'done'
		)`).Scan(&orphans); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-status: orphan reviews count: %v\n", err)
	}
	status.OrphanReviews = orphans

	// Add active soul roles for current workflow stage
	status.ActiveSoulRoles = GetActiveRolesForStage(status.State)

	// Persona digest (lightweight, best-effort)
	var pTotal, pApproved int
	if err := s.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM persona_stats`).Scan(&pTotal, &pApproved); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-status: persona digest: %v\n", err)
	}
	if pTotal > 0 {
		status.PersonaDigest = &PersonaSummary{
			TotalTasks:   pTotal,
			ApprovalRate: float64(pApproved) / float64(pTotal),
		}
	}

	// Add economic mode and worker config info if config is available
	if s.config != nil {
		cfg := s.config.GetConfig()
		routing := cfg.EconomicMode.ModelRouting
		status.EconomicMode = &EconomicModeInfo{
			Enabled: cfg.EconomicMode.Enabled,
			Preset:  cfg.EconomicMode.Preset,
			ModelRouting: map[string]string{
				"implementation": routing.Implementation,
				"review":         routing.Review,
				"checkpoint":     routing.Checkpoint,
			},
		}
		status.WorkerConfig = &WorkerConfigInfo{
			WorkBranchPrefix: cfg.WorkBranchPrefix,
			DefaultBranch:    cfg.DefaultBranch,
			WorktreeEnabled:  cfg.Worktree.Enabled,
			ReviewAsTask:     cfg.ReviewAsTask,
			MaxRevision:      cfg.MaxRevision,
		}
	}

	return status, nil
}

// GetPersonaStats retrieves performance stats for a persona.
func (s *SQLiteStore) GetPersonaStats(personaID string) (map[string]any, error) {
	stats := map[string]any{
		"persona_id": personaID,
	}

	// Total tasks
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM persona_stats WHERE persona_id = ?", personaID).Scan(&total); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-persona-stats: total count: %v\n", err)
	}
	stats["total_tasks"] = total

	if total == 0 {
		return stats, nil
	}

	// Outcome breakdown
	rows, err := s.db.Query("SELECT outcome, COUNT(*) FROM persona_stats WHERE persona_id = ? GROUP BY outcome", personaID)
	if err != nil {
		return stats, nil
	}
	defer rows.Close()

	outcomes := map[string]int{}
	for rows.Next() {
		var outcome string
		var count int
		if err := rows.Scan(&outcome, &count); err != nil {
			continue
		}
		outcomes[outcome] = count
	}
	stats["outcomes"] = outcomes

	// Average review score
	var avgScore sql.NullFloat64
	if err := s.db.QueryRow("SELECT AVG(review_score) FROM persona_stats WHERE persona_id = ? AND review_score > 0", personaID).Scan(&avgScore); err != nil {
		fmt.Fprintf(os.Stderr, "c4: get-persona-stats: avg score: %v\n", err)
	}
	if avgScore.Valid {
		stats["avg_review_score"] = avgScore.Float64
	}

	return stats, nil
}

// ListPersonas returns all known persona IDs with their task counts.
func (s *SQLiteStore) ListPersonas() ([]map[string]any, error) {
	rows, err := s.db.Query("SELECT persona_id, COUNT(*), AVG(review_score) FROM persona_stats GROUP BY persona_id ORDER BY COUNT(*) DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var personas []map[string]any
	for rows.Next() {
		var pid string
		var count int
		var avgScore sql.NullFloat64
		if err := rows.Scan(&pid, &count, &avgScore); err != nil {
			continue
		}
		p := map[string]any{
			"persona_id":  pid,
			"total_tasks": count,
		}
		if avgScore.Valid {
			p["avg_review_score"] = avgScore.Float64
		}
		personas = append(personas, p)
	}
	return personas, nil
}

// TaskFilter defines filtering criteria for ListTasks.
type TaskFilter struct {
	Status   string `json:"status,omitempty"`
	Domain   string `json:"domain,omitempty"`
	WorkerID string `json:"worker_id,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// ListTasks returns tasks matching the given filter with priority DESC, created_at ASC ordering.
func (s *SQLiteStore) ListTasks(filter TaskFilter) ([]Task, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	// Total count (unfiltered)
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT task_id, title, status, priority, domain, worker_id, created_at, dod FROM c4_tasks`
	var conditions []string
	var args []any

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Domain != "" {
		conditions = append(conditions, "domain = ?")
		args = append(args, filter.Domain)
	}
	if filter.WorkerID != "" {
		conditions = append(conditions, "worker_id = ?")
		args = append(args, filter.WorkerID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY priority DESC, created_at ASC LIMIT ?"
	args = append(args, filter.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var domain, workerID, createdAt, dod sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &domain, &workerID, &createdAt, &dod); err != nil {
			continue
		}
		t.Domain = domain.String
		t.WorkerID = workerID.String
		t.CreatedAt = createdAt.String
		t.DoD = dod.String
		tasks = append(tasks, t)
	}

	return tasks, total, rows.Err()
}
