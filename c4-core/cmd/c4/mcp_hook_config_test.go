package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/config"
)

func TestWriteHookConfigJSON(t *testing.T) {
	t.Run("nil cfg writes defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeHookConfigJSON(dir, nil)

		path := filepath.Join(dir, ".c4", "hook-config.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}

		var got hookConfigJSON
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if got.Enabled {
			t.Error("expected enabled=false for nil cfg")
		}
		if got.Mode != "hook" {
			t.Errorf("expected mode=hook, got %q", got.Mode)
		}
		if !got.AutoApprove {
			t.Error("expected auto_approve=true for nil cfg")
		}
		if got.Model != "claude-haiku-4-5-20251001" {
			t.Errorf("expected default model, got %q", got.Model)
		}
		if got.APIKeyEnv != "ANTHROPIC_API_KEY" {
			t.Errorf("expected ANTHROPIC_API_KEY, got %q", got.APIKeyEnv)
		}
		if got.Timeout != 10 {
			t.Errorf("expected timeout=10, got %d", got.Timeout)
		}
		if got.AllowPatterns == nil || len(got.AllowPatterns) != 0 {
			t.Error("expected allow_patterns=[]")
		}
		if got.BlockPatterns == nil || len(got.BlockPatterns) != 0 {
			t.Error("expected block_patterns=[]")
		}
	})

	t.Run("cfg with enabled reviewer", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.C4Config{
			PermissionReviewer: config.PermissionReviewerConfig{
				Enabled:   true,
				Model:     "sonnet",
				APIKeyEnv: "MY_API_KEY",
				FailMode:  "allow",
				Timeout:   30,
			},
		}
		writeHookConfigJSON(dir, cfg)

		path := filepath.Join(dir, ".c4", "hook-config.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}

		var got hookConfigJSON
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if !got.Enabled {
			t.Error("expected enabled=true")
		}
		if !got.AutoApprove {
			t.Error("expected auto_approve=true when fail_mode=allow")
		}
		if got.Model != "claude-sonnet-4-5-20251001" {
			t.Errorf("expected resolved sonnet model, got %q", got.Model)
		}
		if got.APIKeyEnv != "MY_API_KEY" {
			t.Errorf("expected MY_API_KEY, got %q", got.APIKeyEnv)
		}
		if got.Timeout != 30 {
			t.Errorf("expected timeout=30, got %d", got.Timeout)
		}
	})

	t.Run("no rewrite when content identical", func(t *testing.T) {
		dir := t.TempDir()
		// First write
		writeHookConfigJSON(dir, nil)

		path := filepath.Join(dir, ".c4", "hook-config.json")
		info1, err := os.Stat(path)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}

		// Second write with same config — should not modify the file
		writeHookConfigJSON(dir, nil)

		info2, err := os.Stat(path)
		if err != nil {
			t.Fatalf("file missing after second write: %v", err)
		}

		if info1.ModTime() != info2.ModTime() {
			t.Error("file was rewritten despite identical content")
		}
	})

	t.Run("rewrite when content changes", func(t *testing.T) {
		dir := t.TempDir()
		// First write with nil (defaults)
		writeHookConfigJSON(dir, nil)

		path := filepath.Join(dir, ".c4", "hook-config.json")
		info1, err := os.Stat(path)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}

		// Wait a moment to ensure mtime differs if written
		// Second write with different config
		cfg := &config.C4Config{
			PermissionReviewer: config.PermissionReviewerConfig{
				Enabled: true,
				Model:   "haiku",
			},
		}
		writeHookConfigJSON(dir, cfg)

		info2, err := os.Stat(path)
		if err != nil {
			t.Fatalf("file missing: %v", err)
		}

		// Content differs (enabled changed), so file should be rewritten
		data, _ := os.ReadFile(path)
		var got hookConfigJSON
		json.Unmarshal(data, &got)
		if !got.Enabled {
			t.Error("expected enabled=true after second write")
		}
		_ = info1
		_ = info2
	})

	t.Run("model alias resolution", func(t *testing.T) {
		cases := []struct {
			alias    string
			expected string
		}{
			{"haiku", "claude-haiku-4-5-20251001"},
			{"", "claude-haiku-4-5-20251001"},
			{"sonnet", "claude-sonnet-4-5-20251001"},
			{"opus", "claude-opus-4-5-20241101"},
			{"claude-custom-model", "claude-custom-model"},
		}
		for _, tc := range cases {
			got := resolveHookModel(tc.alias)
			if got != tc.expected {
				t.Errorf("resolveHookModel(%q) = %q, want %q", tc.alias, got, tc.expected)
			}
		}
	})
}
