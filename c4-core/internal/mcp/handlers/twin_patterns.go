package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// DetectPatterns analyzes persona_stats, c4_tasks, and c4_checkpoints for patterns.
func (s *SQLiteStore) DetectPatterns(personaID string) []Pattern {
	var patterns []Pattern

	// Pattern 1: Domain success rate variance
	patterns = append(patterns, s.detectDomainVariance(personaID)...)

	// Pattern 2: Recent vs overall approval rate change
	patterns = append(patterns, s.detectTrendShift(personaID)...)

	// Pattern 3: Repeated task type failures
	patterns = append(patterns, s.detectRepeatedFailures(personaID)...)

	// Pattern 4: Checkpoint rejection pattern
	patterns = append(patterns, s.detectCheckpointPatterns()...)

	// Pattern 5: Review feedback repeated keywords
	patterns = append(patterns, s.detectFeedbackKeywords()...)

	// Pattern 6: Task completion speed change
	patterns = append(patterns, s.detectSpeedChange()...)

	return patterns
}

// detectDomainVariance checks if success rates vary significantly across domains.
func (s *SQLiteStore) detectDomainVariance(personaID string) []Pattern {
	rows, err := s.db.Query(`
		SELECT t.domain,
			COUNT(*) as total,
			SUM(CASE WHEN ps.outcome='approved' THEN 1 ELSE 0 END) as approved
		FROM persona_stats ps
		JOIN c4_tasks t ON ps.task_id = t.task_id
		WHERE ps.persona_id = ? AND t.domain != ''
		GROUP BY t.domain
		HAVING total >= 3`,
		personaID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type domainStat struct {
		domain   string
		total    int
		approved int
		rate     float64
	}
	var stats []domainStat

	for rows.Next() {
		var d domainStat
		if err := rows.Scan(&d.domain, &d.total, &d.approved); err != nil {
			continue
		}
		d.rate = float64(d.approved) / float64(d.total) * 100
		stats = append(stats, d)
	}

	if len(stats) < 2 {
		return nil
	}

	// Find best and worst domains
	var best, worst domainStat
	best.rate = -1
	worst.rate = 101
	for _, d := range stats {
		if d.rate > best.rate {
			best = d
		}
		if d.rate < worst.rate {
			worst = d
		}
	}

	if best.rate-worst.rate >= 30 {
		return []Pattern{{
			Type:     "performance",
			Severity: "info",
			Description: fmt.Sprintf("Domain performance gap: '%s' (%.0f%%) vs '%s' (%.0f%%)",
				best.domain, best.rate, worst.domain, worst.rate),
			Evidence: []string{
				fmt.Sprintf("%s: %d/%d approved", best.domain, best.approved, best.total),
				fmt.Sprintf("%s: %d/%d approved", worst.domain, worst.approved, worst.total),
			},
			Suggestion: fmt.Sprintf("Focus on improving '%s' domain skills or review failure patterns there", worst.domain),
		}}
	}
	return nil
}

// detectTrendShift compares recent N tasks vs overall approval rate.
func (s *SQLiteStore) detectTrendShift(personaID string) []Pattern {
	// Recent 10 tasks
	var recentApproved, recentTotal int
	err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM (SELECT outcome FROM persona_stats WHERE persona_id=? ORDER BY created_at DESC LIMIT 10)`,
		personaID).Scan(&recentTotal, &recentApproved)
	if err != nil || recentTotal < 5 {
		return nil
	}

	// Overall
	var overallApproved, overallTotal int
	err = s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM persona_stats WHERE persona_id=?`,
		personaID).Scan(&overallTotal, &overallApproved)
	if err != nil || overallTotal < 10 {
		return nil
	}

	recentRate := float64(recentApproved) / float64(recentTotal) * 100
	overallRate := float64(overallApproved) / float64(overallTotal) * 100
	diff := recentRate - overallRate

	if diff >= 15 {
		return []Pattern{{
			Type:        "growth",
			Severity:    "info",
			Description: fmt.Sprintf("Approval rate improving: recent %.0f%% vs overall %.0f%%", recentRate, overallRate),
			Evidence: []string{
				fmt.Sprintf("Recent %d tasks: %d approved", recentTotal, recentApproved),
				fmt.Sprintf("Overall %d tasks: %d approved", overallTotal, overallApproved),
			},
			Suggestion: "Keep up the momentum. Consider documenting what changed.",
		}}
	} else if diff <= -15 {
		return []Pattern{{
			Type:        "performance",
			Severity:    "warning",
			Description: fmt.Sprintf("Approval rate declining: recent %.0f%% vs overall %.0f%%", recentRate, overallRate),
			Evidence: []string{
				fmt.Sprintf("Recent %d tasks: %d approved", recentTotal, recentApproved),
				fmt.Sprintf("Overall %d tasks: %d approved", overallTotal, overallApproved),
			},
			Suggestion: "Review recent rejections. Consider adding pre-submit checklists.",
		}}
	}
	return nil
}

// detectRepeatedFailures finds task types with consecutive failures.
func (s *SQLiteStore) detectRepeatedFailures(personaID string) []Pattern {
	// Look at last 10 outcomes for consecutive rejections
	rows, err := s.db.Query(`
		SELECT ps.task_id, ps.outcome
		FROM persona_stats ps
		WHERE ps.persona_id = ?
		ORDER BY ps.created_at DESC
		LIMIT 10`,
		personaID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var consecutive int
	var failedTasks []string
	for rows.Next() {
		var taskID, outcome string
		if err := rows.Scan(&taskID, &outcome); err != nil {
			continue
		}
		if outcome == "rejected" {
			consecutive++
			failedTasks = append(failedTasks, taskID)
		} else {
			break // stop counting when we hit an approved task
		}
	}

	if consecutive >= 3 {
		return []Pattern{{
			Type:        "behavioral",
			Severity:    "warning",
			Description: fmt.Sprintf("%d consecutive task rejections", consecutive),
			Evidence:    failedTasks,
			Suggestion:  "Break the pattern: review rejection feedback, add checklist items, or request guidance.",
		}}
	}
	return nil
}

// detectCheckpointPatterns analyzes checkpoint decisions.
func (s *SQLiteStore) detectCheckpointPatterns() []Pattern {
	var total, requestChanges int
	err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN decision='REQUEST_CHANGES' THEN 1 ELSE 0 END)
		FROM c4_checkpoints`).Scan(&total, &requestChanges)
	if err != nil || total < 3 {
		return nil
	}

	rate := float64(requestChanges) / float64(total) * 100
	if rate >= 50 {
		// Get recent required_changes for context
		var evidence []string
		rows, err := s.db.Query(`
			SELECT checkpoint_id, required_changes FROM c4_checkpoints
			WHERE decision='REQUEST_CHANGES'
			ORDER BY created_at DESC LIMIT 3`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cpID, changes string
				if err := rows.Scan(&cpID, &changes); err == nil {
					evidence = append(evidence, fmt.Sprintf("%s: %s", cpID, truncate(changes, 80)))
				}
			}
		}

		return []Pattern{{
			Type:        "behavioral",
			Severity:    "challenge",
			Description: fmt.Sprintf("High checkpoint rejection rate: %.0f%% (%d/%d)", rate, requestChanges, total),
			Evidence:    evidence,
			Suggestion:  "Review the repeated change requests. Consider adding self-review before checkpoint.",
		}}
	}
	return nil
}

// detectFeedbackKeywords extracts repeated keywords from checkpoint notes.
func (s *SQLiteStore) detectFeedbackKeywords() []Pattern {
	rows, err := s.db.Query(`
		SELECT notes FROM c4_checkpoints
		WHERE decision='REQUEST_CHANGES' AND notes != ''
		ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// Count keyword frequency
	keywords := map[string]int{}
	targetWords := []string{
		"test", "error handling", "validation", "documentation",
		"rollback", "security", "performance", "type", "coverage",
	}

	var noteCount int
	for rows.Next() {
		var notes string
		if err := rows.Scan(&notes); err != nil {
			continue
		}
		noteCount++
		lower := strings.ToLower(notes)
		for _, kw := range targetWords {
			if strings.Contains(lower, kw) {
				keywords[kw]++
			}
		}
	}

	if noteCount < 3 {
		return nil
	}

	// Report keywords appearing in >40% of rejection notes
	var patterns []Pattern
	threshold := float64(noteCount) * 0.4
	for kw, count := range keywords {
		if float64(count) >= threshold {
			patterns = append(patterns, Pattern{
				Type:        "behavioral",
				Severity:    "challenge",
				Description: fmt.Sprintf("Recurring review feedback theme: '%s' (appeared in %d/%d rejections)", kw, count, noteCount),
				Evidence:    []string{fmt.Sprintf("%d occurrences across %d rejection notes", count, noteCount)},
				Suggestion:  fmt.Sprintf("Add '%s' to your pre-submit checklist", kw),
			})
		}
	}
	return patterns
}

// detectSpeedChange analyzes task completion speed trends.
func (s *SQLiteStore) detectSpeedChange() []Pattern {
	// Compare average days to complete: recent 5 vs overall
	var recentAvg, overallAvg sql.NullFloat64
	if err := s.db.QueryRow(`
		SELECT AVG(julianday(updated_at) - julianday(created_at))
		FROM (SELECT created_at, updated_at FROM c4_tasks WHERE status='done' ORDER BY updated_at DESC LIMIT 5)
	`).Scan(&recentAvg); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: detectSpeedChange recent avg: %v\n", err)
	}

	if err := s.db.QueryRow(`
		SELECT AVG(julianday(updated_at) - julianday(created_at))
		FROM c4_tasks WHERE status='done'
	`).Scan(&overallAvg); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: detectSpeedChange overall avg: %v\n", err)
	}

	if !recentAvg.Valid || !overallAvg.Valid {
		return nil
	}

	// Skip when durations are under 1 hour (0.042 days) — too noisy for meaningful comparison
	const minMeaningfulDays = 0.042
	if overallAvg.Float64 < minMeaningfulDays {
		return nil
	}

	ratio := recentAvg.Float64 / overallAvg.Float64
	if ratio <= 0.6 {
		return []Pattern{{
			Type:        "growth",
			Severity:    "info",
			Description: fmt.Sprintf("Task completion speeding up: recent avg %.1f days vs overall %.1f days", recentAvg.Float64, overallAvg.Float64),
			Evidence:    []string{fmt.Sprintf("Speed ratio: %.0f%% of baseline", ratio*100)},
			Suggestion:  "Faster execution detected. Ensure quality isn't being sacrificed for speed.",
		}}
	} else if ratio >= 1.5 {
		return []Pattern{{
			Type:        "performance",
			Severity:    "warning",
			Description: fmt.Sprintf("Task completion slowing down: recent avg %.1f days vs overall %.1f days", recentAvg.Float64, overallAvg.Float64),
			Evidence:    []string{fmt.Sprintf("Speed ratio: %.0f%% of baseline", ratio*100)},
			Suggestion:  "Consider breaking tasks into smaller pieces or identifying blockers.",
		}}
	}
	return nil
}
