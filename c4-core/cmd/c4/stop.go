package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop execution and halt workers",
	Long: `Transition the project from EXECUTE to HALTED state.
Running workers will finish their current task and then stop.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
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

	switch state.Status {
	case "EXECUTE":
		if err := transitionToHalted(db, state); err != nil {
			return err
		}
		fmt.Println("State transitioned: EXECUTE -> HALTED")
		fmt.Println("Workers will stop after completing their current task.")
	case "HALTED":
		fmt.Println("Already in HALTED state.")
	case "PLAN":
		fmt.Println("Not running. Currently in PLAN state.")
	case "COMPLETE":
		fmt.Println("Project is already COMPLETE.")
	case "CHECKPOINT":
		fmt.Println("Checkpoint review in progress. Cannot stop during checkpoint.")
	default:
		return fmt.Errorf("cannot stop from state %s", state.Status)
	}

	return nil
}

// transitionToHalted updates the project state to HALTED.
func transitionToHalted(db *sql.DB, state *projectState) error {
	state.Status = "HALTED"

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
