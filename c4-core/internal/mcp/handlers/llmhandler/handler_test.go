//go:build llm_gateway

package llmhandler

import (
	"database/sql"
	"encoding/json"
	"math"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/changmin/c4-core/internal/mcp"
)

// setupTestDB creates an in-memory SQLite database with the llm_usage table
// and returns the DB handle and a cleanup function.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS llm_usage (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		ts              DATETIME NOT NULL,
		provider        TEXT NOT NULL,
		model           TEXT NOT NULL,
		prompt_tok      INTEGER NOT NULL DEFAULT 0,
		completion_tok  INTEGER NOT NULL DEFAULT 0,
		cache_read_tok  INTEGER NOT NULL DEFAULT 0,
		cache_write_tok INTEGER NOT NULL DEFAULT 0,
		cost_usd        REAL NOT NULL DEFAULT 0,
		latency_ms      INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func insertRow(t *testing.T, db *sql.DB, ts time.Time, provider, model string, promptTok, completionTok, cacheReadTok, cacheWriteTok int, costUSD float64, latencyMS int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO llm_usage
		(ts, provider, model, prompt_tok, completion_tok, cache_read_tok, cache_write_tok, cost_usd, latency_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.UTC().Format(time.RFC3339), provider, model,
		promptTok, completionTok, cacheReadTok, cacheWriteTok, costUSD, latencyMS,
	)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
}

func TestLLMUsageStats_Basic(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()

	// Insert mock rows within the last 24h.
	insertRow(t, db, now.Add(-1*time.Hour), "anthropic", "claude-sonnet", 200, 100, 50, 10, 0.05, 500)
	insertRow(t, db, now.Add(-2*time.Hour), "anthropic", "claude-sonnet", 300, 150, 100, 20, 0.08, 700)
	insertRow(t, db, now.Add(-3*time.Hour), "openai", "gpt-4o", 400, 200, 0, 0, 0.12, 600)

	reg := mcp.NewRegistry()
	RegisterLLMHandlers(reg, nil, db) // nil gateway: only c4_llm_usage_stats is exercised here

	result, err := reg.Call("c4_llm_usage_stats", json.RawMessage(`{"hours": 24}`))
	if err != nil {
		t.Fatalf("c4_llm_usage_stats error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}

	// total_cost_usd = 0.05 + 0.08 + 0.12 = 0.25
	totalCost, ok := m["total_cost_usd"].(float64)
	if !ok {
		t.Fatalf("total_cost_usd type = %T", m["total_cost_usd"])
	}
	if math.Abs(totalCost-0.25) > 1e-9 {
		t.Errorf("total_cost_usd = %v, want 0.25", totalCost)
	}

	// total_tokens = (200+300+400) + (100+150+200) + (50+100+0) = 900+450+150 = 1500
	totalTokens, ok := m["total_tokens"].(int64)
	if !ok {
		t.Fatalf("total_tokens type = %T", m["total_tokens"])
	}
	if totalTokens != 1500 {
		t.Errorf("total_tokens = %d, want 1500", totalTokens)
	}

	// cache_utilization_rate = sum(cache_read) / (sum(prompt) + sum(cache_read))
	// = 150 / (900 + 150) = 150 / 1050 ≈ 0.1428...
	cacheHitRate, ok := m["cache_utilization_rate"].(float64)
	if !ok {
		t.Fatalf("cache_utilization_rate type = %T", m["cache_utilization_rate"])
	}
	wantRate := 150.0 / 1050.0
	if math.Abs(cacheHitRate-wantRate) > 1e-9 {
		t.Errorf("cache_utilization_rate = %v, want %v", cacheHitRate, wantRate)
	}

	// top_models: 2 models, ordered by cost desc (gpt-4o: 0.12, claude-sonnet: 0.13)
	topModels, ok := m["top_models"].([]map[string]any)
	if !ok {
		t.Fatalf("top_models type = %T", m["top_models"])
	}
	if len(topModels) != 2 {
		t.Fatalf("top_models count = %d, want 2", len(topModels))
	}
	// claude-sonnet total = 0.05+0.08 = 0.13 > gpt-4o 0.12 → first
	if topModels[0]["model"] != "claude-sonnet" {
		t.Errorf("top_models[0].model = %v, want claude-sonnet", topModels[0]["model"])
	}
	if topModels[0]["call_count"] != 2 {
		t.Errorf("top_models[0].call_count = %v, want 2", topModels[0]["call_count"])
	}

	periodHours, ok := m["period_hours"].(int)
	if !ok {
		t.Fatalf("period_hours type = %T", m["period_hours"])
	}
	if periodHours != 24 {
		t.Errorf("period_hours = %d, want 24", periodHours)
	}
}

func TestLLMUsageStats_EmptyDB(t *testing.T) {
	db := setupTestDB(t)

	reg := mcp.NewRegistry()
	RegisterLLMHandlers(reg, nil, db) // nil gateway: only c4_llm_usage_stats is exercised here

	result, err := reg.Call("c4_llm_usage_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("c4_llm_usage_stats error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}

	// Zero values for empty DB (division by zero prevention).
	totalCost, _ := m["total_cost_usd"].(float64)
	if totalCost != 0 {
		t.Errorf("total_cost_usd = %v, want 0", totalCost)
	}

	cacheHitRate, _ := m["cache_utilization_rate"].(float64)
	if cacheHitRate != 0 {
		t.Errorf("cache_utilization_rate = %v, want 0", cacheHitRate)
	}

	totalTokens, _ := m["total_tokens"].(int64)
	if totalTokens != 0 {
		t.Errorf("total_tokens = %v, want 0", totalTokens)
	}

	topModels, ok := m["top_models"].([]map[string]any)
	if !ok {
		t.Fatalf("top_models type = %T", m["top_models"])
	}
	if len(topModels) != 0 {
		t.Errorf("top_models count = %d, want 0", len(topModels))
	}

	// Default hours = 24.
	if m["period_hours"] != 24 {
		t.Errorf("period_hours = %v, want 24", m["period_hours"])
	}
}

// TestLLMUsageStats_CacheHitRateFormula verifies the specific formula:
// cache_utilization_rate = sum(cache_read_tok) / (sum(prompt_tok) + sum(cache_read_tok))
// where prompt_tok is the new (non-cached) input tokens.
// Example: prompt_tok=100, cache_read_tok=50 → 50 / (100+50) = 50/150 = 0.333...
func TestLLMUsageStats_CacheHitRateFormula(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()

	// Single row: prompt_tok=100, cache_read_tok=50.
	insertRow(t, db, now.Add(-1*time.Hour), "anthropic", "claude-sonnet", 100, 30, 50, 10, 0.01, 100)

	reg := mcp.NewRegistry()
	RegisterLLMHandlers(reg, nil, db) // nil gateway: only c4_llm_usage_stats is exercised here

	result, err := reg.Call("c4_llm_usage_stats", json.RawMessage(`{"hours": 24}`))
	if err != nil {
		t.Fatalf("c4_llm_usage_stats error: %v", err)
	}

	m := result.(map[string]any)

	// cache_utilization_rate = 50 / (100 + 50) = 50 / 150 ≈ 0.333...
	cacheHitRate, ok := m["cache_utilization_rate"].(float64)
	if !ok {
		t.Fatalf("cache_utilization_rate type = %T", m["cache_utilization_rate"])
	}
	want := 50.0 / 150.0
	if math.Abs(cacheHitRate-want) > 1e-9 {
		t.Errorf("cache_utilization_rate = %v, want %v (50/150)", cacheHitRate, want)
	}
}
