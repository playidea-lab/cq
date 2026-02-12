package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var (
	taskTitle    string
	taskScope    string
	taskPriority int
	taskDepends  []string
	taskDoD      string
)

var addTaskCmd = &cobra.Command{
	Use:   "add-task",
	Short: "Add a new task",
	Long: `Add a task to the C4 project. Tasks are tracked in .c4/tasks.db
and can be assigned to workers during execution.`,
	RunE: runAddTask,
}

func init() {
	addTaskCmd.Flags().StringVarP(&taskTitle, "title", "t", "", "task title (required)")
	addTaskCmd.Flags().StringVar(&taskDoD, "dod", "", "definition of done (default: title)")
	addTaskCmd.Flags().StringVarP(&taskScope, "scope", "s", "", "file/directory scope")
	addTaskCmd.Flags().IntVarP(&taskPriority, "priority", "p", 5, "priority (1=highest, 10=lowest)")
	addTaskCmd.Flags().StringSliceVarP(&taskDepends, "depends", "d", nil, "task IDs this depends on")
	_ = addTaskCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addTaskCmd)
}

// task represents a row in c4_tasks.
type task struct {
	TaskID    string   `json:"task_id"`
	Title     string   `json:"title"`
	Scope     string   `json:"scope,omitempty"`
	DoD       string   `json:"dod"`
	Priority  int      `json:"priority"`
	Status    string   `json:"status"`
	DependsOn []string `json:"dependencies,omitempty"`
	CreatedAt string   `json:"created_at"`
}

func runAddTask(cmd *cobra.Command, args []string) error {
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Ensure the tasks table exists
	if err := ensureTasksTable(db); err != nil {
		return err
	}

	// Generate next task ID
	taskID, err := nextTaskID(db)
	if err != nil {
		return err
	}

	// Build the task
	dod := taskDoD
	if dod == "" {
		dod = taskTitle
	}
	t := task{
		TaskID:    taskID,
		Title:     taskTitle,
		Scope:     taskScope,
		DoD:       dod,
		Priority:  taskPriority,
		Status:    "pending",
		DependsOn: taskDepends,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Insert into database
	depsJSON, err := json.Marshal(t.DependsOn)
	if err != nil {
		return fmt.Errorf("failed to marshal depends: %w", err)
	}

	depsColumn := "dependencies"
	if ok, colErr := columnExists(db, "c4_tasks", "dependencies"); colErr == nil && !ok {
		depsColumn = "depends_on"
	}
	hasDoD, _ := columnExists(db, "c4_tasks", "dod")

	var query string
	var params []any
	if hasDoD {
		query = fmt.Sprintf(
			`INSERT INTO c4_tasks (task_id, title, scope, dod, priority, status, %s, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			depsColumn,
		)
		params = []any{t.TaskID, t.Title, t.Scope, t.DoD, t.Priority, t.Status, string(depsJSON), t.CreatedAt, t.CreatedAt}
	} else {
		query = fmt.Sprintf(
			`INSERT INTO c4_tasks (task_id, title, scope, priority, status, %s, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			depsColumn,
		)
		params = []any{t.TaskID, t.Title, t.Scope, t.Priority, t.Status, string(depsJSON), t.CreatedAt, t.CreatedAt}
	}

	_, err = db.Exec(query, params...)
	if err != nil {
		return fmt.Errorf("failed to insert task: %w", err)
	}

	fmt.Printf("Task added: %s\n", t.TaskID)
	fmt.Printf("  Title:    %s\n", t.Title)
	if t.Scope != "" {
		fmt.Printf("  Scope:    %s\n", t.Scope)
	}
	fmt.Printf("  Priority: %d\n", t.Priority)
	if len(t.DependsOn) > 0 {
		fmt.Printf("  Depends:  %s\n", strings.Join(t.DependsOn, ", "))
	}

	return nil
}

// ensureTasksTable creates the c4_tasks table if it doesn't exist.
func ensureTasksTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS c4_tasks (
		task_id      TEXT PRIMARY KEY,
		title        TEXT NOT NULL,
		scope        TEXT DEFAULT '',
		dod          TEXT DEFAULT '',
		status       TEXT DEFAULT 'pending',
		dependencies TEXT DEFAULT '[]',
		domain       TEXT DEFAULT '',
		priority     INTEGER DEFAULT 0,
		model        TEXT DEFAULT '',
		worker_id    TEXT DEFAULT '',
		branch       TEXT DEFAULT '',
		commit_sha   TEXT DEFAULT '',
		created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("failed to create tasks table: %w", err)
	}
	return nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid      int
			name     string
			typ      string
			notnull  int
			defaultV sql.NullString
			primaryK int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultV, &primaryK); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// nextTaskID generates the next task ID like T-001-0, T-002-0, etc.
func nextTaskID(db *sql.DB) (string, error) {
	var maxNum int
	err := db.QueryRow(
		`SELECT COALESCE(MAX(CAST(SUBSTR(task_id, 3, 3) AS INTEGER)), 0) FROM c4_tasks WHERE task_id LIKE 'T-%'`,
	).Scan(&maxNum)
	if err != nil {
		// Table might be empty
		maxNum = 0
	}
	return fmt.Sprintf("T-%03d-0", maxNum+1), nil
}
