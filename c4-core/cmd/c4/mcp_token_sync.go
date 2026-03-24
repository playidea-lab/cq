package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mcpTokenRefreshInterval is how often we sync the token to .mcp.json.
// Set to 10 minutes — frequent enough to catch token rotations promptly,
// but skips writes when the token hasn't changed (cheap no-op).
const mcpTokenRefreshInterval = 10 * time.Minute

// mcpTokenMinRemaining skips syncing tokens that expire within this window.
// TokenProvider refreshes at 5 minutes before expiry, so if session.json
// has a token with <15 min left, it's about to be replaced — wait for the fresh one.
const mcpTokenMinRemaining = 15 * time.Minute

// startMCPTokenSync runs a background goroutine that periodically refreshes
// Bearer tokens in .mcp.json worker entries from ~/.c4/session.json.
// This keeps Claude Code's HTTP MCP connections alive across token rotations.
//
// It also does an immediate sync on startup.
func startMCPTokenSync(ctx context.Context, projectDir string) {
	// Immediate sync on startup
	if err := syncMCPTokens(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "cq: mcp token sync: %v\n", err)
	}

	go func() {
		ticker := time.NewTicker(mcpTokenRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := syncMCPTokens(projectDir); err != nil {
					fmt.Fprintf(os.Stderr, "cq: mcp token sync: %v\n", err)
				}
			}
		}
	}()
}

// syncMCPTokens reads the current access_token from ~/.c4/session.json and
// updates all worker-* entries in .mcp.json with the fresh token.
// Returns nil if no updates were needed or if relay workers are not configured.
func syncMCPTokens(projectDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	// Read current session token
	sessionPath := filepath.Join(home, ".c4", "session.json")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil // no session = nothing to sync
	}
	var session struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if json.Unmarshal(sessionData, &session) != nil || session.AccessToken == "" {
		return nil
	}

	// Skip if token is about to expire — TokenProvider will refresh it shortly,
	// and we'll pick up the fresh token on the next cycle.
	if session.ExpiresAt > 0 {
		remaining := time.Until(time.Unix(session.ExpiresAt, 0))
		if remaining < mcpTokenMinRemaining {
			fmt.Fprintf(os.Stderr, "cq: mcp token sync: skipped (token expires in %s, waiting for refresh)\n", remaining.Round(time.Second))
			return nil
		}
	}

	// Find all .mcp.json files to update (project dir + any other known locations)
	mcpPaths := findMCPJSONPaths(projectDir)
	if len(mcpPaths) == 0 {
		return nil
	}

	updated := 0
	for _, mcpPath := range mcpPaths {
		if n, err := updateMCPTokensInFile(mcpPath, session.AccessToken); err != nil {
			fmt.Fprintf(os.Stderr, "cq: mcp token sync %s: %v\n", mcpPath, err)
		} else {
			updated += n
		}
	}

	if updated > 0 {
		fmt.Fprintf(os.Stderr, "cq: mcp token sync: refreshed %d worker(s)\n", updated)
	}
	return nil
}

// updateMCPTokensInFile updates Bearer tokens for worker-* entries in a single .mcp.json file.
// Returns the number of entries updated.
func updateMCPTokensInFile(mcpPath, newToken string) (int, error) {
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		return 0, nil // file doesn't exist
	}

	var config map[string]any
	if json.Unmarshal(data, &config) != nil {
		return 0, nil // invalid JSON
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return 0, nil
	}

	updated := 0
	for key, val := range servers {
		if !strings.HasPrefix(key, "worker-") {
			continue
		}
		entry, ok := val.(map[string]any)
		if !ok {
			continue
		}
		// Only update HTTP type entries with relay URLs
		if t, _ := entry["type"].(string); t != "http" {
			continue
		}

		headers, ok := entry["headers"].(map[string]any)
		if !ok {
			continue
		}

		oldAuth, _ := headers["Authorization"].(string)
		newAuth := "Bearer " + newToken
		if oldAuth == newAuth {
			continue // already up to date
		}

		// Update token
		headers["Authorization"] = newAuth
		entry["headers"] = headers
		servers[key] = entry
		updated++
	}

	if updated == 0 {
		return 0, nil
	}

	config["mcpServers"] = servers
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(mcpPath, out, 0644); err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}

	return updated, nil
}

// findMCPJSONPaths returns all .mcp.json files that should have their tokens refreshed.
// Includes the current project dir and scans parent directories for monorepo setups.
func findMCPJSONPaths(projectDir string) []string {
	var paths []string
	seen := map[string]bool{}

	// 1. Current project .mcp.json
	p := filepath.Join(projectDir, ".mcp.json")
	if _, err := os.Stat(p); err == nil {
		paths = append(paths, p)
		seen[p] = true
	}

	// 2. Walk up to find parent .mcp.json (e.g., monorepo root)
	dir := projectDir
	for i := 0; i < 3; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		p := filepath.Join(dir, ".mcp.json")
		if _, err := os.Stat(p); err == nil && !seen[p] {
			paths = append(paths, p)
			seen[p] = true
		}
	}

	// 3. Home directory common locations
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, rel := range []string{
			".mcp.json",
			".cursor/mcp.json",
		} {
			p := filepath.Join(home, rel)
			if _, err := os.Stat(p); err == nil && !seen[p] {
				paths = append(paths, p)
				seen[p] = true
			}
		}
	}

	return paths
}
