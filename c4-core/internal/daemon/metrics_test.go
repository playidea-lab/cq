package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMetrics_WithFile verifies that a metrics.json in workdir is parsed and stored.
func TestMetrics_WithFile(t *testing.T) {
	dir := t.TempDir()

	// Write metrics.json to the workdir (same as dir for simplicity)
	metricsData := `{"accuracy": 0.95, "loss": 0.12}`
	if err := os.WriteFile(filepath.Join(dir, "metrics.json"), []byte(metricsData), 0644); err != nil {
		t.Fatalf("write metrics.json: %v", err)
	}

	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})
	sched.Start(context.Background())
	defer sched.Stop()

	job, err := store.CreateJob(&JobSubmitRequest{
		Name:    "metrics-present",
		Command: "echo done",
		Workdir: dir,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for job to complete")
		default:
			got, _ := store.GetJob(job.ID)
			if got.Status == StatusSucceeded {
				if got.Metrics == nil {
					t.Fatal("expected Metrics to be populated, got nil")
				}
				if v, ok := got.Metrics["accuracy"]; !ok || v != 0.95 {
					t.Errorf("expected accuracy=0.95, got %v", v)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestMetrics_NoFile verifies that a missing metrics.json results in nil Metrics and no error.
func TestMetrics_NoFile(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})
	sched.Start(context.Background())
	defer sched.Stop()

	job, err := store.CreateJob(&JobSubmitRequest{
		Name:    "no-metrics",
		Command: "echo done",
		Workdir: dir,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for job to complete")
		default:
			got, _ := store.GetJob(job.ID)
			if got.Status == StatusSucceeded {
				if got.Metrics != nil {
					t.Errorf("expected Metrics to be nil, got %v", got.Metrics)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestMetrics_InvalidJSON verifies that malformed metrics.json logs an error but
// the job still reaches terminal (SUCCEEDED) state.
func TestMetrics_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON
	if err := os.WriteFile(filepath.Join(dir, "metrics.json"), []byte(`{invalid}`), 0644); err != nil {
		t.Fatalf("write metrics.json: %v", err)
	}

	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})
	sched.Start(context.Background())
	defer sched.Stop()

	job, err := store.CreateJob(&JobSubmitRequest{
		Name:    "bad-metrics",
		Command: "echo done",
		Workdir: dir,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for job to complete")
		default:
			got, _ := store.GetJob(job.ID)
			if got.Status.IsTerminal() {
				if got.Status != StatusSucceeded {
					t.Errorf("expected SUCCEEDED, got %s", got.Status)
				}
				// Metrics should be nil since JSON was invalid
				if got.Metrics != nil {
					t.Errorf("expected Metrics to be nil for invalid JSON, got %v", got.Metrics)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestMetrics_CustomPath verifies that MetricsPath takes precedence over default location.
func TestMetrics_CustomPath(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom_metrics.json")

	metricsData := `{"f1": 0.88}`
	if err := os.WriteFile(customPath, []byte(metricsData), 0644); err != nil {
		t.Fatalf("write custom metrics: %v", err)
	}

	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})
	sched.Start(context.Background())
	defer sched.Stop()

	job, err := store.CreateJob(&JobSubmitRequest{
		Name:        "custom-path",
		Command:     "echo done",
		Workdir:     dir,
		MetricsPath: customPath,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for job to complete")
		default:
			got, _ := store.GetJob(job.ID)
			if got.Status == StatusSucceeded {
				if got.Metrics == nil {
					t.Fatal("expected Metrics to be populated, got nil")
				}
				if v, ok := got.Metrics["f1"]; !ok || v != 0.88 {
					t.Errorf("expected f1=0.88, got %v", v)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
