package observe

import "time"

// OnLLMCall implements llm.TraceHook for TraceCollector.
// It auto-creates an ephemeral trace (if none exists for this session) and
// records an LLM step. Uses sessionID as trace grouping key; falls back to
// "unattributed" when empty. Primitive types only to avoid circular imports.
func (tc *TraceCollector) OnLLMCall(sessionID, taskType, provider, model string, inputTok, outputTok int, latencyMs int64, errMsg string, success bool) {
	if sessionID == "" {
		sessionID = "unattributed"
	}
	// Ensure a parent trace row exists for this sessionID.
	tc.ensureTrace(sessionID)
	tc.AddStep(sessionID, TraceStep{
		StepType:  StepTypeLLM,
		Timestamp: time.Now(),
		Provider:  provider,
		Model:     model,
		TaskType:  taskType,
		InputTok:  int64(inputTok),
		OutputTok: int64(outputTok),
		LatencyMs: latencyMs,
		Success:   success,
		ErrorMsg:  errMsg,
	})
}
