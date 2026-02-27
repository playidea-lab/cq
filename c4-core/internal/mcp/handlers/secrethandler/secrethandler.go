package secrethandler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/secrets"
)

// maxSecretValueBytes caps secret values to prevent misuse of the store as a data store.
const maxSecretValueBytes = 64 * 1024 // 64 KB

// maxSecretKeyBytes caps key names to prevent unbounded SQL parameter allocation.
const maxSecretKeyBytes = 256

// Register registers c4_secret_set, c4_secret_get, c4_secret_list, c4_secret_delete.
// store must not be nil.
func Register(reg *mcp.Registry, store *secrets.Store) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_secret_set",
		Description: "Store an encrypted secret in ~/.c4/secrets.db. Use for API keys and credentials instead of config.yaml.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Secret key (e.g. openai.api_key, anthropic.api_key)",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Secret value to encrypt and store",
				},
			},
			"required": []string{"key", "value"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.Key == "" {
			return nil, fmt.Errorf("key is required")
		}
		if p.Value == "" {
			return nil, fmt.Errorf("value is required")
		}
		if len(p.Key) > maxSecretKeyBytes {
			return nil, fmt.Errorf("key too long (max %d chars)", maxSecretKeyBytes)
		}
		if len(p.Value) > maxSecretValueBytes {
			return nil, fmt.Errorf("value exceeds maximum size of %d bytes", maxSecretValueBytes)
		}
		if err := store.Set(p.Key, p.Value); err != nil {
			return nil, err
		}
		return map[string]any{"key": p.Key, "status": "saved"}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_secret_get",
		Description: "Retrieve a secret from ~/.c4/secrets.db. WARNING: the plaintext value is returned in the response and will appear in the LLM context window. Prefer using c4_secret_set and relying on automatic key resolution in config rather than calling this tool directly.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Secret key to retrieve",
				},
			},
			"required": []string{"key"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.Key == "" {
			return nil, fmt.Errorf("key is required")
		}
		if len(p.Key) > maxSecretKeyBytes {
			return nil, fmt.Errorf("key too long (max %d chars)", maxSecretKeyBytes)
		}
		val, err := store.Get(p.Key)
		if errors.Is(err, secrets.ErrNotFound) {
			return nil, fmt.Errorf("secret %q not found", p.Key)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"key": p.Key, "value": val}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_secret_list",
		Description: "List all secret key names stored in ~/.c4/secrets.db (values not shown).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		keys, err := store.List()
		if err != nil {
			return nil, err
		}
		return map[string]any{"keys": keys, "count": len(keys)}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_secret_delete",
		Description: "Delete a secret from ~/.c4/secrets.db.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Secret key to delete",
				},
			},
			"required": []string{"key"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.Key == "" {
			return nil, fmt.Errorf("key is required")
		}
		if len(p.Key) > maxSecretKeyBytes {
			return nil, fmt.Errorf("key too long (max %d chars)", maxSecretKeyBytes)
		}
		if err := store.Delete(p.Key); errors.Is(err, secrets.ErrNotFound) {
			return nil, fmt.Errorf("secret %q not found", p.Key)
		} else if err != nil {
			return nil, err
		}
		return map[string]any{"key": p.Key, "status": "deleted"}, nil
	})
}
