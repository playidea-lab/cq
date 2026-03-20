// Package affinity provides worker affinity tracking and scoring for C5 Hub.
//
// Affinity records worker performance history per project, enabling smarter
// task routing by preferring workers with proven success on similar work.
package affinity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

// AffinityRecord holds a worker's historical performance for one project.
type AffinityRecord struct {
	WorkerID     string
	ProjectID    string
	SuccessCount int
	FailCount    int
	LastSuccess  time.Time
	Tags         []string
}

// WorkerScore pairs a worker ID with its computed affinity score.
type WorkerScore struct {
	WorkerID string
	Score    float64
}

// Store provides affinity persistence backed by any *sql.DB (SQLite expected).
type Store struct {
	db *sql.DB
}

// New creates a Store and initialises the schema.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.InitSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// InitSchema creates the worker_affinity table if it does not exist.
func (s *Store) InitSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS worker_affinity (
			worker_id     TEXT NOT NULL,
			project_id    TEXT NOT NULL,
			success_count INTEGER DEFAULT 0,
			fail_count    INTEGER DEFAULT 0,
			last_success  TIMESTAMP,
			tags          TEXT,
			PRIMARY KEY (worker_id, project_id)
		)`)
	if err != nil {
		return fmt.Errorf("affinity: init schema: %w", err)
	}
	return nil
}

// RecordSuccess increments success_count and sets last_success to now for the
// (workerID, projectID) pair. tags is merged into the stored JSON array.
func (s *Store) RecordSuccess(workerID, projectID string, tags []string) error {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("affinity: marshal tags: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO worker_affinity (worker_id, project_id, success_count, fail_count, last_success, tags)
		VALUES (?, ?, 1, 0, ?, ?)
		ON CONFLICT(worker_id, project_id) DO UPDATE SET
			success_count = success_count + 1,
			last_success  = excluded.last_success,
			tags          = excluded.tags`,
		workerID, projectID, now, string(tagsJSON))
	if err != nil {
		return fmt.Errorf("affinity: record success: %w", err)
	}
	return nil
}

// RecordFailure increments fail_count for the (workerID, projectID) pair.
func (s *Store) RecordFailure(workerID, projectID string) error {
	_, err := s.db.Exec(`
		INSERT INTO worker_affinity (worker_id, project_id, success_count, fail_count, last_success, tags)
		VALUES (?, ?, 0, 1, NULL, '[]')
		ON CONFLICT(worker_id, project_id) DO UPDATE SET
			fail_count = fail_count + 1`,
		workerID, projectID)
	if err != nil {
		return fmt.Errorf("affinity: record failure: %w", err)
	}
	return nil
}

// Score computes the affinity score for a (workerID, projectID) pair.
//
// Formula:
//
//	project_match × 10   — success_count for this project
//	tag_overlap   × 3    — |intersection(worker_tags, requiredTags)|
//	recency_bonus × 2    — 1 if last_success within 7 days, else 0
//	success_rate  × 5    — success_count / (success_count + fail_count)
//
// Returns 0 if no record exists.
func (s *Store) Score(workerID, projectID string, requiredTags []string) (float64, error) {
	row := s.db.QueryRow(`
		SELECT success_count, fail_count, last_success, tags
		FROM worker_affinity
		WHERE worker_id = ? AND project_id = ?`,
		workerID, projectID)

	var (
		successCount int
		failCount    int
		lastSuccSQL  sql.NullTime
		tagsJSON     sql.NullString
	)
	if err := row.Scan(&successCount, &failCount, &lastSuccSQL, &tagsJSON); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("affinity: score query: %w", err)
	}

	return computeScore(successCount, failCount, lastSuccSQL, tagsJSON, requiredTags), nil
}

// RankWorkers returns candidateIDs sorted by descending affinity score for the
// given projectID and requiredTags. Workers with no affinity record score 0.
func (s *Store) RankWorkers(projectID string, requiredTags []string, candidateIDs []string) ([]WorkerScore, error) {
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	// Build a placeholder list for the IN clause.
	placeholders := make([]interface{}, 0, len(candidateIDs)+1)
	placeholders = append(placeholders, projectID)
	inClause := ""
	for i, id := range candidateIDs {
		if i > 0 {
			inClause += ","
		}
		inClause += "?"
		placeholders = append(placeholders, id)
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT worker_id, success_count, fail_count, last_success, tags
		FROM worker_affinity
		WHERE project_id = ? AND worker_id IN (%s)`, inClause),
		placeholders...)
	if err != nil {
		return nil, fmt.Errorf("affinity: rank workers query: %w", err)
	}
	defer rows.Close()

	// Collect known records.
	known := make(map[string]WorkerScore, len(candidateIDs))
	for rows.Next() {
		var (
			workerID     string
			successCount int
			failCount    int
			lastSuccSQL  sql.NullTime
			tagsJSON     sql.NullString
		)
		if err := rows.Scan(&workerID, &successCount, &failCount, &lastSuccSQL, &tagsJSON); err != nil {
			return nil, fmt.Errorf("affinity: rank workers scan: %w", err)
		}
		score := computeScore(successCount, failCount, lastSuccSQL, tagsJSON, requiredTags)
		known[workerID] = WorkerScore{WorkerID: workerID, Score: score}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("affinity: rank workers rows: %w", err)
	}

	// Build result, using 0 for unknown workers.
	result := make([]WorkerScore, len(candidateIDs))
	for i, id := range candidateIDs {
		if ws, ok := known[id]; ok {
			result[i] = ws
		} else {
			result[i] = WorkerScore{WorkerID: id, Score: 0}
		}
	}

	// Sort descending by score, stable by workerID for determinism.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].WorkerID < result[j].WorkerID
	})
	return result, nil
}

// GetWorkerAffinity returns all affinity records for the given workerID,
// one entry per project it has interacted with.
func (s *Store) GetWorkerAffinity(workerID string) ([]AffinityRecord, error) {
	rows, err := s.db.Query(`
		SELECT worker_id, project_id, success_count, fail_count, last_success, tags
		FROM worker_affinity
		WHERE worker_id = ?
		ORDER BY project_id`, workerID)
	if err != nil {
		return nil, fmt.Errorf("affinity: get worker affinity: %w", err)
	}
	defer rows.Close()

	var records []AffinityRecord
	for rows.Next() {
		var (
			rec         AffinityRecord
			lastSuccSQL sql.NullTime
			tagsJSON    sql.NullString
		)
		if err := rows.Scan(&rec.WorkerID, &rec.ProjectID, &rec.SuccessCount, &rec.FailCount, &lastSuccSQL, &tagsJSON); err != nil {
			return nil, fmt.Errorf("affinity: get worker affinity scan: %w", err)
		}
		if lastSuccSQL.Valid {
			rec.LastSuccess = lastSuccSQL.Time
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			_ = json.Unmarshal([]byte(tagsJSON.String), &rec.Tags)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("affinity: get worker affinity rows: %w", err)
	}
	return records, nil
}

// computeScore applies the affinity scoring formula.
func computeScore(successCount, failCount int, lastSucc sql.NullTime, tagsJSON sql.NullString, requiredTags []string) float64 {
	// project_match × 10
	projectMatch := float64(successCount) * 10

	// tag_overlap × 3
	var storedTags []string
	if tagsJSON.Valid && tagsJSON.String != "" {
		_ = json.Unmarshal([]byte(tagsJSON.String), &storedTags)
	}
	tagOverlap := float64(tagIntersection(storedTags, requiredTags)) * 3

	// recency_bonus × 2
	var recencyBonus float64
	if lastSucc.Valid && time.Since(lastSucc.Time) <= 7*24*time.Hour {
		recencyBonus = 2
	}

	// success_rate × 5
	var successRate float64
	total := successCount + failCount
	if total > 0 {
		successRate = float64(successCount) / float64(total) * 5
	}

	return projectMatch + tagOverlap + recencyBonus + successRate
}

// tagIntersection returns the count of tags present in both a and b.
func tagIntersection(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, t := range a {
		set[t] = struct{}{}
	}
	count := 0
	for _, t := range b {
		if _, ok := set[t]; ok {
			count++
		}
	}
	return count
}
