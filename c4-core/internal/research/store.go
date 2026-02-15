// Package research provides a SQLite-backed store for research project tracking.
//
// It tracks paper-experiment iteration loops: projects hold multiple iterations,
// each iteration records review scores, identified gaps, and experiment results.
// Schema is compatible with the Python ResearchStore (c4/research/store.py).
package research

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ProjectStatus represents the lifecycle state of a research project.
type ProjectStatus string

const (
	StatusActive    ProjectStatus = "active"
	StatusPaused    ProjectStatus = "paused"
	StatusCompleted ProjectStatus = "completed"
)

// IterationStatus represents the lifecycle state of a research iteration.
type IterationStatus string

const (
	IterReviewing     IterationStatus = "reviewing"
	IterPlanning      IterationStatus = "planning"
	IterExperimenting IterationStatus = "experimenting"
	IterDone          IterationStatus = "done"
)

// Project represents a research project.
type Project struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	PaperPath        *string       `json:"paper_path"`
	RepoPath         *string       `json:"repo_path"`
	TargetScore      float64       `json:"target_score"`
	CurrentIteration int           `json:"current_iteration"`
	Status           ProjectStatus `json:"status"`
	CreatedAt        *time.Time    `json:"created_at"`
	UpdatedAt        *time.Time    `json:"updated_at"`
}

// Iteration represents a single research iteration.
type Iteration struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	IterationNum int             `json:"iteration_num"`
	ReviewScore  *float64        `json:"review_score"`
	AxisScores   json.RawMessage `json:"axis_scores"`
	Gaps         json.RawMessage `json:"gaps"`
	Experiments  json.RawMessage `json:"experiments"`
	Status       IterationStatus `json:"status"`
	StartedAt    *time.Time      `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at"`
}

// Store provides research project persistence backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the research database at basePath/.c4/research/research.db.
// Uses MaxOpenConns(1) + WAL + busy_timeout to prevent deadlocks.
func NewStore(basePath string) (*Store, error) {
	dbDir := filepath.Join(basePath, ".c4", "research")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create research db dir: %w", err)
	}
	dbPath := filepath.Join(dbDir, "research.db")

	// Note: sql.Open used directly (not openDB) because research.db is an independent
	// database separate from c4.db. MaxOpenConns(1) + WAL prevents deadlocks.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open research db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: research: PRAGMA journal_mode=WAL failed: %v\n", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: research: PRAGMA busy_timeout=5000 failed: %v\n", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: research: PRAGMA foreign_keys=ON failed: %v\n", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("research migrate: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS research_projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			paper_path TEXT,
			repo_path TEXT,
			target_score REAL DEFAULT 7.0,
			current_iteration INTEGER DEFAULT 0,
			status TEXT DEFAULT 'active',
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS research_iterations (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES research_projects(id),
			iteration_num INTEGER NOT NULL,
			review_score REAL,
			axis_scores_json TEXT,
			gaps_json TEXT,
			experiments_json TEXT,
			status TEXT DEFAULT 'reviewing',
			started_at TEXT DEFAULT (datetime('now')),
			completed_at TEXT,
			UNIQUE(project_id, iteration_num)
		);
	`)
	return err
}

// =========================================================================
// Project CRUD
// =========================================================================

// CreateProject creates a new research project and returns its ID.
func (s *Store) CreateProject(name string, paperPath, repoPath *string, targetScore float64) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO research_projects (id, name, paper_path, repo_path, target_score, current_iteration, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, 'active', ?, ?)`,
		id, name, paperPath, repoPath, targetScore, now, now)
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return id, nil
}

// GetProject retrieves a research project by ID.
func (s *Store) GetProject(id string) (*Project, error) {
	row := s.db.QueryRow(`SELECT id, name, paper_path, repo_path, target_score,
		current_iteration, status, created_at, updated_at
		FROM research_projects WHERE id = ?`, id)
	return scanProject(row)
}

// ListProjects returns research projects, optionally filtered by status.
func (s *Store) ListProjects(status string) ([]*Project, error) {
	query := `SELECT id, name, paper_path, repo_path, target_score,
		current_iteration, status, created_at, updated_at
		FROM research_projects`
	args := []any{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProject updates allowed fields on a project.
func (s *Store) UpdateProject(id string, updates map[string]any) error {
	allowed := map[string]bool{
		"name": true, "paper_path": true, "repo_path": true,
		"target_score": true, "current_iteration": true, "status": true,
	}

	setClauses := []string{}
	args := []any{}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		setClauses = append(setClauses, k+" = ?")
		args = append(args, v)
	}
	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	args = append(args, id)

	query := "UPDATE research_projects SET "
	for i, c := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += c
	}
	query += " WHERE id = ?"

	_, err := s.db.Exec(query, args...)
	return err
}

// =========================================================================
// Iteration CRUD
// =========================================================================

// CreateIteration creates a new iteration for a project and returns its ID.
func (s *Store) CreateIteration(projectID string) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get next iteration number
	var nextNum int
	err = tx.QueryRow(`SELECT COALESCE(MAX(iteration_num), 0) + 1
		FROM research_iterations WHERE project_id = ?`, projectID).Scan(&nextNum)
	if err != nil {
		return "", fmt.Errorf("get next iteration num: %w", err)
	}

	_, err = tx.Exec(`INSERT INTO research_iterations
		(id, project_id, iteration_num, status, started_at)
		VALUES (?, ?, ?, 'reviewing', ?)`,
		id, projectID, nextNum, now)
	if err != nil {
		return "", fmt.Errorf("create iteration: %w", err)
	}

	_, err = tx.Exec(`UPDATE research_projects SET current_iteration = ?, updated_at = ? WHERE id = ?`,
		nextNum, now, projectID)
	if err != nil {
		return "", fmt.Errorf("update project iteration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// GetIteration retrieves a single iteration by ID.
func (s *Store) GetIteration(id string) (*Iteration, error) {
	row := s.db.QueryRow(`SELECT id, project_id, iteration_num, review_score,
		axis_scores_json, gaps_json, experiments_json, status, started_at, completed_at
		FROM research_iterations WHERE id = ?`, id)
	return scanIteration(row)
}

// GetCurrentIteration returns the latest iteration for a project.
func (s *Store) GetCurrentIteration(projectID string) (*Iteration, error) {
	row := s.db.QueryRow(`SELECT id, project_id, iteration_num, review_score,
		axis_scores_json, gaps_json, experiments_json, status, started_at, completed_at
		FROM research_iterations WHERE project_id = ?
		ORDER BY iteration_num DESC LIMIT 1`, projectID)
	iter, err := scanIteration(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return iter, err
}

// ListIterations returns all iterations for a project.
func (s *Store) ListIterations(projectID string) ([]*Iteration, error) {
	rows, err := s.db.Query(`SELECT id, project_id, iteration_num, review_score,
		axis_scores_json, gaps_json, experiments_json, status, started_at, completed_at
		FROM research_iterations WHERE project_id = ?
		ORDER BY iteration_num`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list iterations: %w", err)
	}
	defer rows.Close()

	var iters []*Iteration
	for rows.Next() {
		iter, err := scanIterationRow(rows)
		if err != nil {
			return nil, err
		}
		iters = append(iters, iter)
	}
	return iters, rows.Err()
}

// UpdateIteration updates allowed fields on an iteration.
func (s *Store) UpdateIteration(id string, updates map[string]any) error {
	allowed := map[string]bool{
		"review_score": true, "axis_scores": true, "gaps": true,
		"experiments": true, "status": true, "completed_at": true,
	}

	setClauses := []string{}
	args := []any{}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		// JSON fields get serialized
		switch k {
		case "axis_scores", "gaps", "experiments":
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("marshal %s: %w", k, err)
			}
			setClauses = append(setClauses, k+"_json = ?")
			args = append(args, string(jsonBytes))
		default:
			setClauses = append(setClauses, k+" = ?")
			args = append(args, v)
		}
	}
	if len(setClauses) == 0 {
		return nil
	}
	args = append(args, id)

	query := "UPDATE research_iterations SET "
	for i, c := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += c
	}
	query += " WHERE id = ?"

	_, err := s.db.Exec(query, args...)
	return err
}

// =========================================================================
// SuggestNext
// =========================================================================

// SuggestNext recommends the next action for a research project.
func (s *Store) SuggestNext(projectID string) map[string]any {
	project, err := s.GetProject(projectID)
	if err != nil || project == nil {
		return map[string]any{"action": "none", "reason": "Project not found"}
	}

	if project.Status != StatusActive {
		return map[string]any{"action": "none", "reason": fmt.Sprintf("Project is %s", project.Status)}
	}

	current, err := s.GetCurrentIteration(projectID)
	if err != nil || current == nil {
		return map[string]any{"action": "review", "reason": "No iterations yet. Start with a review.", "iteration": 0}
	}

	if current.Status == IterReviewing {
		return map[string]any{"action": "review", "reason": "Review in progress. Record results when done.", "iteration": current.IterationNum}
	}

	if current.ReviewScore != nil && *current.ReviewScore >= project.TargetScore {
		noExpGaps := true
		if current.Gaps != nil {
			var gaps []map[string]any
			if json.Unmarshal(current.Gaps, &gaps) == nil {
				for _, g := range gaps {
					if t, ok := g["type"].(string); ok && t == "experiment" {
						noExpGaps = false
						break
					}
				}
			}
		}
		if noExpGaps {
			return map[string]any{
				"action":    "complete",
				"reason":    fmt.Sprintf("Score %.1f >= target %.1f", *current.ReviewScore, project.TargetScore),
				"iteration": current.IterationNum,
			}
		}
	}

	if current.Status == IterDone {
		return map[string]any{
			"action":    "review",
			"reason":    "Previous iteration complete. Review updated paper.",
			"iteration": current.IterationNum + 1,
		}
	}

	if current.Gaps != nil {
		var gaps []map[string]any
		if json.Unmarshal(current.Gaps, &gaps) == nil {
			pending := 0
			for _, g := range gaps {
				t, _ := g["type"].(string)
				st, _ := g["status"].(string)
				if t == "experiment" && st != "completed" {
					pending++
				}
			}
			if pending > 0 {
				return map[string]any{
					"action":    "run_experiments",
					"reason":    fmt.Sprintf("%d experiments remaining", pending),
					"iteration": current.IterationNum,
				}
			}
		}
	}

	return map[string]any{
		"action":    "plan_experiments",
		"reason":    "Review done. Plan experiments for identified gaps.",
		"iteration": current.IterationNum,
	}
}

// =========================================================================
// Scan helpers
// =========================================================================

type scanner interface {
	Scan(dest ...any) error
}

func populateProject(sc scanner) (*Project, error) {
	var (
		p          Project
		status     string
		paperPath  sql.NullString
		repoPath   sql.NullString
		createdAt  sql.NullString
		updatedAt  sql.NullString
	)
	err := sc.Scan(&p.ID, &p.Name, &paperPath, &repoPath, &p.TargetScore,
		&p.CurrentIteration, &status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.Status = ProjectStatus(status)
	if paperPath.Valid {
		p.PaperPath = &paperPath.String
	}
	if repoPath.Valid {
		p.RepoPath = &repoPath.String
	}
	if createdAt.Valid {
		if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
			p.CreatedAt = &t
		}
	}
	if updatedAt.Valid {
		if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
			p.UpdatedAt = &t
		}
	}
	return &p, nil
}

func scanProject(row *sql.Row) (*Project, error) {
	p, err := populateProject(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan project: %w", err)
	}
	return p, nil
}

func scanProjectRow(rows *sql.Rows) (*Project, error) {
	p, err := populateProject(rows)
	if err != nil {
		return nil, fmt.Errorf("scan project row: %w", err)
	}
	return p, nil
}

func populateIteration(sc scanner) (*Iteration, error) {
	var (
		iter       Iteration
		status     string
		score      sql.NullFloat64
		axisJSON   sql.NullString
		gapsJSON   sql.NullString
		expJSON    sql.NullString
		startedAt  sql.NullString
		completedAt sql.NullString
	)
	err := sc.Scan(&iter.ID, &iter.ProjectID, &iter.IterationNum, &score,
		&axisJSON, &gapsJSON, &expJSON, &status, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	iter.Status = IterationStatus(status)
	if score.Valid {
		iter.ReviewScore = &score.Float64
	}
	if axisJSON.Valid {
		iter.AxisScores = json.RawMessage(axisJSON.String)
	}
	if gapsJSON.Valid {
		iter.Gaps = json.RawMessage(gapsJSON.String)
	}
	if expJSON.Valid {
		iter.Experiments = json.RawMessage(expJSON.String)
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
			iter.StartedAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			iter.CompletedAt = &t
		}
	}
	return &iter, nil
}

func scanIteration(row *sql.Row) (*Iteration, error) {
	iter, err := populateIteration(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan iteration: %w", err)
	}
	return iter, nil
}

func scanIterationRow(rows *sql.Rows) (*Iteration, error) {
	iter, err := populateIteration(rows)
	if err != nil {
		return nil, fmt.Errorf("scan iteration row: %w", err)
	}
	return iter, nil
}
