// Package worker provides the job execution layer for C5 workers.
package worker

// WorkerConfig holds per-worker configuration that is passed into the job
// executor. Callers (typically cmd/c5/worker.go) populate this from caps.yaml
// and CLI flags; the executor reads it to decide whether to activate optional
// subsystems such as ExperimentWrapper.
type WorkerConfig struct {
	// ExperimentProtocol is parsed from caps.yaml experiment_protocol section.
	// When EpochPattern is non-empty and the job payload contains a non-empty
	// exp_run_id, the executor activates ExperimentWrapper for stdout.
	ExperimentProtocol *ExperimentProtocolConfig

	// MCPURL is the MCP server endpoint used by ExperimentWrapper checkpoint
	// calls. Typically the local cq serve MCP URL.
	MCPURL string
}
