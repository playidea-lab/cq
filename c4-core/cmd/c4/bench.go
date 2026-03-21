package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/changmin/c4-core/internal/observe"
	"github.com/spf13/cobra"
)

var (
	benchConfigFile string
	benchName       string
	benchModel      string
)

// benchCmd is the root command for benchmark operations.
var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Run and compare LLM benchmarks",
}

// benchRunCmd runs benchmarks defined in a config file.
var benchRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run benchmarks",
	Long: `Run LLM benchmarks defined in a TOML config file.

Examples:
  cq bench run -c config.toml
  cq bench run -b my-benchmark -m claude-3-5-sonnet`,
	RunE: runBenchRun,
}

// benchCompareCmd compares two benchmark result directories.
var benchCompareCmd = &cobra.Command{
	Use:   "compare <dir1> <dir2>",
	Short: "Compare two benchmark result directories",
	Args:  cobra.ExactArgs(2),
	RunE:  runBenchCompare,
}

// benchReportCmd shows a detailed report for a benchmark result directory.
var benchReportCmd = &cobra.Command{
	Use:   "report <dir>",
	Short: "Show detailed benchmark report",
	Args:  cobra.ExactArgs(1),
	RunE:  runBenchReport,
}

func init() {
	benchRunCmd.Flags().StringVarP(&benchConfigFile, "config", "c", "", "benchmark config file (TOML)")
	benchRunCmd.Flags().StringVarP(&benchName, "benchmark", "b", "", "single benchmark name to run")
	benchRunCmd.Flags().StringVarP(&benchModel, "model", "m", "", "single model name to run")

	benchCmd.AddCommand(benchRunCmd)
	benchCmd.AddCommand(benchCompareCmd)
	benchCmd.AddCommand(benchReportCmd)
	rootCmd.AddCommand(benchCmd)
}

func runBenchRun(cmd *cobra.Command, args []string) error {
	var cfg observe.BenchConfig

	if benchConfigFile != "" {
		loaded, err := observe.LoadBenchConfig(benchConfigFile)
		if err != nil {
			return err
		}
		cfg = *loaded
	} else if benchName != "" && benchModel != "" {
		// Single run mode
		cfg = observe.BenchConfig{
			Models:     []observe.ModelEntry{{Name: benchModel}},
			Benchmarks: []observe.BenchmarkEntry{{Name: benchName}},
		}
	} else {
		return fmt.Errorf("provide -c config.toml or both -b benchmark and -m model")
	}

	summaries, err := observe.RunBench(cfg)
	if err != nil {
		return fmt.Errorf("bench run: %w", err)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODEL\tBENCHMARK\tSAMPLES\tSCORE_MEAN\tLATENCY_P50ms\tERROR_RATE")
	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%.4f\t%.1f\t%.4f\n",
			s.Model, s.Benchmark, s.SampleN,
			s.Score.Mean, s.LatencyMs.Median, s.ErrorRate)
	}
	tw.Flush()
	return nil
}

// loadSummariesFromDir reads bench_results.jsonl in dir and aggregates into summaries.
func loadSummariesFromDir(dir string) ([]*observe.BenchSummary, error) {
	jsonlPath := dir + "/bench_results.jsonl"
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", jsonlPath, err)
	}
	defer f.Close()

	// Group results by model+benchmark.
	type key struct{ model, benchmark string }
	grouped := make(map[key][]observe.BenchResult)
	var order []key

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var r observe.BenchResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("parse result line: %w", err)
		}
		k := key{r.Model, r.Benchmark}
		if _, exists := grouped[k]; !exists {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	summaries := make([]*observe.BenchSummary, 0, len(order))
	for _, k := range order {
		s := observe.AggregateBenchResults(grouped[k])
		if s != nil {
			summaries = append(summaries, s)
		}
	}
	return summaries, nil
}

func runBenchCompare(cmd *cobra.Command, args []string) error {
	dir1, dir2 := args[0], args[1]

	sums1, err := loadSummariesFromDir(dir1)
	if err != nil {
		return fmt.Errorf("load dir1: %w", err)
	}
	sums2, err := loadSummariesFromDir(dir2)
	if err != nil {
		return fmt.Errorf("load dir2: %w", err)
	}

	// Index dir2 summaries by model+benchmark.
	type key struct{ model, benchmark string }
	idx2 := make(map[key]*observe.BenchSummary)
	for _, s := range sums2 {
		idx2[key{s.Model, s.Benchmark}] = s
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "MODEL\tBENCHMARK\tSCORE(%s)\tSCORE(%s)\tΔSCORE\tLATENCY(%s)ms\tLATENCY(%s)ms\tΔLATENCY\n",
		dir1, dir2, dir1, dir2)
	for _, s1 := range sums1 {
		k := key{s1.Model, s1.Benchmark}
		s2 := idx2[k]
		if s2 == nil {
			fmt.Fprintf(tw, "%s\t%s\t%.4f\t(missing)\t-\t%.1f\t(missing)\t-\n",
				s1.Model, s1.Benchmark, s1.Score.Mean, s1.LatencyMs.Median)
			continue
		}
		deltaScore := s2.Score.Mean - s1.Score.Mean
		deltaLatency := s2.LatencyMs.Median - s1.LatencyMs.Median
		fmt.Fprintf(tw, "%s\t%s\t%.4f\t%.4f\t%+.4f\t%.1f\t%.1f\t%+.1f\n",
			s1.Model, s1.Benchmark,
			s1.Score.Mean, s2.Score.Mean, deltaScore,
			s1.LatencyMs.Median, s2.LatencyMs.Median, deltaLatency)
	}
	tw.Flush()
	return nil
}

func runBenchReport(cmd *cobra.Command, args []string) error {
	dir := args[0]

	summaries, err := loadSummariesFromDir(dir)
	if err != nil {
		return fmt.Errorf("load dir: %w", err)
	}

	for _, s := range summaries {
		fmt.Printf("=== %s / %s ===\n", s.Model, s.Benchmark)
		fmt.Printf("  Samples   : %d\n", s.SampleN)
		fmt.Printf("  Error Rate: %.4f\n", s.ErrorRate)
		fmt.Printf("  Score     : mean=%.4f median=%.4f min=%.4f max=%.4f std=%.4f p90=%.4f p95=%.4f p99=%.4f\n",
			s.Score.Mean, s.Score.Median, s.Score.Min, s.Score.Max, s.Score.Std,
			s.Score.P90, s.Score.P95, s.Score.P99)
		fmt.Printf("  Latency ms: mean=%.1f median=%.1f min=%.1f max=%.1f std=%.1f p90=%.1f p95=%.1f p99=%.1f\n",
			s.LatencyMs.Mean, s.LatencyMs.Median, s.LatencyMs.Min, s.LatencyMs.Max, s.LatencyMs.Std,
			s.LatencyMs.P90, s.LatencyMs.P95, s.LatencyMs.P99)
		fmt.Println()
	}
	return nil
}
