// Package c2 implements the C2 document lifecycle in Go.
//
// Provides workspace management, profile YAML load/save, and persona learning
// without requiring the Python sidecar.
package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadProfile loads a c2 profile from YAML.
// If profilePath is empty, defaults to .c2/profile.yaml relative to cwd.
func LoadProfile(profilePath string) (map[string]any, error) {
	if profilePath == "" {
		profilePath = filepath.Join(".c2", "profile.yaml")
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read profile: %w", err)
	}

	var profile map[string]any
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	if profile == nil {
		return map[string]any{}, nil
	}
	return profile, nil
}

// SaveProfile saves profile data to YAML.
// If profilePath is empty, defaults to .c2/profile.yaml.
func SaveProfile(data map[string]any, profilePath string) error {
	if profilePath == "" {
		profilePath = filepath.Join(".c2", "profile.yaml")
	}

	if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	return os.WriteFile(profilePath, out, 0644)
}

// UpdateLearnedPatterns merges new patterns into the learned_patterns section.
func UpdateLearnedPatterns(profile map[string]any, tonePrefs, structPrefs, freqEdits []string) map[string]any {
	patterns, ok := profile["learned_patterns"].(map[string]any)
	if !ok {
		patterns = map[string]any{}
		profile["learned_patterns"] = patterns
	}

	if len(tonePrefs) > 0 {
		existing := toStringSlice(patterns["tone_preferences"])
		patterns["tone_preferences"] = mergeUnique(existing, tonePrefs)
	}

	if len(structPrefs) > 0 {
		existing := toStringSlice(patterns["structure_preferences"])
		patterns["structure_preferences"] = mergeUnique(existing, structPrefs)
	}

	if len(freqEdits) > 0 {
		existing := toStringSlice(patterns["frequent_edits"])
		patterns["frequent_edits"] = mergeUnique(existing, freqEdits)
	}

	patterns["last_updated"] = time.Now().Format("2006-01-02")
	return profile
}

// toStringSlice converts an any (typically []any from YAML) to []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

// mergeUnique adds items from b to a, skipping duplicates.
func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, s := range a {
		seen[s] = true
	}
	result := append([]string{}, a...)
	for _, s := range b {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}
	return result
}
