package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// Pattern represents a detected behavioral or performance pattern.
type Pattern struct {
	Type        string   `json:"type"`        // "behavioral", "performance", "growth", "misalignment"
	Severity    string   `json:"severity"`    // "info", "warning", "challenge"
	Description string   `json:"description"` // human-readable
	Evidence    []string `json:"evidence"`    // task IDs, dates, numbers
	Suggestion  string   `json:"suggestion"`  // actionable suggestion
}

// GrowthMetric represents a time-series growth metric.
type GrowthMetric struct {
	Current       float64 `json:"current"`
	PreviousMonth float64 `json:"previous_month,omitempty"`
	Trend         string  `json:"trend,omitempty"` // "improving", "declining", "stable"
}

// TwinContext is enrichment data injected into claim responses.
type TwinContext struct {
	Patterns     []Pattern `json:"patterns,omitempty"`
	SoulReminder string    `json:"soul_reminder,omitempty"`
}

// TwinReview is enrichment data injected into checkpoint responses.
type TwinReview struct {
	HistoricalPattern string `json:"historical_pattern,omitempty"`
	GrowthNote        string `json:"growth_note,omitempty"`
}

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
		log.Printf("c4: twin: detectSpeedChange recent avg: %v", err)
	}

	if err := s.db.QueryRow(`
		SELECT AVG(julianday(updated_at) - julianday(created_at))
		FROM c4_tasks WHERE status='done'
	`).Scan(&overallAvg); err != nil {
		log.Printf("c4: twin: detectSpeedChange overall avg: %v", err)
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

// RecordGrowthSnapshot records a weekly metric snapshot in twin_growth.
func (s *SQLiteStore) RecordGrowthSnapshot(username string) {
	now := time.Now()
	period := fmt.Sprintf("%d-W%02d", now.Year(), isoWeek(now))

	// Check if already recorded this period
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM twin_growth WHERE username=? AND period=?", username, period).Scan(&exists); err != nil {
		log.Printf("c4: twin: RecordGrowthSnapshot exists check: %v", err)
		return
	}
	if exists > 0 {
		return
	}

	// Calculate current approval rate
	var total, approved int
	if err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM persona_stats`).Scan(&total, &approved); err != nil {
		log.Printf("c4: twin: RecordGrowthSnapshot approval rate: %v", err)
	}

	if total > 0 {
		rate := float64(approved) / float64(total)
		s.insertGrowthMetric(username, "approval_rate", rate, period)
	}

	// Average review score
	var avgScore sql.NullFloat64
	if err := s.db.QueryRow("SELECT AVG(review_score) FROM persona_stats WHERE review_score > 0").Scan(&avgScore); err != nil {
		log.Printf("c4: twin: RecordGrowthSnapshot avg score: %v", err)
	}
	if avgScore.Valid {
		s.insertGrowthMetric(username, "avg_review_score", avgScore.Float64, period)
	}

	// Tasks completed this period
	var completed int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status='done'").Scan(&completed); err != nil {
		log.Printf("c4: twin: RecordGrowthSnapshot tasks completed: %v", err)
	}
	s.insertGrowthMetric(username, "tasks_completed", float64(completed), period)
}

func (s *SQLiteStore) insertGrowthMetric(username, metric string, value float64, period string) {
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO twin_growth (username, metric, value, period)
		VALUES (?, ?, ?, ?)`, username, metric, value, period); err != nil {
		log.Printf("c4: twin: insertGrowthMetric %s: %v", metric, err)
	}
}

// GetGrowthTrend retrieves recent growth metrics for a user.
func (s *SQLiteStore) GetGrowthTrend(username string) map[string]*GrowthMetric {
	metrics := map[string]*GrowthMetric{}

	rows, err := s.db.Query(`
		SELECT metric, value, period FROM twin_growth
		WHERE username = ?
		ORDER BY period DESC
		LIMIT 24`, username)
	if err != nil {
		return metrics
	}
	defer rows.Close()

	// Collect values per metric
	type mv struct {
		value  float64
		period string
	}
	byMetric := map[string][]mv{}

	for rows.Next() {
		var metric, period string
		var value float64
		if err := rows.Scan(&metric, &value, &period); err != nil {
			continue
		}
		byMetric[metric] = append(byMetric[metric], mv{value, period})
	}

	for name, values := range byMetric {
		if len(values) == 0 {
			continue
		}
		gm := &GrowthMetric{Current: values[0].value}
		if len(values) >= 5 {
			gm.PreviousMonth = values[4].value
			if gm.Current > gm.PreviousMonth+0.05 {
				gm.Trend = "improving"
			} else if gm.Current < gm.PreviousMonth-0.05 {
				gm.Trend = "declining"
			} else {
				gm.Trend = "stable"
			}
		}
		metrics[name] = gm
	}

	return metrics
}

// BuildTwinContext creates enrichment data for claim responses (best-effort).
func (s *SQLiteStore) BuildTwinContext(task *Task) *TwinContext {
	ctx := &TwinContext{}

	// Get relevant patterns for the task's domain
	patterns := s.DetectPatterns("direct")
	if task.Domain != "" {
		var relevant []Pattern
		for _, p := range patterns {
			// Include domain-related and general patterns
			if strings.Contains(p.Description, task.Domain) ||
				p.Type == "growth" || p.Severity == "challenge" {
				relevant = append(relevant, p)
			}
		}
		if len(relevant) > 0 {
			ctx.Patterns = relevant
		} else if len(patterns) > 0 && len(patterns) <= 3 {
			ctx.Patterns = patterns
		}
	} else if len(patterns) > 0 {
		// Limit to top 3 most important
		max := 3
		if len(patterns) < max {
			max = len(patterns)
		}
		ctx.Patterns = patterns[:max]
	}

	// Soul reminder
	if s.projectRoot != "" {
		username := getActiveUsername(s.projectRoot)
		if username != "" {
			role := task.Domain
			if role == "" {
				role = "developer"
			}
			result, err := ResolveSoul(s.projectRoot, username, role)
			if err == nil {
				if merged, ok := result["merged"].(string); ok && merged != "" {
					// Extract first principle line
					for _, line := range strings.Split(merged, "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "- ") && !strings.Contains(line, "여기에") {
							ctx.SoulReminder = line
							break
						}
					}
				}
			}
		}
	}

	if len(ctx.Patterns) == 0 && ctx.SoulReminder == "" {
		return nil
	}
	return ctx
}

// BuildTwinReview creates enrichment data for checkpoint responses (best-effort).
func (s *SQLiteStore) BuildTwinReview() *TwinReview {
	review := &TwinReview{}

	// Historical checkpoint pattern
	var total, approved, rejected int
	if err := s.db.QueryRow(`
		SELECT COUNT(*),
			SUM(CASE WHEN decision='APPROVE' THEN 1 ELSE 0 END),
			SUM(CASE WHEN decision='REQUEST_CHANGES' THEN 1 ELSE 0 END)
		FROM c4_checkpoints`).Scan(&total, &approved, &rejected); err != nil {
		log.Printf("c4: twin: BuildTwinReview checkpoint stats: %v", err)
	}

	if total >= 3 {
		review.HistoricalPattern = fmt.Sprintf("Checkpoint history: %d/%d approved (%.0f%%)",
			approved, total, float64(approved)/float64(total)*100)
	}

	// Growth note
	if s.projectRoot != "" {
		username := getActiveUsername(s.projectRoot)
		if username != "" {
			growth := s.GetGrowthTrend(username)
			if ar, ok := growth["approval_rate"]; ok && ar.Trend != "" {
				review.GrowthNote = fmt.Sprintf("Approval rate trend: %s (current %.0f%%)", ar.Trend, ar.Current*100)
			}
		}
	}

	if review.HistoricalPattern == "" && review.GrowthNote == "" {
		return nil
	}
	return review
}

// RegisterTwinHandlers registers the c4_reflect MCP tool.
func RegisterTwinHandlers(reg *mcp.Registry, store *SQLiteStore) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_reflect",
		Description: "Digital Twin reflection — patterns, growth, challenges, permission audit",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"focus": map[string]any{
					"type":        "string",
					"description": "Focus area: 'patterns', 'growth', 'challenges', or 'all' (default: 'all')",
					"enum":        []string{"patterns", "growth", "challenges", "all"},
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Focus string `json:"focus"`
		}
		if len(rawArgs) > 0 {
			_ = json.Unmarshal(rawArgs, &args)
		}
		if args.Focus == "" {
			args.Focus = "all"
		}

		return store.reflect(args.Focus)
	})
}

// resolveUsername tries getActiveUsername, then git config, then "unknown".
func resolveUsername(projectRoot string) string {
	if projectRoot != "" {
		if u := getActiveUsername(projectRoot); u != "" {
			return u
		}
	}
	// Fallback: read git user.name from project
	if projectRoot != "" {
		gitCfg := projectRoot + "/.git/config"
		data, err := readFileBytes(gitCfg)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name") || strings.HasPrefix(line, "name =") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return "default"
}

// readFileBytes reads a file's contents (used for git config fallback).
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// reflect gathers all Twin data for the c4_reflect tool.
func (s *SQLiteStore) reflect(focus string) (map[string]any, error) {
	result := map[string]any{}

	// Identity
	username := resolveUsername(s.projectRoot)

	// Auto-record growth snapshot on reflect (best-effort)
	go s.RecordGrowthSnapshot(username)

	var totalTasks int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks").Scan(&totalTasks); err != nil {
		log.Printf("c4: twin: reflect totalTasks: %v", err)
	}

	var doneTasks int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status='done'").Scan(&doneTasks); err != nil {
		log.Printf("c4: twin: reflect doneTasks: %v", err)
	}

	identity := map[string]any{
		"username":    username,
		"project":     s.projectID,
		"total_tasks": totalTasks,
		"done_tasks":  doneTasks,
	}

	// Active roles
	if s.projectRoot != "" {
		roles := listUserSoulFiles(s.projectRoot, username)
		if len(roles) > 0 {
			identity["active_roles"] = roles
		}
	}
	result["identity"] = identity

	// Patterns
	if focus == "all" || focus == "patterns" {
		patterns := s.DetectPatterns("direct")
		if patterns == nil {
			patterns = []Pattern{}
		}
		result["patterns"] = patterns
	}

	// Growth
	if focus == "all" || focus == "growth" {
		growth := map[string]any{}
		if username != "" {
			trend := s.GetGrowthTrend(username)
			for k, v := range trend {
				growth[k] = v
			}
		}

		// Milestones
		milestones := s.detectMilestones()
		if len(milestones) > 0 {
			growth["milestones"] = milestones
		}
		result["growth"] = growth
	}

	// Challenges
	if focus == "all" || focus == "challenges" {
		var challenges []string
		patterns := s.DetectPatterns("direct")
		for _, p := range patterns {
			if p.Severity == "challenge" || p.Severity == "warning" {
				challenges = append(challenges, p.Description)
			}
		}
		if challenges == nil {
			challenges = []string{}
		}
		result["challenges"] = challenges
	}

	// Soul summary
	if s.projectRoot != "" && username != "" {
		soulFiles := listUserSoulFiles(s.projectRoot, username)
		var soulSummaries []string
		for _, role := range soulFiles {
			res, err := ResolveSoul(s.projectRoot, username, role)
			if err == nil {
				if merged, ok := res["merged"].(string); ok {
					lines := strings.Split(merged, "\n")
					summary := role + " — "
					principleCount := 0
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "- ") && !strings.Contains(line, "여기에") {
							principleCount++
						}
					}
					// Count learned items
					learnedCount := 0
					for _, line := range lines {
						if strings.Contains(line, "] ") && strings.HasPrefix(strings.TrimSpace(line), "- [") {
							learnedCount++
						}
					}
					summary += fmt.Sprintf("%d principles, %d learned", principleCount, learnedCount)
					soulSummaries = append(soulSummaries, summary)
				}
			}
		}
		if len(soulSummaries) > 0 {
			result["soul_summary"] = soulSummaries
		}
	}

	// Permission audit
	if s.projectRoot != "" {
		if warnings := auditApprovedCommands(s.projectRoot); len(warnings) > 0 {
			result["permission_audit"] = warnings
		}
	}

	// Recent agent traces
	if traces := s.getRecentTraces(10); len(traces) > 0 {
		result["recent_traces"] = traces
	}

	return result, nil
}

// auditApprovedCommands scans .claude/settings.json for risky approved commands.
func auditApprovedCommands(projectRoot string) []string {
	riskyPatterns := map[string]string{
		"Bash(rm:*)":     "rm — can delete any file",
		"Bash(chmod:*)":  "chmod — can change file permissions",
		"Bash(chown:*)":  "chown — can change file ownership",
		"Bash(curl:*)":   "curl — can download/upload to any URL",
		"Bash(wget:*)":   "wget — can download from any URL",
		"Bash(kill:*)":   "kill — can kill any process",
		"Bash(pkill:*)":  "pkill — can kill processes by name",
		"Bash(docker:*)": "docker — full container access",
		"Bash(bash:*)":   "bash — arbitrary shell execution",
		"Bash(sh:*)":     "sh — arbitrary shell execution",
		"Bash(sudo:*)":   "sudo — root access",
	}

	var warnings []string
	for _, name := range []string{"settings.json", "settings.local.json"} {
		path := projectRoot + "/.claude/" + name
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		for pattern, desc := range riskyPatterns {
			if strings.Contains(content, pattern) {
				warnings = append(warnings, fmt.Sprintf("[%s] %s (%s)", name, desc, pattern))
			}
		}
	}
	return warnings
}

// detectMilestones finds notable achievements.
func (s *SQLiteStore) detectMilestones() []string {
	var milestones []string

	// Check approval rate milestones
	var total, approved int
	if err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM persona_stats`).Scan(&total, &approved); err != nil {
		log.Printf("c4: twin: detectMilestones approval stats: %v", err)
	}

	if total >= 10 {
		rate := float64(approved) / float64(total) * 100
		if rate >= 90 {
			milestones = append(milestones, fmt.Sprintf("90%% approval rate achieved (%d/%d tasks)", approved, total))
		} else if rate >= 80 {
			milestones = append(milestones, fmt.Sprintf("80%% approval rate achieved (%d/%d tasks)", approved, total))
		}
	}

	// Total tasks completed milestones
	var done int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status='done'").Scan(&done); err != nil {
		log.Printf("c4: twin: detectMilestones done count: %v", err)
	}
	switch {
	case done >= 100:
		milestones = append(milestones, fmt.Sprintf("%d tasks completed", done))
	case done >= 50:
		milestones = append(milestones, fmt.Sprintf("%d tasks completed", done))
	case done >= 20:
		milestones = append(milestones, fmt.Sprintf("%d tasks completed", done))
	}

	return milestones
}

// --- Helper ---

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func isoWeek(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}

// logTrace records an event in the c4_agent_traces table.
func (s *SQLiteStore) logTrace(eventType, agentID, taskID, detail string) {
	if _, err := s.db.Exec(`
		INSERT INTO c4_agent_traces (event_type, agent_id, task_id, detail)
		VALUES (?, ?, ?, ?)`, eventType, agentID, taskID, detail); err != nil {
		log.Printf("c4: twin: logTrace %s: %v", eventType, err)
	}
}

// getRecentTraces returns the most recent agent trace events.
func (s *SQLiteStore) getRecentTraces(limit int) []map[string]string {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT event_type, agent_id, task_id, detail, created_at
		FROM c4_agent_traces
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var traces []map[string]string
	for rows.Next() {
		var eventType, agentID, taskID, detail, createdAt string
		if err := rows.Scan(&eventType, &agentID, &taskID, &detail, &createdAt); err != nil {
			continue
		}
		traces = append(traces, map[string]string{
			"event":      eventType,
			"agent":      agentID,
			"task":       taskID,
			"detail":     detail,
			"created_at": createdAt,
		})
	}
	return traces
}
