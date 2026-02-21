// Package guard implements role-based access control (RBAC), audit logging,
// and policy enforcement for the C4 MCP server.
//
// Architecture:
//   - Engine: central coordinator — Check() is the hot path
//   - RBAC: role → actor mapping + permission grants (SQLite)
//   - Policy: ordered rules evaluated before RBAC (config > DB)
//   - Audit: every Check() call is appended to audit_log table
//
// Decision priority (highest wins):
//  1. Config-level PolicyRules (Priority field, higher = evaluated first)
//  2. DB-stored PolicyRules
//  3. RBAC role permissions
//  4. Default: Allow
package guard

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Action represents the outcome of an access-control check.
type Action int

const (
	// ActionAllow permits the tool call.
	ActionAllow Action = iota
	// ActionDeny blocks the tool call.
	ActionDeny
	// ActionAuditOnly permits the call but flags it for mandatory audit recording.
	ActionAuditOnly
)

func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionDeny:
		return "deny"
	case ActionAuditOnly:
		return "audit_only"
	default:
		return "unknown"
	}
}

// Config holds guard configuration (mirrors config.yaml guard section).
type Config struct {
	// Enabled: if false the engine always returns ActionAllow without evaluation.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
	// Policies: in-config policy rules evaluated with highest priority.
	Policies []PolicyRule `yaml:"policies" mapstructure:"policies"`
}

// Engine is the central guard coordinator.
type Engine struct {
	db     *sql.DB
	config Config
}

// NewEngine opens (or creates) a SQLite database at dbPath, applies the schema,
// and returns an Engine ready for use.
func NewEngine(dbPath string, config Config) (*Engine, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("guard: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Enable WAL for concurrent read/write without blocking.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("guard: pragma WAL: %w", err)
	}

	if err := applySchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("guard: apply schema: %w", err)
	}

	return &Engine{db: db, config: config}, nil
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	return e.db.Close()
}

// Check evaluates whether actor may call tool with args.
// The result is one of ActionAllow, ActionDeny, or ActionAuditOnly.
// Every call is recorded in the audit log regardless of the outcome.
func (e *Engine) Check(ctx context.Context, actor, tool string, args []byte) Action {
	if !e.config.Enabled {
		return ActionAllow
	}

	action, reason := e.evaluate(ctx, actor, tool, args)

	// Record synchronously. Audit write is best-effort; errors are ignored.
	entry := AuditEntry{
		Actor:  actor,
		Tool:   tool,
		Action: action,
		Reason: reason,
	}
	// Best-effort: ignore audit write errors here.
	_ = e.writeAudit(ctx, entry)

	return action
}

// evaluate runs the policy chain and returns the action + reason string.
// Order: config policies → DB policies → RBAC → default Allow.
func (e *Engine) evaluate(ctx context.Context, actor, tool string, _ []byte) (Action, string) {
	// 1. Config-level policies (sorted by descending Priority)
	configPolicies := sortedByPriorityDesc(e.config.Policies)
	for _, p := range configPolicies {
		if matchTool(p.Tool, tool) {
			return p.Action, p.Reason
		}
	}

	// 2. DB-stored policies
	dbAction, dbReason, found, err := e.dbPolicyCheck(ctx, tool)
	if err != nil {
		return ActionDeny, "db error: policy check failed"
	}
	if found {
		return dbAction, dbReason
	}

	// 3. RBAC: check actor's roles
	rbacAction, rbacReason, found, err := e.rbacCheck(ctx, actor, tool)
	if err != nil {
		return ActionDeny, "db error: rbac check failed"
	}
	if found {
		return rbacAction, rbacReason
	}

	// 4. Default: allow
	return ActionAllow, "default"
}

// AuditLog records a custom audit entry. Useful for out-of-band events.
func (e *Engine) AuditLog(ctx context.Context, entry AuditEntry) error {
	return e.writeAudit(ctx, entry)
}

// AuditEntries returns the most recent n audit log entries (newest first).
// AuditEntries returns the last n audit log entries, optionally filtered by
// tool and actor. Empty strings mean "no filter".
func (e *Engine) AuditEntries(ctx context.Context, n int, toolFilter, actorFilter string) ([]AuditEntry, error) {
	query := `SELECT id, actor, tool, action, reason, created_at
		   FROM audit_log
		  WHERE (? = '' OR tool = ?)
		    AND (? = '' OR actor = ?)
		  ORDER BY id DESC
		  LIMIT ?`
	rows, err := e.db.QueryContext(ctx, query, toolFilter, toolFilter, actorFilter, actorFilter, n)
	if err != nil {
		return nil, fmt.Errorf("guard: audit query: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var ent AuditEntry
		var actionStr string
		var createdAt string
		if err := rows.Scan(&ent.ID, &ent.Actor, &ent.Tool, &actionStr, &ent.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("guard: audit scan: %w", err)
		}
		ent.Action = parseAction(actionStr)
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			ent.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			ent.CreatedAt = t
		}
		entries = append(entries, ent)
	}
	return entries, rows.Err()
}

// writeAudit persists an AuditEntry to the database.
func (e *Engine) writeAudit(ctx context.Context, entry AuditEntry) error {
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor, tool, action, reason, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.Actor, entry.Tool, entry.Action.String(), entry.Reason,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// applySchema creates the required tables if they do not exist.
func applySchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    actor      TEXT NOT NULL,
    tool       TEXT NOT NULL,
    action     TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS roles (
    actor TEXT NOT NULL,
    role  TEXT NOT NULL,
    PRIMARY KEY (actor, role)
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role   TEXT NOT NULL,
    tool   TEXT NOT NULL,
    action TEXT NOT NULL,
    PRIMARY KEY (role, tool)
);

CREATE TABLE IF NOT EXISTS policies (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    tool     TEXT NOT NULL UNIQUE,
    action   TEXT NOT NULL,
    reason   TEXT NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0
);
`
	_, err := db.Exec(schema)
	return err
}

// parseAction converts the string stored in SQLite back to an Action.
func parseAction(s string) Action {
	switch s {
	case "deny":
		return ActionDeny
	case "audit_only":
		return ActionAuditOnly
	default:
		return ActionAllow
	}
}
