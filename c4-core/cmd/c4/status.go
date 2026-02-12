package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project state",
	Long:  "Display the current C4 project status including workflow state, task counts, and worker information.",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

// projectState holds the parsed state from the database.
type projectState struct {
	Status    string `json:"status"`
	ProjectID string `json:"project_id"`
}

// taskCounts holds aggregate task statistics.
type taskCounts struct {
	Total      int
	Pending    int
	InProgress int
	Done       int
	Blocked    int
}

func runStatus(cmd *cobra.Command, args []string) error {
	db, err := openDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Load project state
	state, err := loadProjectState(db)
	if err != nil {
		return err
	}

	// Count tasks by status
	counts, err := countTasks(db)
	if err != nil {
		return err
	}

	// Print status
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Project:\t%s\n", state.ProjectID)
	fmt.Fprintf(w, "Status:\t%s\n", state.Status)
	fmt.Fprintf(w, "\nTasks:\n")
	fmt.Fprintf(w, "  Total:\t%d\n", counts.Total)
	fmt.Fprintf(w, "  Done:\t%d\n", counts.Done)
	fmt.Fprintf(w, "  In Progress:\t%d\n", counts.InProgress)
	fmt.Fprintf(w, "  Pending:\t%d\n", counts.Pending)
	fmt.Fprintf(w, "  Blocked:\t%d\n", counts.Blocked)

	if counts.Total > 0 {
		pct := float64(counts.Done) / float64(counts.Total) * 100
		fmt.Fprintf(w, "\nProgress:\t%.0f%%\n", pct)
	}

	w.Flush()
	return nil
}

// loadProjectState reads the project state from the c4_state table.
func loadProjectState(db *sql.DB) (*projectState, error) {
	var stateJSON string
	err := db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return &projectState{Status: "INIT", ProjectID: "(unknown)"}, nil
		}
		return nil, fmt.Errorf("failed to read project state: %w", err)
	}

	var state projectState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("failed to parse project state: %w", err)
	}

	return &state, nil
}

// countTasks reads task counts grouped by status from the c4_tasks table.
func countTasks(db *sql.DB) (*taskCounts, error) {
	counts := &taskCounts{}

	rows, err := db.Query("SELECT status, COUNT(*) FROM c4_tasks GROUP BY status")
	if err != nil {
		// Table might not exist yet
		return counts, nil
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan task count: %w", err)
		}
		counts.Total += count
		switch status {
		case "pending":
			counts.Pending = count
		case "in_progress":
			counts.InProgress = count
		case "done":
			counts.Done = count
		case "blocked":
			counts.Blocked = count
		}
	}

	return counts, nil
}
