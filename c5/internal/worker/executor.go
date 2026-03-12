package worker

import (
	"context"
	"io"
	"log"
)

// JobPayload holds the experiment-related fields injected into a job by the
// CLI (T-1204-0). Workers extract these from model.Job.Params at runtime.
type JobPayload struct {
	// ExpRunID is the experiment run identifier returned by Hub POST /v1/experiment/run.
	// Empty string means no experiment run is associated with this job.
	ExpRunID string

	// ExpID is the experiment identifier (distinct from ExpRunID which is a single run).
	// Passed as "exp_id" in MCP checkpoint calls for downstream tracking.
	ExpID string
}

// ExecuteWithExperiment wraps src (typically the stdout pipe of a running job)
// and writes all output to dst. If the WorkerConfig has a non-nil
// ExperimentProtocol AND payload.ExpRunID is non-empty, an ExperimentWrapper
// is activated to call MCP checkpoints on @key=value matches in stdout.
// Otherwise output is copied verbatim — behaviour identical to io.Copy.
//
// Returns the first error encountered by the wrapper or the plain copy.
func ExecuteWithExperiment(ctx context.Context, cfg *WorkerConfig, payload JobPayload, src io.Reader, dst io.Writer) error {
	if cfg == nil || cfg.ExperimentProtocol == nil {
		// No experiment protocol configured — pass-through.
		_, err := io.Copy(dst, src)
		return err
	}

	if payload.ExpRunID == "" {
		// protocol present but no run_id in job payload — skip wrapper, warn.
		log.Printf("executor: experiment_protocol configured but exp_run_id is empty — skipping ExperimentWrapper")
		_, err := io.Copy(dst, src)
		return err
	}

	wrapper, err := NewExperimentWrapper(cfg.MCPURL, payload.ExpID, payload.ExpRunID, cfg.ExperimentProtocol)
	if err != nil {
		// Pattern compile error — fall back to plain copy so the job is not broken.
		log.Printf("executor: failed to create ExperimentWrapper: %v — falling back to plain copy", err)
		_, err2 := io.Copy(dst, src)
		return err2
	}

	return wrapper.WrapOutput(ctx, src, dst)
}
