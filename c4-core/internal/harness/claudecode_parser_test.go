package harness

import (
	"encoding/json"
	"testing"
)

func TestExtractUsage(t *testing.T) {
	t.Run("assistant with usage returns info", func(t *testing.T) {
		line := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-3-5-sonnet-20241022",
				"usage": map[string]any{
					"input_tokens":               100,
					"output_tokens":              50,
					"cache_read_input_tokens":    10,
					"cache_creation_input_tokens": 5,
				},
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("expected non-nil LLMUsageInfo")
		}
		if info.Model != "claude-3-5-sonnet-20241022" {
			t.Errorf("model = %q, want claude-3-5-sonnet-20241022", info.Model)
		}
		if info.Provider != "anthropic" {
			t.Errorf("provider = %q, want anthropic", info.Provider)
		}
		if info.InputTok != 100 {
			t.Errorf("InputTok = %d, want 100", info.InputTok)
		}
		if info.OutputTok != 50 {
			t.Errorf("OutputTok = %d, want 50", info.OutputTok)
		}
		if info.CacheRead != 10 {
			t.Errorf("CacheRead = %d, want 10", info.CacheRead)
		}
		if info.CacheWrite != 5 {
			t.Errorf("CacheWrite = %d, want 5", info.CacheWrite)
		}
	})

	t.Run("user type returns nil", func(t *testing.T) {
		line := map[string]any{
			"type": "user",
			"message": map[string]any{
				"content": "hello",
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info != nil {
			t.Fatalf("expected nil, got %+v", info)
		}
	})

	t.Run("assistant without usage returns nil", func(t *testing.T) {
		line := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"model":   "claude-3-5-sonnet-20241022",
				"content": "hi",
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info != nil {
			t.Fatalf("expected nil, got %+v", info)
		}
	})

	t.Run("gpt model maps to openai provider", func(t *testing.T) {
		line := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"model": "gpt-4o",
				"usage": map[string]any{
					"input_tokens":  20,
					"output_tokens": 10,
				},
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("expected non-nil LLMUsageInfo")
		}
		if info.Provider != "openai" {
			t.Errorf("provider = %q, want openai", info.Provider)
		}
	})

	t.Run("gemini model maps to google provider", func(t *testing.T) {
		line := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"model": "gemini-1.5-pro",
				"usage": map[string]any{
					"input_tokens":  30,
					"output_tokens": 15,
				},
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("expected non-nil LLMUsageInfo")
		}
		if info.Provider != "google" {
			t.Errorf("provider = %q, want google", info.Provider)
		}
	})

	t.Run("unknown model maps to unknown provider", func(t *testing.T) {
		line := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"model": "llama-3-70b",
				"usage": map[string]any{
					"input_tokens":  5,
					"output_tokens": 5,
				},
			},
		}
		data, _ := json.Marshal(line)
		info, err := ExtractUsage(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("expected non-nil LLMUsageInfo")
		}
		if info.Provider != "unknown" {
			t.Errorf("provider = %q, want unknown", info.Provider)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		_, err := ExtractUsage([]byte("not-json"))
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
