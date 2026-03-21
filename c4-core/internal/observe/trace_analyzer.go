package observe

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// traceOutcomeWithQuality extends TraceOutcome parsing to include an optional quality field.
type traceOutcomeWithQuality struct {
	Quality float64 `json:"quality"`
}

// ModelStats holds aggregated performance metrics for a model on a given task type.
type ModelStats struct {
	Model       string
	TaskType    string
	Count       int64
	SuccessRate float64
	AvgQuality  float64
	Composite   float64
	AvgLatency  float64
	AvgCost     float64
}

// TraceAnalyzer queries trace data and computes model performance statistics.
type TraceAnalyzer struct {
	db *sql.DB
}

// NewTraceAnalyzer creates a TraceAnalyzer backed by the given *sql.DB.
func NewTraceAnalyzer(db *sql.DB) *TraceAnalyzer {
	return &TraceAnalyzer{db: db}
}

// StatsByTaskType returns per-task_type model statistics.
// The map key is task_type; each value is a slice of ModelStats sorted by composite score descending.
func (a *TraceAnalyzer) StatsByTaskType() (map[string][]ModelStats, error) {
	const query = `
SELECT
    ts.task_type,
    ts.model,
    COUNT(*)                           AS cnt,
    CAST(SUM(ts.success) AS REAL) / COUNT(*) AS success_rate,
    AVG(ts.latency_ms)                 AS avg_latency,
    AVG(ts.cost_usd)                   AS avg_cost,
    t.outcome_json
FROM trace_steps ts
JOIN traces t ON t.id = ts.trace_id
WHERE ts.step_type = 'llm'
  AND ts.model != ''
  AND ts.task_type != ''
GROUP BY ts.task_type, ts.model, t.outcome_json
`
	rows, err := a.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("observe: StatsByTaskType query: %w", err)
	}
	defer rows.Close()

	// Accumulate per (task_type, model) across possibly multiple outcome_json rows.
	type accKey struct{ taskType, model string }
	type acc struct {
		count      int64
		successSum float64
		latencySum float64
		costSum    float64
		qualitySum float64
		qualityCnt int64
		rowCnt     int64
	}
	accs := make(map[accKey]*acc)

	for rows.Next() {
		var taskType, model string
		var cnt int64
		var successRate, avgLatency, avgCost float64
		var outcomeJSON sql.NullString

		if err := rows.Scan(&taskType, &model, &cnt, &successRate, &avgLatency, &avgCost, &outcomeJSON); err != nil {
			return nil, fmt.Errorf("observe: StatsByTaskType scan: %w", err)
		}

		k := accKey{taskType, model}
		a2 := accs[k]
		if a2 == nil {
			a2 = &acc{}
			accs[k] = a2
		}
		a2.count += cnt
		a2.successSum += successRate * float64(cnt)
		a2.latencySum += avgLatency * float64(cnt)
		a2.costSum += avgCost * float64(cnt)
		a2.rowCnt += cnt

		if outcomeJSON.Valid && outcomeJSON.String != "" {
			var o traceOutcomeWithQuality
			if jsonErr := json.Unmarshal([]byte(outcomeJSON.String), &o); jsonErr == nil && o.Quality > 0 {
				a2.qualitySum += o.Quality * float64(cnt)
				a2.qualityCnt += cnt
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("observe: StatsByTaskType rows: %w", err)
	}

	result := make(map[string][]ModelStats)
	for k, a2 := range accs {
		if a2.rowCnt == 0 {
			continue
		}
		total := float64(a2.rowCnt)
		successRate := a2.successSum / total
		avgLatency := a2.latencySum / total
		avgCost := a2.costSum / total

		var avgQuality float64
		if a2.qualityCnt > 0 {
			avgQuality = a2.qualitySum / float64(a2.qualityCnt)
		}

		composite := 0.6*successRate + 0.4*avgQuality

		ms := ModelStats{
			Model:       k.model,
			TaskType:    k.taskType,
			Count:       a2.count,
			SuccessRate: successRate,
			AvgQuality:  avgQuality,
			Composite:   composite,
			AvgLatency:  avgLatency,
			AvgCost:     avgCost,
		}
		result[k.taskType] = append(result[k.taskType], ms)
	}

	// Sort each slice by composite score descending.
	for taskType := range result {
		slice := result[taskType]
		for i := 1; i < len(slice); i++ {
			for j := i; j > 0 && slice[j].Composite > slice[j-1].Composite; j-- {
				slice[j], slice[j-1] = slice[j-1], slice[j]
			}
		}
		result[taskType] = slice
	}

	return result, nil
}

// BestModel returns the model with the highest composite score for the given taskType.
// Returns an error if no data is found for that task type.
func (a *TraceAnalyzer) BestModel(taskType string) (string, error) {
	stats, err := a.StatsByTaskType()
	if err != nil {
		return "", err
	}
	slice, ok := stats[taskType]
	if !ok || len(slice) == 0 {
		return "", fmt.Errorf("observe: no stats found for task_type %q", taskType)
	}
	return slice[0].Model, nil
}
