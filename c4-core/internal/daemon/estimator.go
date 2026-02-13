package daemon

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// EstimateResult holds the time estimation for a job.
type EstimateResult struct {
	EstimatedDurationSec float64 `json:"estimated_duration_sec"`
	QueueWaitSec         float64 `json:"queue_wait_sec,omitempty"`
	Confidence           float64 `json:"confidence"`
	Method               string  `json:"method"` // historical, similar_jobs, global_avg, default
}

// Estimator provides job duration estimates based on historical data.
type Estimator struct {
	store *Store
}

// NewEstimator creates an estimator backed by a store.
func NewEstimator(store *Store) *Estimator {
	return &Estimator{store: store}
}

// Estimate returns a duration estimate for a job using a fallback chain:
// 1. historical — exact command hash match with >=3 samples (confidence 0.8)
// 2. similar_jobs — exact hash with 1-2 samples (confidence 0.5)
// 3. global_avg — average of all recorded durations (confidence 0.2)
// 4. default — 300s fallback (confidence 0.1)
func (e *Estimator) Estimate(job *Job) *EstimateResult {
	hash := NormalizeCommandHash(job.Command)

	// 1. Try exact hash match
	durations, _ := e.store.GetDurations(hash, 20)
	if len(durations) >= 3 {
		return &EstimateResult{
			EstimatedDurationSec: avg(durations),
			Confidence:           0.8,
			Method:               "historical",
		}
	}
	if len(durations) > 0 {
		return &EstimateResult{
			EstimatedDurationSec: avg(durations),
			Confidence:           0.5,
			Method:               "similar_jobs",
		}
	}

	// 2. Try global average
	globalDurations, _ := e.store.GetGlobalDurations(20)
	if len(globalDurations) > 0 {
		return &EstimateResult{
			EstimatedDurationSec: avg(globalDurations),
			Confidence:           0.2,
			Method:               "global_avg",
		}
	}

	// 3. Default fallback
	return &EstimateResult{
		EstimatedDurationSec: 300,
		Confidence:           0.1,
		Method:               "default",
	}
}

// EstimateWithQueue adds queue wait time to the estimate.
func (e *Estimator) EstimateWithQueue(job *Job) *EstimateResult {
	result := e.Estimate(job)

	if job.Status == StatusQueued {
		queuedCount, _ := e.store.CountByStatus(StatusQueued)
		if queuedCount > 0 {
			result.QueueWaitSec = float64(queuedCount) * result.EstimatedDurationSec / 2
		}
	}

	return result
}

// NormalizeCommandHash normalizes a command string and returns its hash.
// Removes variable parts (timestamps, seeds, temp paths, run IDs) to match
// similar commands that differ only in ephemeral parameters.
func NormalizeCommandHash(command string) string {
	normalized := normalizeCommand(command)
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:8])
}

// normalizeCommand strips variable parts from a command for hashing.
var (
	// Match common variable patterns
	seedPattern      = regexp.MustCompile(`--seed[= ]\d+`)
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T_ ]\d{2}[:\-]\d{2}`)
	runIDPattern     = regexp.MustCompile(`(?:run|exp|job)[_-]\w{6,}`)
	tmpPathPattern   = regexp.MustCompile(`/tmp/\S+`)
	epochPattern     = regexp.MustCompile(`\b1[6-9]\d{8,9}\b`) // unix timestamps
)

func normalizeCommand(cmd string) string {
	cmd = seedPattern.ReplaceAllString(cmd, "--seed SEED")
	cmd = timestampPattern.ReplaceAllString(cmd, "TIMESTAMP")
	cmd = runIDPattern.ReplaceAllString(cmd, "RUN_ID")
	cmd = tmpPathPattern.ReplaceAllString(cmd, "/tmp/TMPPATH")
	cmd = epochPattern.ReplaceAllString(cmd, "EPOCH")

	// Collapse whitespace
	cmd = strings.Join(strings.Fields(cmd), " ")
	return cmd
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
