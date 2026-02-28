package llm

import (
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// CostEntry records a single LLM API call's cost data.
type CostEntry struct {
	Time       time.Time     `json:"time"`
	Provider   string        `json:"provider"`
	Model      string        `json:"model"`
	Input      int           `json:"input_tokens"`
	Output     int           `json:"output_tokens"`
	CostUSD    float64       `json:"cost_usd"`
	Latency    time.Duration `json:"latency_ms"`
	CacheRead  int           `json:"cache_read_tokens,omitempty"`
	CacheWrite int           `json:"cache_write_tokens,omitempty"`
	SavedUSD   float64       `json:"cache_savings_usd,omitempty"`
}

// ProviderCost aggregates costs for a single provider.
type ProviderCost struct {
	TotalUSD         float64 `json:"total_usd"`
	Requests         int     `json:"requests"`
	InputTok         int     `json:"input_tokens"`
	OutputTok        int     `json:"output_tokens"`
	CacheReadTok     int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTok    int     `json:"cache_write_tokens,omitempty"`
	SavedUSD         float64 `json:"cache_savings_usd,omitempty"`
	CacheHitRate     float64 `json:"cache_hit_rate,omitempty"`
	CacheSavingsRate float64 `json:"cache_savings_rate,omitempty"`
}

// CostReport is the aggregate cost summary.
type CostReport struct {
	TotalUSD               float64                 `json:"total_usd"`
	TotalReqs              int                     `json:"total_requests"`
	ByProvider             map[string]ProviderCost `json:"by_provider"`
	ByModel                map[string]float64      `json:"by_model"`
	GlobalCacheHitRate     float64                 `json:"global_cache_hit_rate,omitempty"`
	GlobalCacheSavingsRate float64                 `json:"global_cache_savings_rate,omitempty"`
}

// dbRow is a single row sent to the async writer goroutine.
type dbRow struct {
	ts              time.Time
	provider        string
	model           string
	promptTok       int
	completionTok   int
	cacheReadTok    int
	cacheWriteTok   int
	costUSD         float64
	latencyMS       int64
}

const dbChanCap = 1000

// CostTracker accumulates LLM usage costs in memory and optionally persists to SQLite.
type CostTracker struct {
	mu      sync.Mutex
	entries []CostEntry

	// async SQLite persistence (nil if SetDB not called)
	ch chan dbRow
	wg sync.WaitGroup
}

// NewCostTracker creates a new CostTracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{}
}

// SetDB wires an open *sql.DB for async persistence.
// It creates the llm_usage table if needed and starts a background writer goroutine.
// Must be called at most once. Safe to skip (in-memory-only mode remains).
func (ct *CostTracker) SetDB(db *sql.DB) {
	if db == nil {
		return
	}
	if ct.ch != nil {
		slog.Warn("llm: SetDB called more than once, ignoring")
		return
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS llm_usage (
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
	)`); err != nil {
		slog.Warn("llm: failed to create llm_usage table", "err", err)
		return
	}

	ct.ch = make(chan dbRow, dbChanCap)
	ct.wg.Add(1)
	go ct.writer(db)
}

// writer is the background goroutine that drains ct.ch and inserts rows.
func (ct *CostTracker) writer(db *sql.DB) {
	defer ct.wg.Done()
	for row := range ct.ch {
		if _, err := db.Exec(
			`INSERT INTO llm_usage
				(ts, provider, model, prompt_tok, completion_tok, cache_read_tok, cache_write_tok, cost_usd, latency_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ts.UTC().Format(time.RFC3339),
			row.provider, row.model,
			row.promptTok, row.completionTok,
			row.cacheReadTok, row.cacheWriteTok,
			row.costUSD, row.latencyMS,
		); err != nil {
			slog.Warn("llm: failed to write llm_usage row", "err", err)
		}
	}
}

// Close drains the channel and waits for the writer goroutine to finish.
// No-op if SetDB was never called.
func (ct *CostTracker) Close() {
	if ct.ch == nil {
		return
	}
	close(ct.ch)
	ct.wg.Wait()
}

// Record logs a single LLM API call.
func (ct *CostTracker) Record(provider, model string, usage TokenUsage, latency time.Duration) {
	info, _ := LookupModel(model)
	regularCost := float64(usage.InputTokens)*info.InputPer1M/1_000_000 +
		float64(usage.OutputTokens)*info.OutputPer1M/1_000_000
	cacheCost := float64(usage.CacheWriteTokens)*info.InputPer1M*1.25/1_000_000 +
		float64(usage.CacheReadTokens)*info.InputPer1M*0.10/1_000_000
	saved := float64(usage.CacheReadTokens) * info.InputPer1M * 0.90 / 1_000_000
	totalCost := regularCost + cacheCost

	now := time.Now()
	ct.mu.Lock()
	ct.entries = append(ct.entries, CostEntry{
		Time:       now,
		Provider:   provider,
		Model:      model,
		Input:      usage.InputTokens,
		Output:     usage.OutputTokens,
		CostUSD:    totalCost,
		Latency:    latency,
		CacheRead:  usage.CacheReadTokens,
		CacheWrite: usage.CacheWriteTokens,
		SavedUSD:   saved,
	})
	ct.mu.Unlock()

	// Async DB write: non-blocking, drop on overflow with warning.
	if ct.ch != nil {
		row := dbRow{
			ts:            now,
			provider:      provider,
			model:         model,
			promptTok:     usage.InputTokens,
			completionTok: usage.OutputTokens,
			cacheReadTok:  usage.CacheReadTokens,
			cacheWriteTok: usage.CacheWriteTokens,
			costUSD:       totalCost,
			latencyMS:     latency.Milliseconds(),
		}
		select {
		case ct.ch <- row:
		default:
			slog.Warn("llm: cost tracker buffer full, dropping row", "provider", provider, "model", model)
		}
	}
}

// Report returns an aggregate cost report.
func (ct *CostTracker) Report() CostReport {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	report := CostReport{
		ByProvider: make(map[string]ProviderCost),
		ByModel:    make(map[string]float64),
	}

	var globalInput, globalCacheRead, globalCacheWrite int

	for _, e := range ct.entries {
		report.TotalUSD += e.CostUSD
		report.TotalReqs++

		pc := report.ByProvider[e.Provider]
		pc.TotalUSD += e.CostUSD
		pc.Requests++
		pc.InputTok += e.Input
		pc.OutputTok += e.Output
		pc.CacheReadTok += e.CacheRead
		pc.CacheWriteTok += e.CacheWrite
		pc.SavedUSD += e.SavedUSD
		report.ByProvider[e.Provider] = pc

		report.ByModel[e.Model] += e.CostUSD

		globalInput += e.Input
		globalCacheRead += e.CacheRead
		globalCacheWrite += e.CacheWrite
	}

	// Calculate per-provider cache rates.
	for provider, pc := range report.ByProvider {
		cacheAttempts := pc.CacheReadTok + pc.CacheWriteTok
		if cacheAttempts > 0 {
			pc.CacheHitRate = float64(pc.CacheReadTok) / float64(cacheAttempts)
		}
		totalInput := pc.InputTok + pc.CacheReadTok + pc.CacheWriteTok
		if totalInput > 0 {
			pc.CacheSavingsRate = float64(pc.CacheReadTok) / float64(totalInput)
		}
		report.ByProvider[provider] = pc
	}

	// Calculate global cache rates.
	globalCacheAttempts := globalCacheRead + globalCacheWrite
	if globalCacheAttempts > 0 {
		report.GlobalCacheHitRate = float64(globalCacheRead) / float64(globalCacheAttempts)
	}
	globalTotalInput := globalInput + globalCacheRead + globalCacheWrite
	if globalTotalInput > 0 {
		report.GlobalCacheSavingsRate = float64(globalCacheRead) / float64(globalTotalInput)
	}

	return report
}

// EntryCount returns the number of recorded entries (for testing).
func (ct *CostTracker) EntryCount() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.entries)
}
