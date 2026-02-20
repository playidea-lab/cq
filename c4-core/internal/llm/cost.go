package llm

import (
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
	TotalUSD              float64                 `json:"total_usd"`
	TotalReqs             int                     `json:"total_requests"`
	ByProvider            map[string]ProviderCost `json:"by_provider"`
	ByModel               map[string]float64      `json:"by_model"`
	GlobalCacheHitRate    float64                 `json:"global_cache_hit_rate,omitempty"`
	GlobalCacheSavingsRate float64                `json:"global_cache_savings_rate,omitempty"`
}

// CostTracker accumulates LLM usage costs in memory.
type CostTracker struct {
	mu      sync.Mutex
	entries []CostEntry
}

// NewCostTracker creates a new CostTracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{}
}

// Record logs a single LLM API call.
func (ct *CostTracker) Record(provider, model string, usage TokenUsage, latency time.Duration) {
	info, _ := LookupModel(model)
	regularCost := float64(usage.InputTokens)*info.InputPer1M/1_000_000 +
		float64(usage.OutputTokens)*info.OutputPer1M/1_000_000
	cacheCost := float64(usage.CacheWriteTokens)*info.InputPer1M*1.25/1_000_000 +
		float64(usage.CacheReadTokens)*info.InputPer1M*0.10/1_000_000
	saved := float64(usage.CacheReadTokens) * info.InputPer1M * 0.90 / 1_000_000

	ct.mu.Lock()
	ct.entries = append(ct.entries, CostEntry{
		Time:       time.Now(),
		Provider:   provider,
		Model:      model,
		Input:      usage.InputTokens,
		Output:     usage.OutputTokens,
		CostUSD:    regularCost + cacheCost,
		Latency:    latency,
		CacheRead:  usage.CacheReadTokens,
		CacheWrite: usage.CacheWriteTokens,
		SavedUSD:   saved,
	})
	ct.mu.Unlock()
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
