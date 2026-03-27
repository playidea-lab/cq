//go:build !llm_gateway

package main

// captureSessionSummarizeFn remains nil when LLM gateway is not built in.
// captureSession will skip summarization silently.
