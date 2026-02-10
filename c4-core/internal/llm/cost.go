package llm

import (
	"sync"
	"time"
)

// CostEntry records a single LLM API call's cost data.
type CostEntry struct {
	Time     time.Time     `json:"time"`
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Input    int           `json:"input_tokens"`
	Output   int           `json:"output_tokens"`
	CostUSD  float64       `json:"cost_usd"`
	Latency  time.Duration `json:"latency_ms"`
}

// ProviderCost aggregates costs for a single provider.
type ProviderCost struct {
	TotalUSD  float64 `json:"total_usd"`
	Requests  int     `json:"requests"`
	InputTok  int     `json:"input_tokens"`
	OutputTok int     `json:"output_tokens"`
}

// CostReport is the aggregate cost summary.
type CostReport struct {
	TotalUSD   float64                 `json:"total_usd"`
	TotalReqs  int                     `json:"total_requests"`
	ByProvider map[string]ProviderCost `json:"by_provider"`
	ByModel    map[string]float64      `json:"by_model"`
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
	cost := float64(usage.InputTokens)*info.InputPer1M/1_000_000 +
		float64(usage.OutputTokens)*info.OutputPer1M/1_000_000

	ct.mu.Lock()
	ct.entries = append(ct.entries, CostEntry{
		Time:     time.Now(),
		Provider: provider,
		Model:    model,
		Input:    usage.InputTokens,
		Output:   usage.OutputTokens,
		CostUSD:  cost,
		Latency:  latency,
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

	for _, e := range ct.entries {
		report.TotalUSD += e.CostUSD
		report.TotalReqs++

		pc := report.ByProvider[e.Provider]
		pc.TotalUSD += e.CostUSD
		pc.Requests++
		pc.InputTok += e.Input
		pc.OutputTok += e.Output
		report.ByProvider[e.Provider] = pc

		report.ByModel[e.Model] += e.CostUSD
	}

	return report
}

// EntryCount returns the number of recorded entries (for testing).
func (ct *CostTracker) EntryCount() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.entries)
}
