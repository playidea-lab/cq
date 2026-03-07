package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/serve"
)

// compile-time: knowledgeHubPoller implements serve.Component
var _ serve.Component = (*knowledgeHubPoller)(nil)

func TestParseMetrics_KeyValue(t *testing.T) {
	lines := []string{"LOSS=0.42", "ACCURACY=0.98"}
	got := parseHubMetrics(lines)
	if got["LOSS"] != "0.42" {
		t.Errorf("LOSS = %q, want 0.42", got["LOSS"])
	}
	if got["ACCURACY"] != "0.98" {
		t.Errorf("ACCURACY = %q, want 0.98", got["ACCURACY"])
	}
}

func TestParseMetrics_EmptyLines(t *testing.T) {
	lines := []string{"", "   ", "no-equal-sign", "KEY=val"}
	got := parseHubMetrics(lines)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
	if got["KEY"] != "val" {
		t.Errorf("KEY = %q, want val", got["KEY"])
	}
}

func TestParseMetrics_DuplicateKeys_LastWins(t *testing.T) {
	lines := []string{"K=first", "K=second", "K=last"}
	got := parseHubMetrics(lines)
	if got["K"] != "last" {
		t.Errorf("K = %q, want last", got["K"])
	}
}

func TestParseMetrics_NoEquals(t *testing.T) {
	lines := []string{"justtext", "another line without equals"}
	got := parseHubMetrics(lines)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseMetrics_ValueContainsEquals(t *testing.T) {
	// First '=' is the split point; value may contain '='
	lines := []string{"FORMULA=a=b+c"}
	got := parseHubMetrics(lines)
	if got["FORMULA"] != "a=b+c" {
		t.Errorf("FORMULA = %q, want a=b+c", got["FORMULA"])
	}
}

func TestHubPoller_SeenIDs_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	seenPath := filepath.Join(dir, "seen.json")

	p := newKnowledgeHubPoller(knowledgeHubPollerConfig{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	// Initially empty
	loaded := p.loadSeenIDs()
	if len(loaded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(loaded))
	}

	// Save two entries
	m := map[string]seenEntry{
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
	raw := map[string]seenEntry{
		"old-job": {CompletedAt: stale},
		"new-job": {CompletedAt: fresh},
	}
	data, _ := json.Marshal(raw)
	os.WriteFile(seenPath, data, 0600) //nolint:errcheck

	p := newKnowledgeHubPoller(knowledgeHubPollerConfig{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	// Load and apply TTL cleanup via the extracted helper (same logic as poll()).
	seenIDs := p.loadSeenIDs()
	cleanupSeenIDs(seenIDs, time.Now().Add(-30*24*time.Hour))

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

	p := newKnowledgeHubPoller(knowledgeHubPollerConfig{
		SeenPath:     seenPath,
		PollInterval: 30 * time.Second,
	})

	m := map[string]seenEntry{"j": {CompletedAt: time.Now()}}
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
	p := newKnowledgeHubPoller(knowledgeHubPollerConfig{PollInterval: 30 * time.Second})
	if p.Name() != "hub-knowledge-poller" {
		t.Errorf("Name() = %q, want hub-knowledge-poller", p.Name())
	}
	h := p.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status = %q, want ok", h.Status)
	}
}
