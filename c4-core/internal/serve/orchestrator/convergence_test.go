//go:build research

package orchestrator

import (
	"math"
	"testing"
)

func TestConvergenceChecker_Improvement(t *testing.T) {
	c := &ConvergenceChecker{Threshold: 0.5, Patience: 3, LowerIsBetter: true}
	s := &LoopSession{BestMetric: math.Inf(1)}

	// First metric: always sets BestMetric.
	r := c.Check(s, 100.0)
	if r.Converged {
		t.Error("should not converge on first check")
	}
	if s.BestMetric != 100.0 {
		t.Errorf("BestMetric = %v, want 100.0", s.BestMetric)
	}

	// Improvement >= threshold: reset patience.
	r = c.Check(s, 99.0)
	if r.PatienceCount != 0 {
		t.Errorf("PatienceCount = %d, want 0 after improvement", r.PatienceCount)
	}
	if s.BestMetric != 99.0 {
		t.Errorf("BestMetric = %v, want 99.0", s.BestMetric)
	}
}

func TestConvergenceChecker_NoImprovement(t *testing.T) {
	c := &ConvergenceChecker{Threshold: 0.5, Patience: 2, LowerIsBetter: true}
	s := &LoopSession{BestMetric: 100.0}

	// No improvement (diff < threshold).
	c.Check(s, 99.8)
	if s.PatienceCount != 1 {
		t.Errorf("PatienceCount = %d, want 1", s.PatienceCount)
	}

	// Still no improvement → converged.
	r := c.Check(s, 99.9)
	if !r.Converged {
		t.Error("should converge after patience exceeded")
	}
	if r.Reason != "patience_exceeded" {
		t.Errorf("Reason = %q, want patience_exceeded", r.Reason)
	}
}

func TestConvergenceChecker_MaxRounds(t *testing.T) {
	c := &ConvergenceChecker{Threshold: 0.5, Patience: 10, MaxRounds: 3, LowerIsBetter: true}
	s := &LoopSession{BestMetric: 100.0, Round: 3}

	r := c.Check(s, 99.0)
	if !r.Converged {
		t.Error("should converge at max rounds")
	}
	if r.Reason != "max_rounds" {
		t.Errorf("Reason = %q, want max_rounds", r.Reason)
	}
}

func TestConvergenceChecker_HigherIsBetter(t *testing.T) {
	c := &ConvergenceChecker{Threshold: 1.0, Patience: 2, LowerIsBetter: false}
	s := &LoopSession{BestMetric: math.Inf(-1)}

	// First metric.
	c.Check(s, 80.0)
	if s.BestMetric != 80.0 {
		t.Errorf("BestMetric = %v, want 80.0", s.BestMetric)
	}

	// Improvement.
	r := c.Check(s, 82.0)
	if r.PatienceCount != 0 {
		t.Errorf("PatienceCount = %d, want 0", r.PatienceCount)
	}

	// No improvement.
	c.Check(s, 82.5) // diff=0.5 < threshold=1.0
	if s.PatienceCount != 1 {
		t.Errorf("PatienceCount = %d, want 1", s.PatienceCount)
	}
}

func TestConvergenceChecker_Disabled(t *testing.T) {
	c := &ConvergenceChecker{Patience: 0, MaxRounds: 0}
	s := &LoopSession{BestMetric: 100.0}

	r := c.Check(s, 99.0)
	if r.Converged {
		t.Error("should not converge when disabled")
	}
}
