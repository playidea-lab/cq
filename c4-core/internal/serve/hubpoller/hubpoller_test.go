package hubpoller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/serve"
)

// compile-time: KnowledgeHubPoller implements serve.Component
var _ serve.Component = (*KnowledgeHubPoller)(nil)

func TestParseHubMetrics(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  map[string]string
	}{
		{
			name:  "plain KEY=VALUE",
			lines: []string{"mpjpe=42.1", "pa_mpjpe=38.5"},
			want:  map[string]string{"mpjpe": "42.1", "pa_mpjpe": "38.5"},
		},
		{
			name:  "at-prefixed @key=value from ExperimentWrapper",
			lines: []string{"@mpjpe=42.1", "@pa_mpjpe=38.5"},
			want:  map[string]string{"mpjpe": "42.1", "pa_mpjpe": "38.5"},
		},
		{
			name:  "mixed lines with noise",
			lines: []string{"Epoch 100 done", "@mpjpe=42.1", "status=completed", "no-equals-here"},
			want:  map[string]string{"mpjpe": "42.1", "status": "completed"},
		},
		{
			name:  "duplicate keys last wins",
			lines: []string{"@mpjpe=50.0", "@mpjpe=42.1"},
			want:  map[string]string{"mpjpe": "42.1"},
		},
		{
			name:  "empty lines",
			lines: []string{"", "  ", "=nokey"},
			want:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHubMetrics(tt.lines)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d keys, want %d: %v", len(got), len(tt.want), got)
			}
			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

// TestPollerWakeChannel verifies that sending on the wake channel triggers an
// immediate poll() without waiting for the next ticker tick.
// A subtest also verifies that a nil wake channel leaves ticker-only behavior intact.
func TestPollerWakeChannel(t *testing.T) {
	t.Run("wake triggers immediate poll", func(t *testing.T) {
		// Count actual HTTP requests made to the Hub during poll().
		var pollCount atomic.Int64
		fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pollCount.Add(1)
			// Return empty completed-jobs list so poll() succeeds quickly.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]")) //nolint:errcheck
		}))
		defer fakeSrv.Close()

		// Use a very long ticker interval so the ticker never fires during the test.
		p := New(Config{
			HubURL:       fakeSrv.URL,
			PollInterval: 24 * time.Hour,
			SeenPath:     t.TempDir() + "/seen.json",
		})

		wakeCh := make(chan struct{}, 1)
		p.SetWakeChannel(wakeCh)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := p.Start(ctx); err != nil {
			t.Fatalf("Start: %v", err)
		}

		// Capture baseline count (ticker-driven poll before wake signal).
		time.Sleep(20 * time.Millisecond)
		before := pollCount.Load()

		// Send wake signal — should trigger an immediate poll.
		wakeCh <- struct{}{}

		// Give the loop time to process.
		time.Sleep(100 * time.Millisecond)

		after := pollCount.Load()
		if after <= before {
			t.Errorf("expected at least one wake-driven poll: before=%d after=%d", before, after)
		}

		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		if err := p.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})

	t.Run("nil wake channel uses ticker only", func(t *testing.T) {
		// A poller with nil wake channel should compile and run without panicking.
		p := New(Config{
			PollInterval: 24 * time.Hour,
		})
		// wake is nil — the select case on nil channel never fires (Go spec).

		ctx, cancel := context.WithCancel(context.Background())
		if err := p.Start(ctx); err != nil {
			cancel()
			t.Fatalf("Start: %v", err)
		}

		// Let it run briefly; it should not panic.
		time.Sleep(20 * time.Millisecond)

		cancel()

		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		if err := p.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})
}

func TestParseMetrics_KeyValue(t *testing.T) {
	lines := []string{"LOSS=0.42", "ACCURACY=0.98"}
	got := ParseHubMetrics(lines)
	if got["LOSS"] != "0.42" {
		t.Errorf("LOSS = %q, want 0.42", got["LOSS"])
	}
	if got["ACCURACY"] != "0.98" {
		t.Errorf("ACCURACY = %q, want 0.98", got["ACCURACY"])
	}
}

func TestParseMetrics_EmptyLines(t *testing.T) {
	lines := []string{"", "   ", "no-equal-sign", "KEY=val"}
	got := ParseHubMetrics(lines)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
	if got["KEY"] != "val" {
		t.Errorf("KEY = %q, want val", got["KEY"])
	}
}

func TestParseMetrics_DuplicateKeys_LastWins(t *testing.T) {
	lines := []string{"K=first", "K=second", "K=last"}
	got := ParseHubMetrics(lines)
	if got["K"] != "last" {
		t.Errorf("K = %q, want last", got["K"])
	}
}

func TestParseMetrics_NoEquals(t *testing.T) {
	lines := []string{"justtext", "another line without equals"}
	got := ParseHubMetrics(lines)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseMetrics_ValueContainsEquals(t *testing.T) {
	// First '=' is the split point; value may contain '='
	lines := []string{"FORMULA=a=b+c"}
	got := ParseHubMetrics(lines)
	if got["FORMULA"] != "a=b+c" {
		t.Errorf("FORMULA = %q, want a=b+c", got["FORMULA"])
	}
}

func TestHubPoller_SeenIDs_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	seenPath := filepath.Join(dir, "seen.json")

	p := New(Config{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	// Initially empty
	loaded := p.loadSeenIDs()
	if len(loaded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(loaded))
	}

	// Save two entries
	m := map[string]SeenEntry{
		"job-1": {CompletedAt: time.Now()},
		"job-2": {CompletedAt: time.Now().Add(-time.Hour)},
	}
	if err := p.saveSeenIDs(m); err != nil {
		t.Fatalf("saveSeenIDs: %v", err)
	}

	// Load back
	loaded2 := p.loadSeenIDs()
	if len(loaded2) != 2 {
		t.Errorf("expected 2 entries, got %d", len(loaded2))
	}
	if _, ok := loaded2["job-1"]; !ok {
		t.Error("job-1 not found after load")
	}
	if _, ok := loaded2["job-2"]; !ok {
		t.Error("job-2 not found after load")
	}
}

func TestHubPoller_SeenIDs_TTLCleanup(t *testing.T) {
	dir := t.TempDir()
	seenPath := filepath.Join(dir, "seen.json")

	// Write a file with one stale (>30 days) and one fresh entry
	stale := time.Now().Add(-31 * 24 * time.Hour)
	fresh := time.Now()
	raw := map[string]SeenEntry{
		"old-job": {CompletedAt: stale},
		"new-job": {CompletedAt: fresh},
	}
	data, _ := json.Marshal(raw)
	os.WriteFile(seenPath, data, 0600) //nolint:errcheck

	p := New(Config{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	// Load and apply TTL cleanup via the extracted helper (same logic as poll()).
	seenIDs := p.loadSeenIDs()
	CleanupSeenIDs(seenIDs, time.Now().Add(-30*24*time.Hour))

	if len(seenIDs) != 1 {
		t.Errorf("expected 1 entry after TTL cleanup, got %d: %v", len(seenIDs), seenIDs)
	}
	if _, ok := seenIDs["new-job"]; !ok {
		t.Error("new-job should survive TTL cleanup")
	}
	if _, ok := seenIDs["old-job"]; ok {
		t.Error("old-job should be removed by TTL cleanup")
	}
}

func TestHubPoller_SeenIDs_AtomicSave(t *testing.T) {
	// Verify that saveSeenIDs uses atomic rename (no partial writes).
	dir := t.TempDir()
	seenPath := filepath.Join(dir, "seen.json")

	p := New(Config{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	m := map[string]SeenEntry{"j": {CompletedAt: time.Now()}}
	if err := p.saveSeenIDs(m); err != nil {
		t.Fatalf("saveSeenIDs: %v", err)
	}

	// Only the target file should exist; tmp files are gone
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file, got %d", len(entries))
	}
	if entries[0].Name() != filepath.Base(seenPath) {
		t.Errorf("unexpected file: %s", entries[0].Name())
	}
}

func TestKnowledgeHubPoller_ImplementsComponent(t *testing.T) {
	p := New(Config{PollInterval: 30 * time.Second})
	if p.Name() != "hub-knowledge-poller" {
		t.Errorf("Name() = %q, want hub-knowledge-poller", p.Name())
	}
	h := p.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status = %q, want ok", h.Status)
	}
}
