package guard

import (
	"context"
	"fmt"
)

// AssignRole assigns a role to an actor.
// If the actor already has this role the operation is a no-op.
func (e *Engine) AssignRole(ctx context.Context, actor, role string) error {
	_, err := e.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO roles (actor, role) VALUES (?, ?)`,
		actor, role,
	)
	if err != nil {
		return fmt.Errorf("guard: assign role %q to %q: %w", role, actor, err)
	}
	return nil
}

// UnassignRole removes a role from an actor.
func (e *Engine) UnassignRole(ctx context.Context, actor, role string) error {
	_, err := e.db.ExecContext(ctx,
		`DELETE FROM roles WHERE actor = ? AND role = ?`,
		actor, role,
	)
	if err != nil {
		return fmt.Errorf("guard: unassign role %q from %q: %w", role, actor, err)
	}
	return nil
}

// GrantPermission sets a permission for a role on a specific tool.
// Calling this again with the same (role, tool) key updates the action.
func (e *Engine) GrantPermission(ctx context.Context, role, tool string, action Action) error {
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO role_permissions (role, tool, action) VALUES (?, ?, ?)
		 ON CONFLICT(role, tool) DO UPDATE SET action = excluded.action`,
		role, tool, action.String(),
	)
	if err != nil {
		return fmt.Errorf("guard: grant permission role=%q tool=%q: %w", role, tool, err)
	}
	return nil
}

// rbacCheck evaluates the RBAC tables for actor + tool.
// Returns (action, reason, found, error).
// If the actor has no roles or no matching permission, found=false.
// When an actor has multiple roles with conflicting rules, Deny wins.
func (e *Engine) rbacCheck(ctx context.Context, actor, tool string) (Action, string, bool, error) {
	// Fetch all roles for actor.
	roleRows, err := e.db.QueryContext(ctx,
		`SELECT role FROM roles WHERE actor = ?`, actor)
	if err != nil {
		return ActionAllow, "", false, err
	}
	defer roleRows.Close()

	var roles []string
	for roleRows.Next() {
		var r string
		if err := roleRows.Scan(&r); err != nil {
			return ActionAllow, "", false, err
		}
		roles = append(roles, r)
	}
	if err := roleRows.Err(); err != nil {
		return ActionAllow, "", false, err
	}
	if len(roles) == 0 {
		return ActionAllow, "", false, nil
	}

	// For each role, look up the permission for this tool.
	// Deny always wins over Allow/AuditOnly.
	var (
		bestAction = ActionAllow
		found      bool
	)
	for _, role := range roles {
		var actionStr string
		err := e.db.QueryRowContext(ctx,
			`SELECT action FROM role_permissions WHERE role = ? AND tool = ?`,
			role, tool,
		).Scan(&actionStr)
		if err != nil {
			continue // no row for this role/tool pair — skip
		}
		found = true
		a := parseAction(actionStr)
		if a == ActionDeny {
			return ActionDeny, fmt.Sprintf("role %q denies %q", role, tool), true, nil
		}
		if a == ActionAuditOnly && bestAction == ActionAllow {
			bestAction = ActionAuditOnly
		}
	}

	if !found {
		return ActionAllow, "", false, nil
	}
	return bestAction, fmt.Sprintf("rbac: %v", bestAction), true, nil
}
