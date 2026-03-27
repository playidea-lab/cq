package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/persona"
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

// autoLearnFromDiff extracts coding patterns from a git diff and appends
// them to raw_patterns.json. Best-effort: runs in goroutine, failures logged.
// Called automatically after successful worktree merge in SubmitTask.
func (s *SQLiteStore) autoLearnFromDiff(commitRange string) {
	if s.projectRoot == "" || commitRange == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", commitRange)
		cmd.Dir = s.projectRoot
		out, err := cmd.Output()
		if err != nil {
			return
		}

		files := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(files) == 0 || (len(files) == 1 && files[0] == "") {
			return
		}

		parts := strings.SplitN(commitRange, "..", 2)
		if len(parts) != 2 {
			return
		}
		beforeRef, afterRef := parts[0], parts[1]

		var allPatterns []persona.EditPattern
		for _, file := range files {
			if file == "" || !isCodeFile(file) {
				continue
			}

			before, _ := exec.CommandContext(ctx, "git", "-C", s.projectRoot, "show", beforeRef+":"+file).Output()
			after, _ := exec.CommandContext(ctx, "git", "-C", s.projectRoot, "show", afterRef+":"+file).Output()
			if len(before) == 0 && len(after) == 0 {
				continue
			}

			patterns := persona.AnalyzeEdits(string(before), string(after))
			for i := range patterns {
				patterns[i].Description = fmt.Sprintf("[%s] %s", file, patterns[i].Description)
			}
			allPatterns = append(allPatterns, patterns...)
		}

		if len(allPatterns) == 0 {
			return
		}

		username := getActiveUsername(s.projectRoot)
		if username == "" {
			username = os.Getenv("USER")
		}
		if username == "" {
			username = "default"
		}

		patternsPath := filepath.Join(s.projectRoot, ".c4", "souls", username, "raw_patterns.json")
		_ = os.MkdirAll(filepath.Dir(patternsPath), 0755)

		var existing []persona.EditPattern
		if data, err := os.ReadFile(patternsPath); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &existing)
		}
		existing = append(existing, allPatterns...)

		data, _ := json.MarshalIndent(existing, "", "  ")
		if err := os.WriteFile(patternsPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "c4: autoLearnFromDiff: write failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "c4: autoLearnFromDiff: %d patterns from %s\n", len(allPatterns), commitRange)
		}
	}()
}

// recordPersonaStat records a persona outcome for a task (best-effort).
// reviewScore is optional — pass 0 if not available.
func (s *SQLiteStore) recordPersonaStat(personaID, taskID, outcome string, reviewScore ...float64) {
	if personaID == "" {
		personaID = "direct"
	}
	score := 0.0
	if len(reviewScore) > 0 {
		score = reviewScore[0]
	}
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO persona_stats (persona_id, task_id, outcome, review_score, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		personaID, taskID, outcome, score, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		fmt.Fprintf(os.Stderr, "c4: recordPersonaStat %s/%s: %v\n", personaID, taskID, err)
	}
}
