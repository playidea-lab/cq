//go:build llm_gateway

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/secrets"
	"github.com/spf13/cobra"
)

var llmCmd = &cobra.Command{
	Use:   "llm [prompt]",
	Short: "Send a prompt to the LLM gateway",
	Long: `Send a prompt to the LLM gateway and print the response.

Uses the same provider routing as c4_llm_call MCP tool.
Connected tier users need only 'cq auth login' — no API key required.
Solo tier users need a local API key (cq secret set anthropic.api_key ...).

Examples:
  cq llm "Is this command safe: rm -rf /tmp/build"
  echo "Summarize this" | cq llm
  cq llm --model haiku --max-tokens 100 "Hello"
  cq llm --json "Respond with JSON: {allow: bool, reason: string}"`,
	RunE: runLLM,
}

func init() {
	llmCmd.Flags().String("model", "claude-haiku-4-5-20251001", "Model ID or alias")
	llmCmd.Flags().Int("max-tokens", 256, "Max output tokens")
	llmCmd.Flags().String("system", "", "System prompt")
	llmCmd.Flags().Bool("json", false, "Output full JSON response (model, usage, content)")
	rootCmd.AddCommand(llmCmd)
}

func runLLM(cmd *cobra.Command, args []string) error {
	// Collect prompt: args first, then stdin
	var prompt string
	if len(args) > 0 {
		prompt = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("no prompt provided (pass as argument or pipe to stdin)")
	}

	modelFlag, _ := cmd.Flags().GetString("model")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	system, _ := cmd.Flags().GetString("system")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	result, err := buildLLMGateway()
	if err != nil {
		return fmt.Errorf("initializing LLM gateway: %w", err)
	}
	gw := result.gateway

	// Route haiku models to cq-proxy or anthropic explicitly,
	// avoiding default provider mismatch (e.g. openai receiving haiku model ID).
	model := modelFlag
	if strings.Contains(model, "haiku") && !strings.Contains(model, "/") {
		if gw.HasProvider("cq-proxy") {
			model = "cq-proxy/" + model
		} else if gw.HasProvider("anthropic") {
			model = "anthropic/" + model
		}
	}

	req := &llm.ChatRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		System: system,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := gw.Chat(ctx, "", req)
	if err != nil {
		// Fallback: if cq-proxy failed, try direct Anthropic with local API key
		if strings.Contains(model, "cq-proxy/") {
			haikuModel := strings.TrimPrefix(model, "cq-proxy/")
			apiKey := result.anthropicKey
			if apiKey != "" {
				directProvider := llm.NewAnthropicProvider(apiKey, "")
				directReq := &llm.ChatRequest{
					Model:     haikuModel,
					MaxTokens: maxTokens,
					Messages:  req.Messages,
					System:    system,
				}
				resp, err = directProvider.Chat(ctx, directReq)
			}
		}
		if err != nil {
			return fmt.Errorf("llm call: %w", err)
		}
	}

	if jsonOutput {
		out := map[string]any{
			"content":       resp.Content,
			"model":         resp.Model,
			"finish_reason": resp.FinishReason,
			"usage": map[string]any{
				"input_tokens":  resp.Usage.InputTokens,
				"output_tokens": resp.Usage.OutputTokens,
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Print(resp.Content)
	return nil
}

type llmGatewayResult struct {
	gateway      *llm.Gateway
	anthropicKey string // resolved key for fallback
}

// buildLLMGateway creates a lightweight LLM Gateway without full MCP server init.
// Provider priority: cq-proxy (cloud JWT) > anthropic (local key) > others.
func buildLLMGateway() (*llmGatewayResult, error) {
	// Load config if available (best-effort)
	var cfgMgr *config.Manager
	if projectDir != "" {
		if mgr, err := config.New(projectDir); err == nil {
			cfgMgr = mgr
		}
	}

	// Open secret store (best-effort)
	var ss *secrets.Store
	if s, err := secrets.New(); err == nil {
		ss = s
	}

	// Resolve anthropic API key for fallback
	var anthropicKey string
	if ss != nil {
		anthropicKey, _ = ss.Get("anthropic.api_key")
	}
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	// Build cloud token provider (best-effort)
	var cloudTP *cloud.TokenProvider
	var supabaseURL, anonKey string
	if cfgMgr != nil {
		cfg := cfgMgr.GetConfig()
		supabaseURL = cfg.Cloud.URL
		anonKey = cfg.Cloud.AnonKey
	}
	if supabaseURL == "" {
		supabaseURL = readCloudURL(projectDir)
	}
	if anonKey == "" {
		anonKey = readCloudAnonKey(projectDir)
	}
	if supabaseURL != "" && anonKey != "" {
		authClient := cloud.NewAuthClient(supabaseURL, anonKey)
		if session, err := authClient.GetSession(); err == nil && session != nil {
			if session.ExpiresAt > 0 && time.Now().Unix() >= session.ExpiresAt {
				if refreshed, err := authClient.RefreshToken(); err == nil {
					session = refreshed
				}
			}
			cloudTP = cloud.NewTokenProvider(session.AccessToken, session.ExpiresAt, authClient)
		}
	}

	gwCfg := toLLMGatewayConfig(cfgMgr, ss, cloudTP)

	// If cq-proxy wasn't registered by toLLMGatewayConfig (builtinSupabaseURL empty)
	// but we have cloud config + valid token, register it manually.
	if _, hasProxy := gwCfg.Providers["cq-proxy"]; !hasProxy && cloudTP != nil && supabaseURL != "" {
		proxyBaseURL := strings.TrimRight(supabaseURL, "/") + "/functions/v1/llm-proxy"
		if cloudTP.Token() != "" {
			gwCfg.Providers["cq-proxy"] = llm.GatewayProviderConfig{
				Enabled:      true,
				TokenFunc:    cloudTP.Token,
				BaseURL:      proxyBaseURL,
				DefaultModel: "claude-haiku-4-5-20251001",
			}
		}
	}

	if len(gwCfg.Providers) == 0 {
		return nil, fmt.Errorf("no LLM providers available (run 'cq auth login' or 'cq secret set anthropic.api_key <key>')")
	}

	gw := llm.NewGatewayFromConfig(gwCfg)
	if gw.ProviderCount() == 0 {
		return nil, fmt.Errorf("no LLM providers enabled")
	}
	return &llmGatewayResult{
		gateway:      gw,
		anthropicKey: anthropicKey,
	}, nil
}
