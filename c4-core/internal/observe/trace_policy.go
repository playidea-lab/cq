package observe

import (
	"fmt"

	"github.com/changmin/c4-core/internal/llm"
)

const defaultMinSamples = 5

// TraceDrivenPolicy suggests optimal model routing based on trace statistics.
type TraceDrivenPolicy struct {
	analyzer   *TraceAnalyzer
	minSamples int
}

// NewTraceDrivenPolicy creates a TraceDrivenPolicy with the given analyzer.
// minSamples defaults to 5 if <= 0.
func NewTraceDrivenPolicy(analyzer *TraceAnalyzer, minSamples int) *TraceDrivenPolicy {
	if minSamples <= 0 {
		minSamples = defaultMinSamples
	}
	return &TraceDrivenPolicy{
		analyzer:   analyzer,
		minSamples: minSamples,
	}
}

// SuggestRoutes returns the best ModelRef per task_type for task types that
// have at least minSamples recorded traces. The result is read-only; callers
// must not apply routes automatically (observe-only, v1).
func (p *TraceDrivenPolicy) SuggestRoutes() (map[string]llm.ModelRef, error) {
	statsByType, err := p.analyzer.StatsByTaskType()
	if err != nil {
		return nil, fmt.Errorf("observe: SuggestRoutes: %w", err)
	}

	routes := make(map[string]llm.ModelRef)
	for taskType, stats := range statsByType {
		if len(stats) == 0 {
			continue
		}
		// stats is already sorted by composite score descending.
		best := stats[0]
		if best.Count < int64(p.minSamples) {
			continue
		}
		routes[taskType] = llm.ModelRef{Model: best.Model}
	}
	return routes, nil
}
