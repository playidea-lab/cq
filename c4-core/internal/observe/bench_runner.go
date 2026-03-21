package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ChatMessage is a minimal message type used by the bench runner.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatReq is a minimal chat request used by the bench runner.
type ChatReq struct {
	Model    string
	Messages []ChatMessage
}

// ChatResp is a minimal chat response used by the bench runner.
type ChatResp struct {
	Content string
}

// BenchGateway is the minimal interface the bench runner needs from an LLM gateway.
// This avoids a circular import between observe and llm packages.
type BenchGateway interface {
	Chat(ctx context.Context, taskType string, req *ChatReq) (*ChatResp, error)
}

// Scorer evaluates a model's answer against a reference solution.
type Scorer interface {
	Score(problem, answer, reference string) (passed bool, score float64, err error)
}

// ExactMatchScorer returns 1.0 if answer == reference (trimmed), 0.0 otherwise.
type ExactMatchScorer struct{}

// Score implements Scorer for exact match comparison.
func (s ExactMatchScorer) Score(_, answer, reference string) (bool, float64, error) {
	passed := strings.TrimSpace(answer) == strings.TrimSpace(reference)
	score := 0.0
	if passed {
		score = 1.0
	}
	return passed, score, nil
}

// TestPassScorer runs the answer as a test (go test or pytest) and returns pass/fail.
// The problem field is interpreted as the language hint ("go" or "python").
// The answer field is written to a temp file and the appropriate test runner is invoked.
type TestPassScorer struct{}

// Score implements Scorer by running a subprocess test.
func (s TestPassScorer) Score(problem, answer, _ string) (bool, float64, error) {
	lang := strings.ToLower(strings.TrimSpace(problem))

	tmpDir, err := os.MkdirTemp("", "bench-test-*")
	if err != nil {
		return false, 0.0, fmt.Errorf("testpass: mkdtemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var cmd *exec.Cmd
	switch lang {
	case "python":
		testFile := filepath.Join(tmpDir, "test_bench.py")
		if err := os.WriteFile(testFile, []byte(answer), 0o600); err != nil {
			return false, 0.0, fmt.Errorf("testpass: write python file: %w", err)
		}
		cmd = exec.Command("pytest", testFile, "-q")
	default: // "go" or unspecified
		// Write a minimal Go test file.
		testFile := filepath.Join(tmpDir, "bench_test.go")
		if err := os.WriteFile(testFile, []byte(answer), 0o600); err != nil {
			return false, 0.0, fmt.Errorf("testpass: write go file: %w", err)
		}
		cmd = exec.Command("go", "test", tmpDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	runErr := cmd.Run()
	passed := runErr == nil
	score := 0.0
	if passed {
		score = 1.0
	}
	return passed, score, nil
}

// BenchSample is a single problem in a benchmark dataset.
type BenchSample struct {
	Problem   string `json:"problem"`
	Reference string `json:"reference"`
}

// SampleLoader loads benchmark samples from a dataset path.
// Returns a slice of BenchSamples up to maxSamples (0 = unlimited).
type SampleLoader func(dataset string, maxSamples int) ([]BenchSample, error)

// defaultSampleLoader reads JSONL from the dataset path.
func defaultSampleLoader(dataset string, maxSamples int) ([]BenchSample, error) {
	f, err := os.Open(dataset)
	if err != nil {
		return nil, fmt.Errorf("sample loader: open %q: %w", dataset, err)
	}
	defer f.Close()

	var samples []BenchSample
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var s BenchSample
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("sample loader: parse line: %w", err)
		}
		samples = append(samples, s)
		if maxSamples > 0 && len(samples) >= maxSamples {
			break
		}
	}
	return samples, scanner.Err()
}

// benchRunnerOpts holds optional dependencies for RunBench (for testing).
type benchRunnerOpts struct {
	gateway     BenchGateway
	loader      SampleLoader
	scorerFn    func(name string) Scorer
	outputPath  string // override output path; if empty, derived from cfg
}

// BenchRunnerOption is a functional option for RunBench internals.
type BenchRunnerOption func(*benchRunnerOpts)

// WithGateway sets the LLM gateway for RunBench.
func WithGateway(g BenchGateway) BenchRunnerOption {
	return func(o *benchRunnerOpts) { o.gateway = g }
}

// WithSampleLoader overrides the default JSONL loader.
func WithSampleLoader(l SampleLoader) BenchRunnerOption {
	return func(o *benchRunnerOpts) { o.loader = l }
}

// WithScorerFn overrides the scorer selection logic.
func WithScorerFn(fn func(name string) Scorer) BenchRunnerOption {
	return func(o *benchRunnerOpts) { o.scorerFn = fn }
}

// WithOutputPath overrides the JSONL output path (used in tests).
func WithOutputPath(p string) BenchRunnerOption {
	return func(o *benchRunnerOpts) { o.outputPath = p }
}

// defaultScorerFn maps scorer names to implementations.
func defaultScorerFn(name string) Scorer {
	switch name {
	case "test_pass":
		return TestPassScorer{}
	default: // "exact_match" and fallback
		return ExactMatchScorer{}
	}
}

// pair holds one (model, benchmark) combination to execute.
type pair struct {
	model     ModelEntry
	benchmark BenchmarkEntry
}

// pairResult is the per-(model,benchmark) aggregate before final summary.
type pairResult struct {
	model     string
	benchmark string
	results   []BenchResult
}

// RunBench executes the N-model × M-benchmark matrix defined in cfg.
// Results are streamed to a JSONL file under cfg.Run.OutputDir and
// aggregated into a slice of BenchSummary (one per model×benchmark pair).
func RunBench(cfg BenchConfig, opts ...BenchRunnerOption) ([]*BenchSummary, error) {
	o := &benchRunnerOpts{
		loader:   defaultSampleLoader,
		scorerFn: defaultScorerFn,
	}
	for _, opt := range opts {
		opt(o)
	}

	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("RunBench: no models configured")
	}
	if len(cfg.Benchmarks) == 0 {
		return nil, fmt.Errorf("RunBench: no benchmarks configured")
	}

	// Prepare output directory and JSONL file.
	outputDir := cfg.Run.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("RunBench: mkdir %q: %w", outputDir, err)
	}

	jsonlPath := o.outputPath
	if jsonlPath == "" {
		jsonlPath = filepath.Join(outputDir, "bench_results.jsonl")
	}

	outFile, err := os.Create(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("RunBench: create output file: %w", err)
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	var writerMu sync.Mutex

	writeResult := func(r BenchResult) error {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		writerMu.Lock()
		defer writerMu.Unlock()
		_, err = writer.Write(append(data, '\n'))
		if err != nil {
			return err
		}
		return writer.Flush()
	}

	// Build the full matrix of pairs.
	var pairs []pair
	for _, m := range cfg.Models {
		for _, b := range cfg.Benchmarks {
			pairs = append(pairs, pair{model: m, benchmark: b})
		}
	}

	maxWorkers := cfg.Run.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 4
	}

	// semaphore channel for concurrency control
	sem := make(chan struct{}, maxWorkers)

	// Collect results per (model, benchmark) pair.
	resultMap := make(map[string]*pairResult)
	var resultMapMu sync.Mutex

	for _, p := range pairs {
		key := p.model.Name + "||" + p.benchmark.Name
		resultMapMu.Lock()
		resultMap[key] = &pairResult{model: p.model.Name, benchmark: p.benchmark.Name}
		resultMapMu.Unlock()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(pairs))

	for _, p := range pairs {
		p := p // capture
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // release

			results, err := runOnePair(cfg, p, o, writeResult)
			if err != nil {
				errCh <- fmt.Errorf("pair %s/%s: %w", p.model.Name, p.benchmark.Name, err)
				return
			}

			key := p.model.Name + "||" + p.benchmark.Name
			resultMapMu.Lock()
			resultMap[key].results = results
			resultMapMu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	// Collect errors (non-fatal: log but continue to produce partial summaries).
	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}

	// Build summaries.
	summaries := make([]*BenchSummary, 0, len(pairs))
	for _, p := range pairs {
		key := p.model.Name + "||" + p.benchmark.Name
		pr := resultMap[key]
		if pr == nil || len(pr.results) == 0 {
			continue
		}
		summary := aggregateResults(pr.results)
		summaries = append(summaries, summary)
	}

	if len(errs) > 0 && len(summaries) == 0 {
		return nil, fmt.Errorf("RunBench: all pairs failed: %s", strings.Join(errs, "; "))
	}

	return summaries, nil
}

// runOnePair executes all samples for a single (model, benchmark) pair.
func runOnePair(cfg BenchConfig, p pair, o *benchRunnerOpts, writeResult func(BenchResult) error) ([]BenchResult, error) {
	samples, err := o.loader(p.benchmark.Dataset, p.benchmark.MaxSamples)
	if err != nil {
		return nil, fmt.Errorf("load samples: %w", err)
	}

	scorer := o.scorerFn(p.benchmark.Scorer)
	timeout := cfg.Defaults.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}

	var results []BenchResult
	for _, sample := range samples {
		start := time.Now()
		var answer string
		var callErr error

		if o.gateway != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			resp, err := o.gateway.Chat(ctx, "bench", &ChatReq{
				Model: p.model.Name,
				Messages: []ChatMessage{
					{Role: "user", Content: sample.Problem},
				},
			})
			cancel()
			if err != nil {
				callErr = err
			} else {
				answer = resp.Content
			}
		}

		latency := time.Since(start)

		r := BenchResult{
			Model:     p.model.Name,
			Benchmark: p.benchmark.Name,
			Latency:   latency,
		}

		if callErr != nil {
			r.Error = callErr.Error()
		} else {
			_, score, scoreErr := scorer.Score(sample.Problem, answer, sample.Reference)
			if scoreErr != nil {
				r.Error = scoreErr.Error()
			} else {
				r.Score = score
			}
		}

		results = append(results, r)

		if wErr := writeResult(r); wErr != nil {
			return results, fmt.Errorf("write result: %w", wErr)
		}
	}

	return results, nil
}

// aggregateResults builds a BenchSummary from a slice of BenchResult for one (model,benchmark) pair.
func aggregateResults(results []BenchResult) *BenchSummary {
	if len(results) == 0 {
		return nil
	}

	model := results[0].Model
	benchmark := results[0].Benchmark

	var scores []float64
	var latenciesMs []float64
	errCount := 0

	for _, r := range results {
		if r.Error != "" {
			errCount++
		} else {
			scores = append(scores, r.Score)
		}
		latenciesMs = append(latenciesMs, float64(r.Latency.Milliseconds()))
	}

	summary := &BenchSummary{
		Model:     model,
		Benchmark: benchmark,
		SampleN:   len(results),
		ErrorRate: float64(errCount) / float64(len(results)),
		LatencyMs: ComputeMetricStats(latenciesMs),
	}

	if len(scores) > 0 {
		summary.Score = ComputeMetricStats(scores)
	}

	return summary
}

// ComputeMetricStats computes descriptive statistics for a slice of float64 values.
// Returns zero-value MetricStats if values is empty.
func ComputeMetricStats(values []float64) MetricStats {
	n := len(values)
	if n == 0 {
		return MetricStats{}
	}

	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(n)

	// Median
	var median float64
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2.0
	} else {
		median = sorted[n/2]
	}

	// Variance / std dev (population)
	variance := 0.0
	for _, v := range sorted {
		d := v - mean
		variance += d * d
	}
	variance /= float64(n)
	std := math.Sqrt(variance)

	percentile := func(p float64) float64 {
		idx := p / 100.0 * float64(n-1)
		lo := int(math.Floor(idx))
		hi := int(math.Ceil(idx))
		if lo == hi {
			return sorted[lo]
		}
		frac := idx - float64(lo)
		return sorted[lo]*(1-frac) + sorted[hi]*frac
	}

	return MetricStats{
		Mean:   mean,
		Median: median,
		Min:    sorted[0],
		Max:    sorted[n-1],
		Std:    std,
		P90:    percentile(90),
		P95:    percentile(95),
		P99:    percentile(99),
	}
}
