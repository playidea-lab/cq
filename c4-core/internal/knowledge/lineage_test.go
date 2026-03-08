package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDebateDoc writes a debate markdown file directly to the store's docs dir
// so custom frontmatter fields (hypothesis_id, verdict, val_loss, test_metric) are preserved.
// round is used to generate a unique updated_at timestamp so List ordering is deterministic.
func writeDebateDoc(t *testing.T, s *Store, id, hypothesisID, verdict string, round int, valLoss, testMetric float64) {
	t.Helper()
	// Use round to create distinct timestamps: higher round = more recent.
	updatedAt := fmt.Sprintf("2024-01-%02dT00:00:00Z", round)
	content := fmt.Sprintf(`---
id: %s
type: debate
title: Debate %s
hypothesis_id: %s
verdict: %s
round: %d
val_loss: %g
test_metric: %g
created_at: 2024-01-01T00:00:00Z
updated_at: %s
version: 1
---

Debate body.
`, id, id, hypothesisID, verdict, round, valLoss, testMetric, updatedAt)
	path := filepath.Join(s.DocsDir(), id+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeDebateDoc %s: %v", id, err)
	}
	if _, err := s.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
}

func TestLineageBuildContext_Empty(t *testing.T) {
	s := setupTestStore(t)
	lb := NewLineageBuilder(s)

	got, err := lb.BuildContext(context.Background(), "hyp-abc", 5)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if got != "## Lineage Context\n" {
		t.Errorf("empty: got %q, want %q", got, "## Lineage Context\n")
	}
}

func TestLineageBuildContext_Limit(t *testing.T) {
	s := setupTestStore(t)
	lb := NewLineageBuilder(s)

	for i := 1; i <= 7; i++ {
		writeDebateDoc(t, s, fmt.Sprintf("deb-lim%d", i), "hyp-111", "confirmed", i, float64(i)*0.1, float64(i)*0.01)
	}

	got, err := lb.BuildContext(context.Background(), "hyp-111", 5)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	count := strings.Count(got, "- round ")
	if count != 5 {
		t.Errorf("limit: got %d entries, want 5\noutput:\n%s", count, got)
	}
}

func TestLineageBuildContext_FrontmatterParsing(t *testing.T) {
	s := setupTestStore(t)
	lb := NewLineageBuilder(s)

	writeDebateDoc(t, s, "deb-fp1", "hyp-222", "confirmed", 3, 0.25, 0.88)

	got, err := lb.BuildContext(context.Background(), "hyp-222", 5)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	for _, want := range []string{"round 3", "val_loss=0.25", "test_metric=0.88", "verdict=confirmed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestLineageBuildContext_NullResultCount(t *testing.T) {
	s := setupTestStore(t)
	lb := NewLineageBuilder(s)

	writeDebateDoc(t, s, "deb-nr1", "hyp-333", "confirmed", 1, 0.5, 0.7)
	writeDebateDoc(t, s, "deb-nr2", "hyp-333", "null_result", 2, 0.5, 0.7)
	writeDebateDoc(t, s, "deb-nr3", "hyp-333", "null_result", 3, 0.5, 0.7)

	got, err := lb.BuildContext(context.Background(), "hyp-333", 5)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	// List returns updated_at DESC → [nr3(null), nr2(null), nr1(confirmed)]
	// Consecutive null_result from most recent (index 0): nr3+nr2 = 2.
	if !strings.Contains(got, "연속 null_result: 2회") {
		t.Errorf("null_result count want 2회, got:\n%s", got)
	}
}

func TestLineageBuildContext_DefaultLimit(t *testing.T) {
	s := setupTestStore(t)
	lb := NewLineageBuilder(s)

	for i := 1; i <= 7; i++ {
		writeDebateDoc(t, s, fmt.Sprintf("deb-dl%d", i), "hyp-444", "confirmed", i, 0.1, 0.9)
	}

	got, err := lb.BuildContext(context.Background(), "hyp-444", 0)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	count := strings.Count(got, "- round ")
	if count != 5 {
		t.Errorf("default limit: got %d entries, want 5\noutput:\n%s", count, got)
	}
}
