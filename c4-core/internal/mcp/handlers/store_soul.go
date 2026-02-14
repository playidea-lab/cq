package handlers

import (
	"fmt"
	"os"
	"time"
)

// injectSoulContext reads team.yaml to find the active user, then resolves
// their soul for the task's domain. This is best-effort: failures are logged, not fatal.
func (s *SQLiteStore) injectSoulContext(a *TaskAssignment) {
	// Reuse proper YAML parsing from persona.go
	username := getActiveUsername(s.projectRoot)
	if username == "" {
		return
	}

	// Get active persona for the user
	activePersona := getActivePersonaForUser(s.projectRoot, username)

	// Determine role: use task domain if available, otherwise active persona
	role := a.Domain
	if role == "" {
		role = activePersona
	}
	if role == "" {
		return
	}

	// Resolve soul (best-effort)
	result, err := ResolveSoul(s.projectRoot, username, role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: injectSoulContext failed for %s/%s: %v\n", username, role, err)
		return
	}

	merged, _ := result["merged"].(string)
	if merged != "" {
		a.SoulContext = merged
	}

	// Also inject project soul if available (3-way merge: role + personal + project)
	if projectRoleForStage != "" && projectRoleForStage != role {
		projResult, projErr := ResolveSoul(s.projectRoot, username, projectRoleForStage)
		if projErr == nil {
			if projMerged, ok := projResult["merged"].(string); ok && projMerged != "" {
				a.SoulContext += "\n\n---\n## Project Context\n" + projMerged
			}
		}
	}
}

// recordGrowthOnCompletion records a growth snapshot after task completion (best-effort).
func (s *SQLiteStore) recordGrowthOnCompletion() {
	if s.projectRoot == "" {
		return
	}
	go func() {
		username := getActiveUsername(s.projectRoot)
		if username != "" {
			s.RecordGrowthSnapshot(username)
		}
	}()
}

// autoLearn analyzes persona patterns and updates the soul's Learned section.
// Best-effort: runs in a goroutine, failures are logged not fatal.
func (s *SQLiteStore) autoLearn(personaID string) {
	if s.projectRoot == "" {
		return
	}

	go func() {
		stats, err := s.GetPersonaStats(personaID)
		if err != nil {
			return
		}

		total, _ := stats["total_tasks"].(int)
		if total < 5 {
			return // need minimum data to generate meaningful suggestions
		}

		suggestions := analyzePatternsForSuggestions(stats, total)
		if len(suggestions) == 0 {
			return
		}

		username := getActiveUsername(s.projectRoot)
		if username == "" {
			return
		}

		if err := applySuggestionsToSoul(s.projectRoot, username, personaID, suggestions); err != nil {
			fmt.Fprintf(os.Stderr, "c4: autoLearn failed for %s/%s: %v\n", username, personaID, err)
		}
	}()
}

// recordPersonaStat records a persona outcome for a task (best-effort).
func (s *SQLiteStore) recordPersonaStat(personaID, taskID, outcome string) {
	if personaID == "" {
		personaID = "direct"
	}
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO persona_stats (persona_id, task_id, outcome, created_at)
		VALUES (?, ?, ?, ?)`,
		personaID, taskID, outcome, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		fmt.Fprintf(os.Stderr, "c4: recordPersonaStat %s/%s: %v\n", personaID, taskID, err)
	}
}
