package guard

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PolicyRule defines a single access-control rule.
// Rules are evaluated in descending Priority order; the first match wins.
type PolicyRule struct {
	// Tool is the tool name to match. Use "*" to match all tools.
	Tool string `yaml:"tool" mapstructure:"tool"`
	// Action is the enforcement action when matched.
	Action Action `yaml:"action" mapstructure:"action"`
	// Reason is an optional human-readable justification.
	Reason string `yaml:"reason" mapstructure:"reason"`
	// Priority controls evaluation order (higher = evaluated first).
	// Config-level rules default to 0 unless explicitly set.
	Priority int `yaml:"priority" mapstructure:"priority"`
}

// matchTool returns true if pattern matches tool.
// Supports exact match and the "*" wildcard.
func matchTool(pattern, tool string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, tool)
}

// sortedByPriorityDesc returns a copy of rules sorted by descending Priority.
func sortedByPriorityDesc(rules []PolicyRule) []PolicyRule {
	if len(rules) == 0 {
		return nil
	}
	cp := make([]PolicyRule, len(rules))
	copy(cp, rules)
	sort.Slice(cp, func(i, j int) bool {
		return cp[i].Priority > cp[j].Priority
	})
	return cp
}

// SavePolicy persists a PolicyRule to the database.
// If a rule for the same tool already exists, it is updated.
func (e *Engine) SavePolicy(ctx context.Context, rule PolicyRule) error {
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO policies (tool, action, reason, priority) VALUES (?, ?, ?, ?)
		 ON CONFLICT(tool) DO UPDATE SET
		     action   = excluded.action,
		     reason   = excluded.reason,
		     priority = excluded.priority`,
		rule.Tool, rule.Action.String(), rule.Reason, rule.Priority,
	)
	if err != nil {
		return fmt.Errorf("guard: save policy: %w", err)
	}
	return nil
}

// ListPolicies returns all policy rules stored in the database, ordered by descending priority.
func (e *Engine) ListPolicies(ctx context.Context) ([]PolicyRule, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT tool, action, reason, priority FROM policies ORDER BY priority DESC`)
	if err != nil {
		return nil, fmt.Errorf("guard: list policies: %w", err)
	}
	defer rows.Close()

	var rules []PolicyRule
	for rows.Next() {
		var p PolicyRule
		var actionStr string
		if err := rows.Scan(&p.Tool, &actionStr, &p.Reason, &p.Priority); err != nil {
			return nil, fmt.Errorf("guard: list policies scan: %w", err)
		}
		p.Action = parseAction(actionStr)
		rules = append(rules, p)
	}
	return rules, rows.Err()
}

// dbPolicyCheck queries the policies table for a matching rule.
// Returns (action, reason, found, error).
func (e *Engine) dbPolicyCheck(ctx context.Context, tool string) (Action, string, bool, error) {
	// Check exact tool match first, then wildcard.
	row := e.db.QueryRowContext(ctx,
		`SELECT action, reason FROM policies
		  WHERE tool = ? OR tool = '*'
		  ORDER BY priority DESC, CASE WHEN tool = ? THEN 0 ELSE 1 END
		  LIMIT 1`,
		tool, tool,
	)
	var actionStr, reason string
	err := row.Scan(&actionStr, &reason)
	if err != nil {
		// sql.ErrNoRows is not an error in our domain — just "not found".
		return ActionAllow, "", false, nil
	}
	return parseAction(actionStr), reason, true, nil
}
