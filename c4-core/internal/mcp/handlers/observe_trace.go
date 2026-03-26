//go:build c7_observe

package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/observe"
)

// traceState holds shared dependencies for trace MCP tools.
// Set once during initialization; read from handlers.
var traceState struct {
	mu       sync.RWMutex
	db       *sql.DB
	analyzer *observe.TraceAnalyzer
	policy   *observe.TraceDrivenPolicy
	gateway  *llm.Gateway // may be nil if llm_gateway not enabled
}

// InitObserveTraceState sets the dependencies used by trace MCP tools.
// Must be called before RegisterObserveTraceHandlers.
func InitObserveTraceState(db *sql.DB, analyzer *observe.TraceAnalyzer, policy *observe.TraceDrivenPolicy) {
	traceState.mu.Lock()
	defer traceState.mu.Unlock()
	traceState.db = db
	traceState.analyzer = analyzer
	traceState.policy = policy
}

// SetObserveTraceGateway wires an optional llm.Gateway for c4_observe_policy.
// Called separately (in mcp_init_observe_trace_llm.go) after LLM init.
func SetObserveTraceGateway(gw *llm.Gateway) {
	traceState.mu.Lock()
	defer traceState.mu.Unlock()
	traceState.gateway = gw
}

// RegisterObserveTraceHandlers registers the c4_observe_traces, c4_observe_trace_stats,
// and c4_observe_policy MCP tools.
func RegisterObserveTraceHandlers(reg *mcp.Registry) {
	// c4_observe_traces — list recent traces with optional filters
	reg.Register(mcp.ToolSchema{
		Name:        "cq_observe_traces",
		Description: "List recent LLM traces with optional filtering by session_id, task_id, or limit",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "Filter traces by session ID",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Filter traces by task ID",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of traces to return (default: 20, max: 100)",
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			SessionID string `json:"session_id"`
			TaskID    string `json:"task_id"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Limit <= 0 {
			args.Limit = 20
		}
		if args.Limit > 100 {
			args.Limit = 100
		}

		traceState.mu.RLock()
		db := traceState.db
		traceState.mu.RUnlock()

		if db == nil {
			return map[string]any{
				"traces": []any{},
				"note":   "trace DB not available; trace collection requires observe initialization with a database",
			}, nil
		}

		query := `
SELECT t.id, t.session_id, t.task_id, t.created_at, t.ended_at, t.outcome_json,
       COUNT(ts.id) AS step_count
FROM traces t
LEFT JOIN trace_steps ts ON ts.trace_id = t.id
WHERE 1=1`
		args2 := make([]any, 0, 3)
		if args.SessionID != "" {
			query += " AND t.session_id = ?"
			args2 = append(args2, args.SessionID)
		}
		if args.TaskID != "" {
			query += " AND t.task_id = ?"
			args2 = append(args2, args.TaskID)
		}
		query += " GROUP BY t.id ORDER BY t.created_at DESC LIMIT ?"
		args2 = append(args2, args.Limit)

		rows, err := db.Query(query, args2...)
		if err != nil {
			return nil, fmt.Errorf("query traces: %w", err)
		}
		defer rows.Close()

		type traceRow struct {
			ID          string  `json:"id"`
			SessionID   string  `json:"session_id"`
			TaskID      string  `json:"task_id"`
			CreatedAt   string  `json:"created_at"`
			EndedAt     *string `json:"ended_at,omitempty"`
			OutcomeJSON *string `json:"outcome,omitempty"`
			StepCount   int64   `json:"step_count"`
		}
		traces := make([]traceRow, 0)
		for rows.Next() {
			var r traceRow
			var endedAt sql.NullString
			var outcomeJSON sql.NullString
			if err := rows.Scan(&r.ID, &r.SessionID, &r.TaskID, &r.CreatedAt, &endedAt, &outcomeJSON, &r.StepCount); err != nil {
				return nil, fmt.Errorf("scan trace: %w", err)
			}
			if endedAt.Valid {
				r.EndedAt = &endedAt.String
			}
			if outcomeJSON.Valid && outcomeJSON.String != "" {
				r.OutcomeJSON = &outcomeJSON.String
			}
			traces = append(traces, r)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("traces rows: %w", err)
		}

		return map[string]any{
			"traces": traces,
			"count":  len(traces),
		}, nil
	})

	// c4_observe_trace_stats — per task_type model performance statistics
	reg.Register(mcp.ToolSchema{
		Name:        "cq_observe_trace_stats",
		Description: "Return per-task_type model performance statistics (success rate, quality, latency, cost) computed from trace data",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_type": map[string]any{
					"type":        "string",
					"description": "Optional task type filter; if omitted all task types are returned",
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			TaskType string `json:"task_type"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		traceState.mu.RLock()
		analyzer := traceState.analyzer
		traceState.mu.RUnlock()

		if analyzer == nil {
			return map[string]any{
				"stats": map[string]any{},
				"note":  "trace analyzer not available; requires observe initialization with a database",
			}, nil
		}

		allStats, err := analyzer.StatsByTaskType()
		if err != nil {
			return nil, fmt.Errorf("compute stats: %w", err)
		}

		if args.TaskType != "" {
			// Return only the requested task type.
			slice, ok := allStats[args.TaskType]
			if !ok {
				return map[string]any{
					"stats":     map[string]any{},
					"task_type": args.TaskType,
					"note":      fmt.Sprintf("no trace data found for task_type %q", args.TaskType),
				}, nil
			}
			return map[string]any{
				"task_type": args.TaskType,
				"models":    slice,
			}, nil
		}

		// Return all task types.
		return map[string]any{
			"stats": allStats,
		}, nil
	})

	// c4_observe_policy — compare current RoutingTable with SuggestRoutes()
	reg.Register(mcp.ToolSchema{
		Name:        "cq_observe_policy",
		Description: "Show the current LLM routing table and compare it with data-driven route suggestions from trace analysis",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		traceState.mu.RLock()
		policy := traceState.policy
		gw := traceState.gateway
		traceState.mu.RUnlock()

		if policy == nil {
			return map[string]any{
				"current_routing": nil,
				"suggested":       map[string]any{},
				"note":            "trace policy not available; requires observe initialization with a database",
			}, nil
		}

		suggested, err := policy.SuggestRoutes()
		if err != nil {
			return nil, fmt.Errorf("compute suggested routes: %w", err)
		}

		// Build suggested map for JSON output.
		suggestedOut := make(map[string]any, len(suggested))
		for taskType, ref := range suggested {
			suggestedOut[taskType] = map[string]any{
				"provider": ref.Provider,
				"model":    ref.Model,
			}
		}

		// Get current routing from gateway if available.
		var currentRouting any
		var diff []map[string]any
		if gw != nil {
			rt := gw.GetRouting()
			currentRouting = rt

			// Compute diff: task types where suggestion differs from current route.
			for taskType, sugg := range suggested {
				curr, hasCurr := rt.Routes[taskType]
				if !hasCurr {
					diff = append(diff, map[string]any{
						"task_type":    taskType,
						"action":       "add",
						"current":      nil,
						"suggested":    map[string]any{"provider": sugg.Provider, "model": sugg.Model},
						"note":         "no current route; suggested route available",
					})
				} else if curr.Model != sugg.Model || curr.Provider != sugg.Provider {
					diff = append(diff, map[string]any{
						"task_type":    taskType,
						"action":       "update",
						"current":      map[string]any{"provider": curr.Provider, "model": curr.Model},
						"suggested":    map[string]any{"provider": sugg.Provider, "model": sugg.Model},
					})
				}
			}
		}

		if diff == nil {
			diff = []map[string]any{}
		}

		return map[string]any{
			"current_routing": currentRouting,
			"suggested":       suggestedOut,
			"diff":            diff,
			"generated_at":    time.Now().UTC().Format(time.RFC3339),
		}, nil
	})
}
