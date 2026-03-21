package observe

import "time"

// BenchConfig holds the full benchmark configuration loaded from TOML.
type BenchConfig struct {
	Meta       BenchMeta        `toml:"meta"`
	Defaults   BenchDefaults    `toml:"defaults"`
	Run        BenchRun         `toml:"run"`
	Models     []ModelEntry     `toml:"models"`
	Benchmarks []BenchmarkEntry `toml:"benchmarks"`
}

// BenchMeta holds metadata about the benchmark suite.
type BenchMeta struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

// BenchDefaults holds default values applied to all runs.
type BenchDefaults struct {
	TimeoutSeconds int `toml:"timeout_seconds"`
	MaxRetries     int `toml:"max_retries"`
}

// BenchRun holds runtime execution parameters.
type BenchRun struct {
	MaxWorkers int    `toml:"max_workers"`
	OutputDir  string `toml:"output_dir"`
	Seed       int64  `toml:"seed"`
}

// ModelEntry represents a single model under evaluation.
type ModelEntry struct {
	Name     string `toml:"name"`
	Provider string `toml:"provider"`
}

// BenchmarkEntry represents a single benchmark task definition.
type BenchmarkEntry struct {
	Name       string `toml:"name"`
	Dataset    string `toml:"dataset"`
	MaxSamples int    `toml:"max_samples"`
	Scorer     string `toml:"scorer"`
}

// MetricStats holds descriptive statistics for a single metric.
type MetricStats struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Std    float64 `json:"std"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

// BenchResult holds the raw result of a single benchmark run on one model.
type BenchResult struct {
	Model     string            `json:"model"`
	Benchmark string            `json:"benchmark"`
	Score     float64           `json:"score"`
	Latency   time.Duration     `json:"latency"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// BenchSummary aggregates BenchResult entries into per-model statistics.
type BenchSummary struct {
	Model      string                 `json:"model"`
	Benchmark  string                 `json:"benchmark"`
	SampleN    int                    `json:"sample_n"`
	Score      MetricStats            `json:"score"`
	LatencyMs  MetricStats            `json:"latency_ms"`
	ErrorRate  float64                `json:"error_rate"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}
