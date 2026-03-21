package observe

import "time"

// OnLLMCall implements llm.TraceHook for TraceCollector.
// It records an LLM step as a trace_step row. The sessionID is used as the
// traceID for best-effort attribution; if empty the call is a no-op.
// This method uses only primitive types to avoid a circular import of the llm package.
func (tc *TraceCollector) OnLLMCall(sessionID, taskType, provider, model string, inputTok, outputTok int, latencyMs int64, errMsg string, success bool) {
	if sessionID == "" {
		return
	}
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
