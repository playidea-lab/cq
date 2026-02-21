package serve

// Status represents the lifecycle state of an Agent or component.
type Status = string

// Status constants for Agent lifecycle.
const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusDegraded Status = "degraded"
	StatusFailed   Status = "failed"
)
