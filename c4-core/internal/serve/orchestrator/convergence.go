//go:build research

package orchestrator

import "math"

// ConvergenceChecker determines whether a research loop has converged
// based on metric improvement and patience.
type ConvergenceChecker struct {
	Threshold     float64 // minimum improvement to reset patience
	Patience      int     // max rounds without improvement; 0 = disabled
	LowerIsBetter bool    // true: lower metric is better (e.g. loss)
	MaxRounds     int     // hard limit; 0 = unlimited
}

// ConvergenceResult holds the outcome of a convergence check.
type ConvergenceResult struct {
	Converged     bool
	Reason        string // "patience_exceeded" | "max_rounds" | ""
	PatienceCount int
	BestMetric    float64
}

// Check evaluates whether the session has converged given a new metric value.
// It updates session.BestMetric and session.PatienceCount in place.
func (c *ConvergenceChecker) Check(session *LoopSession, newMetric float64) ConvergenceResult {
	if c.Patience <= 0 && c.MaxRounds <= 0 {
		return ConvergenceResult{}
	}

	// Determine if the new metric is an improvement.
	improved := false
	if c.LowerIsBetter {
		if session.BestMetric-newMetric >= c.Threshold {
			improved = true
		}
	} else {
		if newMetric-session.BestMetric >= c.Threshold {
			improved = true
		}
	}

	if improved {
		session.BestMetric = newMetric
		session.PatienceCount = 0
	} else {
		// First round: initialize BestMetric if still at Inf.
		if math.IsInf(session.BestMetric, 0) {
			session.BestMetric = newMetric
		}
		session.PatienceCount++
	}

	result := ConvergenceResult{
		PatienceCount: session.PatienceCount,
		BestMetric:    session.BestMetric,
	}

	// Patience check.
	if c.Patience > 0 && session.PatienceCount >= c.Patience {
		result.Converged = true
		result.Reason = "patience_exceeded"
		return result
	}

	// Max rounds check.
	if c.MaxRounds > 0 && session.Round >= c.MaxRounds {
		result.Converged = true
		result.Reason = "max_rounds"
		return result
	}

	return result
}
