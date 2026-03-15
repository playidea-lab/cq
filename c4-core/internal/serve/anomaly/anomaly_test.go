package anomaly

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// compile-time interface assertion
var _ serve.Component = (*Monitor)(nil)

func mustNewKnowledgeStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAnomalyMonitor_Name(t *testing.T) {
	a := New(Config{Store: mustNewKnowledgeStore(t)})
	if a.Name() != "anomaly_monitor" {
		t.Errorf("Name() = %q, want %q", a.Name(), "anomaly_monitor")
	}
}

func TestAnomalyMonitor_DetectsAnomaly(t *testing.T) {
	store := mustNewKnowledgeStore(t)

	// Write an experiment doc with a metric outside range directly to the docs dir.
	writeExperimentDoc(t, store, "exp-test01", 0.9, `[{"name":"loss","min":0.1,"max":0.5}]`)

	a := New(Config{Store: store, PollInterval: time.Minute})
	a.check(context.Background())

	debates, err := store.List(string(knowledge.TypeDebate), "", 10)
	if err != nil {
		t.Fatalf("List debates: %v", err)
	}
	if len(debates) != 1 {
		t.Fatalf("expected 1 debate doc, got %d", len(debates))
	}
	domain, _ := debates[0]["domain"].(string)
	if domain != "escalation" {
		t.Errorf("domain = %q, want %q", domain, "escalation")
	}
}

func TestAnomalyMonitor_NoAnomaly(t *testing.T) {
	store := mustNewKnowledgeStore(t)

	// Metric is within range: no debate should be created.
	writeExperimentDoc(t, store, "exp-test02", 0.3, `[{"name":"loss","min":0.1,"max":0.5}]`)

	a := New(Config{Store: store, PollInterval: time.Minute})
	a.check(context.Background())

	debates, err := store.List(string(knowledge.TypeDebate), "", 10)
	if err != nil {
		t.Fatalf("List debates: %v", err)
	}
	if len(debates) != 0 {
		t.Errorf("expected 0 debate docs, got %d", len(debates))
	}
}

func TestAnomalyMonitor_StopGraceful(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	a := New(Config{Store: store, PollInterval: time.Hour})

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Stop(ctx) //nolint:errcheck
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Error("Stop() did not return within 2s (goroutine leak?)")
	}
}

func TestAnomalyMonitor_NoDuplicateEscalation(t *testing.T) {
	store := mustNewKnowledgeStore(t)

	// Metric is out of range.
	writeExperimentDoc(t, store, "exp-test03", 0.9, `[{"name":"loss","min":0.1,"max":0.5}]`)

	a := New(Config{Store: store, PollInterval: time.Minute})

	// First check: should create one debate.
	a.check(context.Background())
	debates, err := store.List(string(knowledge.TypeDebate), "", 10)
	if err != nil {
		t.Fatalf("List debates (round 1): %v", err)
	}
	if len(debates) != 1 {
		t.Fatalf("expected 1 debate after first check, got %d", len(debates))
	}

	// Second check: same hypothesis_id within 24h — must be skipped.
	a.check(context.Background())
	debates2, err := store.List(string(knowledge.TypeDebate), "", 10)
	if err != nil {
		t.Fatalf("List debates (round 2): %v", err)
	}
	if len(debates2) != 1 {
		t.Errorf("expected still 1 debate after second check (dedup), got %d", len(debates2))
	}
}

// writeExperimentDoc writes a Markdown experiment document with expected_metrics_range
// and a metric value directly to the store's docs directory, then rebuilds the index.
func writeExperimentDoc(t *testing.T, store *knowledge.Store, id string, lossVal float64, rangeJSON string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	md := fmt.Sprintf(`---
id: %s
type: experiment
title: test experiment
expected_metrics_range: %s
loss: %g
created_at: %s
updated_at: %s
version: 1
---
test body
`, id, rangeJSON, lossVal, now, now)

	path := filepath.Join(store.DocsDir(), id+".md")
	if err := os.WriteFile(path, []byte(md), 0644); err != nil {
		t.Fatalf("writeExperimentDoc: %v", err)
	}
	if _, err := store.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
}
