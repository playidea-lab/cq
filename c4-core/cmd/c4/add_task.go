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
	addTaskCmd.Flags().StringVarP(&taskScope, "scope", "s", "", "file/directory scope")
	addTaskCmd.Flags().IntVarP(&taskPriority, "priority", "p", 5, "priority (1=highest, 10=lowest)")
	addTaskCmd.Flags().StringSliceVarP(&taskDepends, "depends", "d", nil, "task IDs this depends on")
	_ = addTaskCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addTaskCmd)
}

// task represents a row in c4_tasks.
type task struct {
	TaskID     string   `json:"task_id"`
	Title      string   `json:"title"`
	Scope      string   `json:"scope,omitempty"`
	Priority   int      `json:"priority"`
	Status     string   `json:"status"`
	DependsOn  []string `json:"depends_on,omitempty"`
	CreatedAt  string   `json:"created_at"`
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
	t := task{
		TaskID:    taskID,
		Title:     taskTitle,
		Scope:     taskScope,
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

	_, err = db.Exec(
		`INSERT INTO c4_tasks (task_id, title, scope, priority, status, depends_on, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.TaskID, t.Title, t.Scope, t.Priority, t.Status, string(depsJSON), t.CreatedAt,
	)
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
		task_id    TEXT PRIMARY KEY,
		title      TEXT NOT NULL,
		scope      TEXT DEFAULT '',
		priority   INTEGER DEFAULT 5,
		status     TEXT DEFAULT 'pending',
		depends_on TEXT DEFAULT '[]',
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("failed to create tasks table: %w", err)
	}
	return nil
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
