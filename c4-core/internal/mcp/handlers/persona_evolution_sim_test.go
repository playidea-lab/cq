package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// recordPersonaStatAt records a persona stat with an explicit timestamp for deterministic ordering.
func (s *SQLiteStore) recordPersonaStatAt(personaID, taskID, outcome string, ts time.Time) {
	if personaID == "" {
		personaID = "direct"
	}
	_, _ = s.db.Exec(`
		INSERT OR REPLACE INTO persona_stats (persona_id, task_id, outcome, created_at)
		VALUES (?, ?, ?, ?)`,
		personaID, taskID, outcome, ts.UTC().Format(time.RFC3339),
	)
}

// TestPersonaEvolutionSimulation is a full end-to-end simulation of the
// persona evolution feedback loop: record → analyze → learn → inject.
func TestPersonaEvolutionSimulation(t *testing.T) {
	// --- Setup: in-memory store + temp projectRoot for soul files ---
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(db, WithProjectRoot(tmpDir))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	// Create team.yaml with test user
	teamDir := filepath.Join(tmpDir, ".c4")
	os.MkdirAll(teamDir, 0755)
	os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(`members:
  testuser:
    role: developer
    roles: [developer]
`), 0644)

	// Base time for deterministic ordering
	baseTime := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)

	// ======================================================
	// Phase 1: Simulate 11 tasks — 7 approved, then 4 rejected
	// 36% rejection rate > 30% threshold, total > 10 for specialization
	// Rejected tasks are LAST (most recent) for consecutive detection
	// ======================================================
	t.Run("Phase1_RecordMixedOutcomes", func(t *testing.T) {
		for i := 1; i <= 11; i++ {
			taskID := fmt.Sprintf("T-%03d-0", i)
			if err := store.AddTask(&Task{
				ID: taskID, Title: fmt.Sprintf("Task %d", i),
				DoD: "done", Status: "pending", Domain: "backend",
			}); err != nil {
				t.Fatalf("add task %s: %v", taskID, err)
			}
			db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-sim' WHERE task_id=?", taskID)

			ts := baseTime.Add(time.Duration(i) * time.Minute) // each task 1 min apart
			if i <= 7 {
				store.recordPersonaStatAt("worker-sim", taskID, "approved", ts)
			} else {
				store.recordPersonaStatAt("worker-sim", taskID, "rejected", ts)
			}
		}

		stats, err := store.GetPersonaStats("worker-sim")
		if err != nil {
			t.Fatalf("get stats: %v", err)
		}

		total := stats["total_tasks"].(int)
		if total != 11 {
			t.Fatalf("total_tasks = %d, want 11", total)
		}

		outcomes := stats["outcomes"].(map[string]int)
		if outcomes["approved"] != 7 {
			t.Fatalf("approved = %d, want 7", outcomes["approved"])
		}
		if outcomes["rejected"] != 4 {
			t.Fatalf("rejected = %d, want 4", outcomes["rejected"])
		}
		t.Logf("Stats: total=%d, approved=%d, rejected=%d",
			total, outcomes["approved"], outcomes["rejected"])
	})

	// ======================================================
	// Phase 2: Verify analyzePatternsForSuggestions
	// 36% rejection > 30% → rejection warning
	// 11 tasks > 10 → specialization suggestion
	// ======================================================
	t.Run("Phase2_AnalyzePatterns", func(t *testing.T) {
		stats, _ := store.GetPersonaStats("worker-sim")
		total := stats["total_tasks"].(int)
		suggestions := analyzePatternsForSuggestions(stats, total)

		if len(suggestions) == 0 {
			t.Fatal("expected suggestions, got none")
		}

		foundRejection := false
		foundSpecialize := false
		for _, s := range suggestions {
			if strings.Contains(s, "rejection rate") {
				foundRejection = true
			}
			if strings.Contains(s, "specializing") {
				foundSpecialize = true
			}
		}

		if !foundRejection {
			t.Error("expected rejection rate suggestion")
		}
		if !foundSpecialize {
			t.Error("expected specialization suggestion (total > 10)")
		}

		t.Logf("Suggestions (%d):", len(suggestions))
		for _, s := range suggestions {
			t.Logf("  → %s", s)
		}
	})

	// ======================================================
	// Phase 3: Verify applySuggestionsToSoul writes to file
	// ======================================================
	t.Run("Phase3_ApplyToSoul", func(t *testing.T) {
		stats, _ := store.GetPersonaStats("worker-sim")
		total := stats["total_tasks"].(int)
		suggestions := analyzePatternsForSuggestions(stats, total)

		err := applySuggestionsToSoul(tmpDir, "testuser", "worker-sim", suggestions)
		if err != nil {
			t.Fatalf("apply suggestions: %v", err)
		}

		soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-worker-sim.md")
		data, err := os.ReadFile(soulPath)
		if err != nil {
			t.Fatalf("read soul: %v", err)
		}

		content := string(data)
		if !strings.Contains(content, "## Learned") {
			t.Error("soul file missing ## Learned section")
		}
		if !strings.Contains(content, "rejection rate") {
			t.Error("soul file missing rejection rate suggestion")
		}

		t.Logf("Soul file written at: %s", soulPath)
		t.Logf("Content preview:\n%s", truncateForLog(content, 500))
	})

	// ======================================================
	// Phase 4: Deduplication — same suggestions should not duplicate
	// ======================================================
	t.Run("Phase4_Deduplication", func(t *testing.T) {
		stats, _ := store.GetPersonaStats("worker-sim")
		total := stats["total_tasks"].(int)
		suggestions := analyzePatternsForSuggestions(stats, total)

		err := applySuggestionsToSoul(tmpDir, "testuser", "worker-sim", suggestions)
		if err != nil {
			t.Fatalf("apply suggestions (2nd): %v", err)
		}

		soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-worker-sim.md")
		data, _ := os.ReadFile(soulPath)
		content := string(data)

		count := strings.Count(content, "rejection rate")
		if count != 1 {
			t.Fatalf("expected 1 occurrence of 'rejection rate', got %d", count)
		}
		t.Log("Deduplication OK — no duplicate entries")
	})

	// ======================================================
	// Phase 5: Digital Twin pattern detection
	// Tasks 8-11 are consecutive rejected (most recent) → detect
	// ======================================================
	t.Run("Phase5_TwinPatterns", func(t *testing.T) {
		patterns := store.DetectPatterns("worker-sim")
		t.Logf("Detected %d patterns:", len(patterns))
		for _, p := range patterns {
			t.Logf("  [%s/%s] %s", p.Type, p.Severity, p.Description)
			if p.Suggestion != "" {
				t.Logf("    → %s", p.Suggestion)
			}
		}

		foundRepeated := false
		for _, p := range patterns {
			if strings.Contains(p.Description, "consecutive") {
				foundRepeated = true
			}
		}
		if !foundRepeated {
			t.Error("expected repeated failures pattern (4 consecutive rejected at end)")
		}
	})

	// ======================================================
	// Phase 6: PersonaDigest in GetStatus
	// ======================================================
	t.Run("Phase6_PersonaDigest", func(t *testing.T) {
		status, err := store.GetStatus()
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		if status.PersonaDigest == nil {
			t.Fatal("expected PersonaDigest, got nil")
		}
		if status.PersonaDigest.TotalTasks != 11 {
			t.Fatalf("total = %d, want 11", status.PersonaDigest.TotalTasks)
		}
		// 7/11 approved ≈ 0.636
		rate := status.PersonaDigest.ApprovalRate
		if rate < 0.63 || rate > 0.65 {
			t.Fatalf("approval_rate = %.3f, want ~0.636", rate)
		}
		t.Logf("PersonaDigest: total=%d, approval_rate=%.1f%%",
			status.PersonaDigest.TotalTasks, rate*100)
	})

	// ======================================================
	// Phase 7: Growth snapshot recording
	// ======================================================
	t.Run("Phase7_GrowthSnapshot", func(t *testing.T) {
		store.RecordGrowthSnapshot("testuser")

		trend := store.GetGrowthTrend("testuser")
		if ar, ok := trend["approval_rate"]; ok {
			t.Logf("Growth: approval_rate=%.2f", ar.Current)
			if ar.Current < 0.63 || ar.Current > 0.65 {
				t.Errorf("growth approval_rate = %.3f, want ~0.636", ar.Current)
			}
		} else {
			t.Error("expected approval_rate in growth trend")
		}
	})

	// ======================================================
	// Phase 8: BuildTwinContext for task assignment
	// ======================================================
	t.Run("Phase8_BuildTwinContext", func(t *testing.T) {
		task := &Task{ID: "T-NEW-0", Title: "New task", Domain: "backend"}
		ctx := store.BuildTwinContext(task)
		if ctx == nil {
			t.Log("TwinContext is nil (no soul reminder + domain-filtered patterns empty)")
			return
		}
		t.Logf("TwinContext: patterns=%d, soul_reminder=%q",
			len(ctx.Patterns), truncateForLog(ctx.SoulReminder, 80))
	})

	// ======================================================
	// Phase 9: Full autoLearn pipeline (integration)
	// ======================================================
	t.Run("Phase9_AutoLearnIntegration", func(t *testing.T) {
		stats, _ := store.GetPersonaStats("worker-sim")
		total := stats["total_tasks"].(int)
		if total < 5 {
			t.Skip("need minimum 5 tasks")
		}

		suggestions := analyzePatternsForSuggestions(stats, total)
		if len(suggestions) == 0 {
			t.Skip("no suggestions to apply")
		}

		username := getActiveUsername(tmpDir)
		if username != "testuser" {
			t.Fatalf("username = %q, want testuser", username)
		}

		err := applySuggestionsToSoul(tmpDir, username, "worker-sim", suggestions)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}

		soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-worker-sim.md")
		data, _ := os.ReadFile(soulPath)
		sections := parseSoulSections(string(data))

		learned := sections["Learned"]
		if learned == "" {
			t.Fatal("Learned section is empty")
		}

		today := time.Now().Format("2006-01-02")
		if !strings.Contains(learned, today) {
			t.Errorf("Learned section missing today's date %s", today)
		}

		t.Log("Full pipeline: record→analyze→apply→soul OK")
		t.Logf("  Learned:\n%s", learned)
	})

	// ======================================================
	// Phase 10: Trend shift detection
	// Add 10 more approved tasks with later timestamps
	// Recent 10 = all approved (100%), overall = 17/21 ≈ 81%
	// Diff = +19% ≥ +15% → should detect improving trend
	// ======================================================
	t.Run("Phase10_TrendShift", func(t *testing.T) {
		laterTime := baseTime.Add(1 * time.Hour) // clearly after all previous tasks
		for i := 12; i <= 21; i++ {
			taskID := fmt.Sprintf("T-%03d-0", i)
			store.AddTask(&Task{ID: taskID, Title: fmt.Sprintf("Task %d", i), DoD: "done", Status: "pending"})
			ts := laterTime.Add(time.Duration(i) * time.Minute)
			store.recordPersonaStatAt("worker-sim", taskID, "approved", ts)
		}

		// Now: 17 approved, 4 rejected out of 21
		// Recent 10 (ORDER BY created_at DESC): T-021 through T-012, all approved → 100%
		// Overall: 17/21 ≈ 80.9%
		// Diff = 100% - 80.9% = +19.1% ≥ +15% → improving
		patterns := store.detectTrendShift("worker-sim")
		t.Logf("Trend shift patterns: %d", len(patterns))
		for _, p := range patterns {
			t.Logf("  [%s] %s", p.Severity, p.Description)
		}

		foundImproving := false
		for _, p := range patterns {
			if strings.Contains(p.Description, "improving") {
				foundImproving = true
			}
		}
		if !foundImproving {
			t.Error("expected improving trend (recent 100% vs overall ~81%)")
		}
	})

	// ======================================================
	// Final summary
	// ======================================================
	t.Run("Summary", func(t *testing.T) {
		stats, _ := store.GetPersonaStats("worker-sim")
		total := stats["total_tasks"].(int)
		outcomes := stats["outcomes"].(map[string]int)

		status, _ := store.GetStatus()
		patterns := store.DetectPatterns("worker-sim")
		trend := store.GetGrowthTrend("testuser")

		t.Log("=== Persona Evolution Simulation Summary ===")
		t.Logf("Total tasks: %d (approved=%d, rejected=%d)",
			total, outcomes["approved"], outcomes["rejected"])
		t.Logf("PersonaDigest: total=%d, approval_rate=%.1f%%",
			status.PersonaDigest.TotalTasks, status.PersonaDigest.ApprovalRate*100)
		t.Logf("Twin patterns: %d detected", len(patterns))
		for k, v := range trend {
			t.Logf("Growth[%s]: current=%.2f, trend=%s", k, v.Current, v.Trend)
		}

		soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-worker-sim.md")
		data, _ := os.ReadFile(soulPath)
		sections := parseSoulSections(string(data))
		t.Logf("Soul Learned: %d lines", len(strings.Split(sections["Learned"], "\n")))
		t.Log("=== All systems operational ===")
	})
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
