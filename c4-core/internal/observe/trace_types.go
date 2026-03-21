package observe

import "time"

// StepType classifies each step recorded in a trace.
type StepType string

const (
	// StepTypeLLM represents an LLM inference call.
	StepTypeLLM StepType = "llm"
	// StepTypeTool represents an MCP tool invocation.
	StepTypeTool StepType = "tool"
	// StepTypeTask represents a task-level event (start, submit, etc.).
	StepTypeTask StepType = "task"
)

// TraceStep represents one row in the trace_steps table.
type TraceStep struct {
	ID        int64     `json:"id"`
	TraceID   string    `json:"trace_id"`
	StepType  StepType  `json:"step_type"`
	Timestamp time.Time `json:"ts"`

	// LLM fields (populated when StepType == StepTypeLLM)
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	TaskType   string  `json:"task_type,omitempty"`
	InputTok   int64   `json:"input_tok,omitempty"`
	OutputTok  int64   `json:"output_tok,omitempty"`
	LatencyMs  int64   `json:"latency_ms,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`

	// Tool fields (populated when StepType == StepTypeTool)
	ToolName string `json:"tool_name,omitempty"`

	// Common outcome fields
	Success  bool   `json:"success"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// TraceOutcome summarises the result of a complete trace.
// It is stored as JSON in traces.outcome_json.
type TraceOutcome struct {
	TotalInputTok  int64   `json:"total_input_tok"`
	TotalOutputTok int64   `json:"total_output_tok"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	TotalLatencyMs int64   `json:"total_latency_ms"`
	StepCount      int     `json:"step_count"`
	Success        bool    `json:"success"`
	ErrorMsg       string  `json:"error_msg,omitempty"`
}
