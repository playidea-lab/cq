package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
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
		fmt.Fprintf(os.Stderr, "c4: twin: BuildTwinReview checkpoint stats: %v\n", err)
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
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if args.Focus == "" {
			args.Focus = "all"
		}

		return store.reflect(args.Focus)
	})
}

// RegisterPopReflectHandlers registers the c4_pop_reflect MCP tool.
// ks may be nil (knowledge store optional); in that case the tool returns an empty list.
func RegisterPopReflectHandlers(reg *mcp.Registry, ks *knowledge.Store) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_reflect",
		Description: "List pending HIGH-confidence proposals awaiting user validation. Use `cq reflect` CLI for interactive validation. This tool returns the list for programmatic access.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max proposals to return (default: 5)",
				},
				"confidence": map[string]any{
					"type":        "string",
					"enum":        []string{"HIGH", "MEDIUM", "ALL"},
					"description": "Filter by confidence level (default: HIGH)",
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Limit      int    `json:"limit"`
			Confidence string `json:"confidence"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if args.Confidence == "" {
			args.Confidence = "HIGH"
		}
		if args.Limit <= 0 {
			args.Limit = 5
		}

		if ks == nil {
			return map[string]any{
				"pending":       []map[string]any{},
				"total_pending": 0,
				"hint":          "Run `cq reflect` for interactive validation",
			}, nil
		}

		pending, err := ks.ListPending(args.Confidence, args.Limit)
		if err != nil {
			return nil, fmt.Errorf("listing pending proposals: %w", err)
		}

		return map[string]any{
			"pending":       pending,
			"total_pending": len(pending),
			"hint":          "Run `cq reflect` for interactive validation",
		}, nil
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
		fmt.Fprintf(os.Stderr, "c4: twin: reflect totalTasks: %v\n", err)
	}

	var doneTasks int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status='done'").Scan(&doneTasks); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: reflect doneTasks: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "c4: twin: detectMilestones approval stats: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "c4: twin: detectMilestones done count: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "c4: twin: logTrace %s: %v\n", eventType, err)
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
