package eventbus

// JobSubmitSpec describes a job submission request for the Hub.
// This is a local type to avoid importing the hub package directly.
type JobSubmitSpec struct {
	Name        string            `json:"name"`
	Workdir     string            `json:"workdir"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	RequiresGPU bool              `json:"requires_gpu"`
	Priority    int               `json:"priority,omitempty"`
	ExpID       string            `json:"exp_id,omitempty"`
	Memo        string            `json:"memo,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
}

// JobSubmitResult describes the response from a job submission.
type JobSubmitResult struct {
	JobID string `json:"job_id"`
}

// JobSubmitter submits jobs to C5 Hub.
// Implementations must convert JobSubmitSpec to the hub-native request type.
type JobSubmitter interface {
	Submit(spec *JobSubmitSpec) (string, error)
}
