package api

import (
	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// Estimator provides job duration estimates based on historical data.
type Estimator struct {
	store *store.Store
}

// NewEstimator creates an estimator backed by a store.
func NewEstimator(s *store.Store) *Estimator {
	return &Estimator{store: s}
}

// Estimate returns a duration estimate using a fallback chain:
// 1. historical — exact command hash with >=3 samples (confidence 0.8)
// 2. similar_jobs — exact hash with 1-2 samples (confidence 0.5)
// 3. global_avg — average of all recorded durations (confidence 0.2)
// 4. default — 300s fallback (confidence 0.1)
func (e *Estimator) Estimate(job *model.Job) *model.EstimateResponse {
	hash := job.CommandHash()

	durations, _ := e.store.GetDurations(hash, 20)
	if len(durations) >= 3 {
		return &model.EstimateResponse{
			EstimatedDurationSec: avg(durations),
			Confidence:           0.8,
			Method:               "historical",
		}
	}
	if len(durations) > 0 {
		return &model.EstimateResponse{
			EstimatedDurationSec: avg(durations),
			Confidence:           0.5,
			Method:               "similar_jobs",
		}
	}

	globalDurations, _ := e.store.GetGlobalDurations(20)
	if len(globalDurations) > 0 {
		return &model.EstimateResponse{
			EstimatedDurationSec: avg(globalDurations),
			Confidence:           0.2,
			Method:               "global_avg",
		}
	}

	return &model.EstimateResponse{
		EstimatedDurationSec: 300,
		Confidence:           0.1,
		Method:               "default",
	}
}

// EstimateWithQueue adds queue wait time to the estimate.
func (e *Estimator) EstimateWithQueue(job *model.Job) *model.EstimateResponse {
	result := e.Estimate(job)

	if job.Status == model.StatusQueued {
		queuedCount, _ := e.store.CountByStatus(model.StatusQueued)
		if queuedCount > 0 {
			result.QueueWaitSec = float64(queuedCount) * result.EstimatedDurationSec / 2
		}
	}

	return result
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
