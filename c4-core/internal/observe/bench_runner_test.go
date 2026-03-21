package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubGateway implements BenchGateway for testing.
type stubGateway struct {
	answer string
	err    error
}

func (s *stubGateway) Chat(_ context.Context, _ string, req *ChatReq) (*ChatResp, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &ChatResp{Content: s.answer}, nil
}

// stubLoader returns a fixed set of samples.
func stubLoader(samples []BenchSample) SampleLoader {
	return func(_ string, maxSamples int) ([]BenchSample, error) {
		if maxSamples > 0 && len(samples) > maxSamples {
			return samples[:maxSamples], nil
		}
		return samples, nil
	}
}

// TestComputeMetricStats verifies statistical calculations.
func TestComputeMetricStats(t *testing.T) {
	// Empty slice.
	s := ComputeMetricStats(nil)
	if s.Mean != 0 || s.Max != 0 {
		t.Errorf("empty: want zero, got %+v", s)
	}

	// Single value.
	s = ComputeMetricStats([]float64{5.0})
	if s.Mean != 5.0 || s.Min != 5.0 || s.Max != 5.0 || s.Std != 0 {
		t.Errorf("single: unexpected stats %+v", s)
	}

	// Known values: [1,2,3,4,5]
	s = ComputeMetricStats([]float64{3, 1, 4, 1, 5})
	wantMean := (3.0 + 1.0 + 4.0 + 1.0 + 5.0) / 5.0
	if abs(s.Mean-wantMean) > 1e-9 {
		t.Errorf("mean = %.4f, want %.4f", s.Mean, wantMean)
	}
	if s.Min != 1.0 {
		t.Errorf("min = %.4f, want 1.0", s.Min)
	}
	if s.Max != 5.0 {
		t.Errorf("max = %.4f, want 5.0", s.Max)
	}
	if s.Median != 3.0 {
		t.Errorf("median = %.4f, want 3.0", s.Median)
	}
	if s.P90 <= s.Median {
		t.Errorf("P90 (%.4f) should be >= median (%.4f)", s.P90, s.Median)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestExactMatchScorer verifies exact-match logic.
func TestExactMatchScorer(t *testing.T) {
	sc := ExactMatchScorer{}

	passed, score, err := sc.Score("", "hello", "hello")
	if err != nil || !passed || score != 1.0 {
		t.Errorf("exact match: want (true,1.0,nil), got (%v,%v,%v)", passed, score, err)
	}

	passed, score, err = sc.Score("", "hello", "world")
	if err != nil || passed || score != 0.0 {
		t.Errorf("no match: want (false,0.0,nil), got (%v,%v,%v)", passed, score, err)
	}

	// Whitespace trimming.
	passed, score, err = sc.Score("", "  hello  ", "hello")
	if err != nil || !passed || score != 1.0 {
		t.Errorf("trim: want (true,1.0,nil), got (%v,%v,%v)", passed, score, err)
	}
}

// TestRunBench verifies end-to-end matrix execution.
func TestRunBench(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "results.jsonl")

	samples := []BenchSample{
		{Problem: "q1", Reference: "ans1"},
		{Problem: "q2", Reference: "ans2"},
	}

	cfg := BenchConfig{
		Meta: BenchMeta{Name: "test-suite", Version: "1.0"},
		Defaults: BenchDefaults{
			TimeoutSeconds: 10,
			MaxRetries:     1,
		},
		Run: BenchRun{
			MaxWorkers: 2,
			OutputDir:  tmpDir,
		},
		Models: []ModelEntry{
			{Name: "model-a", Provider: "test"},
		},
		Benchmarks: []BenchmarkEntry{
			{Name: "bench-x", Dataset: "fake.jsonl", MaxSamples: 10, Scorer: "exact_match"},
		},
	}

	gw := &stubGateway{answer: "ans1"} // always returns "ans1"

	summaries, err := RunBench(cfg,
		WithGateway(gw),
		WithSampleLoader(stubLoader(samples)),
		WithScorerFn(defaultScorerFn),
		WithOutputPath(jsonlPath),
	)
	if err != nil {
		t.Fatalf("RunBench error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.Model != "model-a" {
		t.Errorf("model = %q, want %q", s.Model, "model-a")
	}
	if s.Benchmark != "bench-x" {
		t.Errorf("benchmark = %q, want %q", s.Benchmark, "bench-x")
	}
	if s.SampleN != 2 {
		t.Errorf("sample_n = %d, want 2", s.SampleN)
	}
	// 1 out of 2 samples should match (gateway always returns "ans1", ref[0]=="ans1", ref[1]=="ans2")
	// score mean = 0.5
	if abs(s.Score.Mean-0.5) > 1e-9 {
		t.Errorf("score.mean = %.4f, want 0.5", s.Score.Mean)
	}

	// Verify JSONL file was written.
	f, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		t.Errorf("jsonl line count = %d, want 2", len(lines))
	}
	for _, line := range lines {
		var r BenchResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("parse jsonl line: %v", err)
		}
		if r.Model != "model-a" {
			t.Errorf("jsonl model = %q, want %q", r.Model, "model-a")
		}
	}
}

// TestRunBenchMultiMatrix verifies 2 models × 2 benchmarks = 4 summaries.
func TestRunBenchMultiMatrix(t *testing.T) {
	tmpDir := t.TempDir()

	samples := []BenchSample{
		{Problem: "q1", Reference: "correct"},
	}

	cfg := BenchConfig{
		Defaults: BenchDefaults{TimeoutSeconds: 5},
		Run: BenchRun{
			MaxWorkers: 4,
			OutputDir:  tmpDir,
		},
		Models: []ModelEntry{
			{Name: "m1", Provider: "test"},
			{Name: "m2", Provider: "test"},
		},
		Benchmarks: []BenchmarkEntry{
			{Name: "b1", Dataset: "fake1.jsonl", Scorer: "exact_match"},
			{Name: "b2", Dataset: "fake2.jsonl", Scorer: "exact_match"},
		},
	}

	gw := &stubGateway{answer: "correct"}

	summaries, err := RunBench(cfg,
		WithGateway(gw),
		WithSampleLoader(stubLoader(samples)),
		WithScorerFn(defaultScorerFn),
	)
	if err != nil {
		t.Fatalf("RunBench multi: %v", err)
	}
	if len(summaries) != 4 {
		t.Fatalf("expected 4 summaries (2×2), got %d", len(summaries))
	}
	for _, s := range summaries {
		if s.Score.Mean != 1.0 {
			t.Errorf("(%s, %s) score.mean = %.2f, want 1.0", s.Model, s.Benchmark, s.Score.Mean)
		}
	}
}

// TestRunBenchNoModels verifies error on empty models.
func TestRunBenchNoModels(t *testing.T) {
	cfg := BenchConfig{
		Benchmarks: []BenchmarkEntry{{Name: "b"}},
	}
	_, err := RunBench(cfg)
	if err == nil {
		t.Error("expected error for empty models")
	}
}

// TestRunBenchNoBenchmarks verifies error on empty benchmarks.
func TestRunBenchNoBenchmarks(t *testing.T) {
	cfg := BenchConfig{
		Models: []ModelEntry{{Name: "m"}},
	}
	_, err := RunBench(cfg)
	if err == nil {
		t.Error("expected error for empty benchmarks")
	}
}
