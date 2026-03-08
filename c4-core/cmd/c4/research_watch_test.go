//go:build research

package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestResearchWatch_MetricLayerFormat verifies val_loss history formatting.
func TestResearchWatch_MetricLayerFormat(t *testing.T) {
	data := &watchData{
		HypothesisID:   "hyp-001",
		Round:          3,
		Status:         "running",
		ValLossHistory: []float64{0.45, 0.42, 0.38},
		TestMetricHist: []float64{0.72, 0.75, 0.77},
		VerdictHistory: []string{"approved", "approved", "approved"},
	}

	out := captureWatchFrame(t, data)

	if !strings.Contains(out, "0.45") {
		t.Errorf("expected val_loss history to contain 0.45, got:\n%s", out)
	}
	if !strings.Contains(out, "0.38") {
		t.Errorf("expected val_loss history to contain 0.38, got:\n%s", out)
	}
	if !strings.Contains(out, "improving") {
		t.Errorf("expected 'improving' label in val_loss output, got:\n%s", out)
	}
	if !strings.Contains(out, "0.77") {
		t.Errorf("expected test_metric history to contain 0.77, got:\n%s", out)
	}
}

// TestResearchWatch_InterventionSignal_ValLoss verifies val_loss 3-consecutive degradation signal.
func TestResearchWatch_InterventionSignal_ValLoss(t *testing.T) {
	tests := []struct {
		name    string
		history []float64
		wantSig bool
	}{
		{
			name:    "3 consecutive increases → signal",
			history: []float64{0.30, 0.35, 0.40, 0.45},
			wantSig: true,
		},
		{
			name:    "only 2 consecutive increases → no signal",
			history: []float64{0.30, 0.35, 0.40},
			wantSig: false,
		},
		{
			name:    "improving → no signal",
			history: []float64{0.45, 0.42, 0.38},
			wantSig: false,
		},
		{
			name:    "single value → no signal",
			history: []float64{0.50},
			wantSig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &watchData{ValLossHistory: tt.history}
			sig := interventionSignal(data)
			hasSig := strings.Contains(sig, "val_loss")
			if hasSig != tt.wantSig {
				t.Errorf("interventionSignal() hasSig=%v, want %v (sig=%q)", hasSig, tt.wantSig, sig)
			}
		})
	}
}

// TestResearchWatch_InterventionSignal_NullResult verifies null_result consecutive signal.
func TestResearchWatch_InterventionSignal_NullResult(t *testing.T) {
	tests := []struct {
		name     string
		verdicts []string
		wantSig  bool
	}{
		{
			name:     "2 consecutive null_result → signal",
			verdicts: []string{"approved", "null_result", "null_result"},
			wantSig:  true,
		},
		{
			name:     "1 null_result → no signal",
			verdicts: []string{"approved", "null_result"},
			wantSig:  false,
		},
		{
			name:     "null_result not at end → no signal",
			verdicts: []string{"null_result", "null_result", "approved"},
			wantSig:  false,
		},
		{
			name:     "all approved → no signal",
			verdicts: []string{"approved", "approved"},
			wantSig:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &watchData{VerdictHistory: tt.verdicts}
			sig := interventionSignal(data)
			hasSig := strings.Contains(sig, "null_result")
			if hasSig != tt.wantSig {
				t.Errorf("interventionSignal() hasSig=%v, want %v (sig=%q)", hasSig, tt.wantSig, sig)
			}
		})
	}
}

// TestResearchWatch_IntervalDefault verifies watch command has default interval=10.
func TestResearchWatch_IntervalDefault(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if hasCmdPrefix(cmd.Use, "research") {
			for _, sub := range cmd.Commands() {
				if hasCmdPrefix(sub.Use, "watch") {
					iv, err := sub.Flags().GetInt("interval")
					if err != nil {
						t.Fatalf("--interval flag not found: %v", err)
					}
					if iv != 10 {
						t.Errorf("default interval = %d, want 10", iv)
					}
					return
				}
			}
		}
	}
	t.Error("'cq research watch' command not registered")
}

// captureWatchFrame renders a watchData frame to a string for assertions.
func captureWatchFrame(t *testing.T, data *watchData) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	printWatchFrame(w, data)
	w.Close()
	var buf strings.Builder
	io.Copy(&buf, r) //nolint:errcheck
	r.Close()
	return buf.String()
}
