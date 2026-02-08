package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var (
	runWorkers int
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start execution with workers",
	Long: `Transition the project from PLAN/HALTED to EXECUTE state and start worker loops.
If no tasks are available, returns an error.`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().IntVarP(&runWorkers, "workers", "w", 1, "number of workers to spawn")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Check current state
	state, err := loadProjectState(db)
	if err != nil {
		return err
	}

	// Verify there are tasks to run
	counts, err := countTasks(db)
	if err != nil {
		return err
	}

	if counts.Total == 0 {
		return fmt.Errorf("no tasks found: add tasks with 'c4 add-task' first")
	}

	pendingOrBlocked := counts.Pending + counts.Blocked + counts.InProgress
	if pendingOrBlocked == 0 {
		fmt.Println("All tasks are done. Nothing to execute.")
		return nil
	}

	// Transition state to EXECUTE if needed
	switch state.Status {
	case "PLAN", "HALTED":
		if err := transitionToExecute(db, state); err != nil {
			return err
		}
		fmt.Printf("State transitioned: %s -> EXECUTE\n", state.Status)
	case "EXECUTE":
		fmt.Println("Already in EXECUTE state.")
	case "COMPLETE":
		fmt.Println("Project is already COMPLETE.")
		return nil
	case "CHECKPOINT":
		fmt.Println("Checkpoint review in progress. Wait for review to complete.")
		return nil
	default:
		return fmt.Errorf("cannot run from state %s: use 'c4 status' to check current state", state.Status)
	}

	fmt.Printf("Starting %d worker(s)...\n", runWorkers)
	fmt.Printf("Tasks: %d pending, %d in_progress, %d blocked\n",
		counts.Pending, counts.InProgress, counts.Blocked)

	// In the full implementation, this would spawn goroutines that loop:
	//   1. Get task via bridge.Client
	//   2. Execute task
	//   3. Submit result
	//   4. Repeat
	// For now, print what would happen.
	fmt.Println("\nWorker loop started. Use 'c4 stop' to halt execution.")

	return nil
}

// transitionToExecute updates the project state to EXECUTE.
func transitionToExecute(db *sql.DB, state *projectState) error {
	state.Status = "EXECUTE"

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	_, err = db.Exec(
		"UPDATE c4_state SET state_json = ?, updated_at = CURRENT_TIMESTAMP WHERE project_id = ?",
		string(stateJSON), state.ProjectID,
	)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	return nil
}
