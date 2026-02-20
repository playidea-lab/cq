package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// isUUID returns true if s looks like a UUID (8-4-4-4-12 hex pattern).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// resolveProjectUUID queries Supabase PostgREST to look up a project UUID by name.
func resolveProjectUUID(supabaseURL, anonKey, authToken, projectName string) (string, error) {
	url := supabaseURL + "/rest/v1/c4_projects?select=id&name=eq." + projectName + "&limit=1"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("project %q not found", projectName)
	}
	return rows[0].ID, nil
}

// writeSupabaseJSON writes ~/.c4/supabase.json so Rust c1 app can read Supabase credentials.
// This is a no-op if the file already exists with the same content.
func writeSupabaseJSON(supabaseURL, anonKey string) {
	if supabaseURL == "" || anonKey == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".c4")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	path := filepath.Join(dir, "supabase.json")
	content := fmt.Sprintf(`{"url":%q,"anon_key":%q}`, supabaseURL, anonKey)
	// Skip write if file already has the same content
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0600)
}
