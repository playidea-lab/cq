package observe

import (
	"testing"
)

const testBenchTOML = `
[meta]
name = "cq-eval-suite"
version = "1.0.0"

[defaults]
timeout_seconds = 30
max_retries = 3

[run]
max_workers = 4
output_dir = "/tmp/bench"
seed = 42

[[models]]
name = "gpt-4o"
provider = "openai"

[[models]]
name = "claude-3-5-sonnet"
provider = "anthropic"

[[benchmarks]]
name = "skill-trigger"
dataset = "datasets/skill_trigger_v1.jsonl"
max_samples = 500
scorer = "exact_match"

[[benchmarks]]
name = "code-quality"
dataset = "datasets/code_quality_v1.jsonl"
max_samples = 200
scorer = "llm_judge"
`

func TestBenchConfig(t *testing.T) {
	cfg, err := ParseBenchConfig([]byte(testBenchTOML))
	if err != nil {
		t.Fatalf("ParseBenchConfig error: %v", err)
	}

	// meta
	if cfg.Meta.Name != "cq-eval-suite" {
		t.Errorf("meta.name = %q, want %q", cfg.Meta.Name, "cq-eval-suite")
	}
	if cfg.Meta.Version != "1.0.0" {
		t.Errorf("meta.version = %q, want %q", cfg.Meta.Version, "1.0.0")
	}

	// defaults
	if cfg.Defaults.TimeoutSeconds != 30 {
		t.Errorf("defaults.timeout_seconds = %d, want 30", cfg.Defaults.TimeoutSeconds)
	}
	if cfg.Defaults.MaxRetries != 3 {
		t.Errorf("defaults.max_retries = %d, want 3", cfg.Defaults.MaxRetries)
	}

	// run
	if cfg.Run.MaxWorkers != 4 {
		t.Errorf("run.max_workers = %d, want 4", cfg.Run.MaxWorkers)
	}
	if cfg.Run.OutputDir != "/tmp/bench" {
		t.Errorf("run.output_dir = %q, want %q", cfg.Run.OutputDir, "/tmp/bench")
	}
	if cfg.Run.Seed != 42 {
		t.Errorf("run.seed = %d, want 42", cfg.Run.Seed)
	}

	// models
	if len(cfg.Models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(cfg.Models))
	}
	if cfg.Models[0].Name != "gpt-4o" || cfg.Models[0].Provider != "openai" {
		t.Errorf("models[0] = %+v, want {gpt-4o openai}", cfg.Models[0])
	}
	if cfg.Models[1].Name != "claude-3-5-sonnet" || cfg.Models[1].Provider != "anthropic" {
		t.Errorf("models[1] = %+v, want {claude-3-5-sonnet anthropic}", cfg.Models[1])
	}

	// benchmarks
	if len(cfg.Benchmarks) != 2 {
		t.Fatalf("len(benchmarks) = %d, want 2", len(cfg.Benchmarks))
	}
	b0 := cfg.Benchmarks[0]
	if b0.Name != "skill-trigger" || b0.Dataset != "datasets/skill_trigger_v1.jsonl" ||
		b0.MaxSamples != 500 || b0.Scorer != "exact_match" {
		t.Errorf("benchmarks[0] = %+v", b0)
	}
	b1 := cfg.Benchmarks[1]
	if b1.Name != "code-quality" || b1.MaxSamples != 200 || b1.Scorer != "llm_judge" {
		t.Errorf("benchmarks[1] = %+v", b1)
	}
}

func TestBenchConfigInvalidTOML(t *testing.T) {
	_, err := ParseBenchConfig([]byte("not valid toml :::"))
	if err == nil {
		t.Error("expected error for invalid TOML, got nil")
	}
}

func TestBenchConfigEmptyInput(t *testing.T) {
	cfg, err := ParseBenchConfig([]byte(""))
	if err != nil {
		t.Fatalf("empty TOML should not error: %v", err)
	}
	if cfg == nil {
		t.Error("expected non-nil config for empty input")
	}
}
