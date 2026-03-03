package pop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGaugeTracker_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".c4", "pop", "gauge.json")

	g := NewGaugeTracker(path)

	// Load on missing file — should succeed with zero values.
	if err := g.Load(); err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if v := g.Get("merge_ambiguity"); v != 0 {
		t.Fatalf("expected 0 for unknown gauge, got %v", v)
	}

	// Set and save.
	g.Set("merge_ambiguity", 0.15)
	g.Set("avg_fan_out", 4.5)
	if err := g.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify persistence.
	g2 := NewGaugeTracker(path)
	if err := g2.Load(); err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if v := g2.Get("merge_ambiguity"); v != 0.15 {
		t.Fatalf("expected 0.15, got %v", v)
	}
	if v := g2.Get("avg_fan_out"); v != 4.5 {
		t.Fatalf("expected 4.5, got %v", v)
	}

	// Verify gauge.json was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("gauge.json not found: %v", err)
	}
}

func TestGaugeTracker_Thresholds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gauge.json")
	g := NewGaugeTracker(path)
	if err := g.Load(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		belowVal  float64
		aboveVal  float64
	}{
		{"merge_ambiguity", 0.10, 0.25},
		{"avg_fan_out", 2.0, 5.0},
		{"contradictions", 5.0, 15.0},
		{"temporal_queries", 3.0, 8.0},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_below", func(t *testing.T) {
			g.Set(tc.name, tc.belowVal)
			if g.ExceedsThreshold(tc.name) {
				t.Fatalf("%s=%.2f should NOT exceed threshold", tc.name, tc.belowVal)
			}
		})
		t.Run(tc.name+"_above", func(t *testing.T) {
			g.Set(tc.name, tc.aboveVal)
			if !g.ExceedsThreshold(tc.name) {
				t.Fatalf("%s=%.2f SHOULD exceed threshold", tc.name, tc.aboveVal)
			}
		})
	}

	// Unknown gauge name never exceeds threshold.
	g.Set("unknown_gauge", 9999)
	if g.ExceedsThreshold("unknown_gauge") {
		t.Fatal("unknown gauge should never exceed threshold")
	}
}
