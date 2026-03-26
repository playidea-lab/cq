package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// HealthDeps holds optional dependencies for health checks.
type HealthDeps struct {
	DB             *sql.DB
	Sidecar        LazyAddrGetter // nil if no sidecar
	KnowledgeStore *knowledge.Store
	StartTime      time.Time
}

// RegisterHealthHandler registers the c4_health MCP tool.
func RegisterHealthHandler(reg *mcp.Registry, deps *HealthDeps) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_health",
		Description: "Check subsystem health: sqlite, sidecar, knowledge",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleHealth(deps)
	})
}

type healthCheck struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // "ok" or "error"
	LatencyMs float64 `json:"latency_ms"`
	Error     *string `json:"error"`
}

func handleHealth(deps *HealthDeps) (any, error) {
	var checks []healthCheck
	timeout := 2 * time.Second

	// 1. SQLite
	checks = append(checks, checkSQLite(deps.DB, timeout))

	// 2. Sidecar
	checks = append(checks, checkSidecar(deps.Sidecar))

	// 3. Knowledge
	checks = append(checks, checkKnowledge(deps.KnowledgeStore, timeout))

	// Determine overall status
	overall := "healthy"
	for _, c := range checks {
		if c.Status == "error" {
			if c.Name == "sqlite" {
				overall = "unhealthy"
				break
			}
			overall = "degraded"
		}
	}

	return map[string]any{
		"status":         overall,
		"checks":         checks,
		"uptime_seconds": int(time.Since(deps.StartTime).Seconds()),
	}, nil
}

func checkSQLite(db *sql.DB, timeout time.Duration) healthCheck {
	if db == nil {
		errMsg := "database not initialized"
		return healthCheck{Name: "sqlite", Status: "error", Error: &errMsg}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	var n int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&n)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		errMsg := err.Error()
		return healthCheck{Name: "sqlite", Status: "error", LatencyMs: latency, Error: &errMsg}
	}
	return healthCheck{Name: "sqlite", Status: "ok", LatencyMs: latency}
}

func checkSidecar(lazy LazyAddrGetter) healthCheck {
	if lazy == nil {
		errMsg := "sidecar not configured"
		return healthCheck{Name: "sidecar", Status: "error", Error: &errMsg}
	}
	start := time.Now()
	addr, err := lazy.Addr()
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil || addr == "" {
		errMsg := "sidecar not started"
		if err != nil {
			errMsg = err.Error()
		}
		return healthCheck{Name: "sidecar", Status: "error", LatencyMs: latency, Error: &errMsg}
	}
	return healthCheck{Name: "sidecar", Status: "ok", LatencyMs: latency}
}

func checkKnowledge(store *knowledge.Store, timeout time.Duration) healthCheck {
	if store == nil {
		errMsg := "knowledge store not initialized"
		return healthCheck{Name: "knowledge", Status: "error", Error: &errMsg}
	}
	start := time.Now()
	// Quick DB ping via the underlying connection
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var n int
	err := store.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		errMsg := err.Error()
		return healthCheck{Name: "knowledge", Status: "error", LatencyMs: latency, Error: &errMsg}
	}
	return healthCheck{Name: "knowledge", Status: "ok", LatencyMs: latency}
}
