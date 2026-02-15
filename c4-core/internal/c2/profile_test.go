package c2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile_NotFound(t *testing.T) {
	profile, err := LoadProfile("/nonexistent/profile.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profile) != 0 {
		t.Errorf("expected empty map, got %v", profile)
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".c2", "profile.yaml")

	data := map[string]any{
		"preferences": map[string]any{
			"write": map[string]any{
				"language": "ko",
			},
		},
	}

	if err := SaveProfile(data, path); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	prefs, ok := loaded["preferences"].(map[string]any)
	if !ok {
		t.Fatalf("preferences not found")
	}
	write, ok := prefs["write"].(map[string]any)
	if !ok {
		t.Fatalf("write preferences not found")
	}
	if write["language"] != "ko" {
		t.Errorf("language = %v, want ko", write["language"])
	}
}

func TestSaveProfile_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "profile.yaml")

	err := SaveProfile(map[string]any{"test": true}, path)
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file not created")
	}
}

func TestUpdateLearnedPatterns(t *testing.T) {
	profile := map[string]any{}

	UpdateLearnedPatterns(profile, []string{"soft tone"}, []string{"concise"}, nil)

	patterns, ok := profile["learned_patterns"].(map[string]any)
	if !ok {
		t.Fatal("learned_patterns not found")
	}

	tones := toStringSlice(patterns["tone_preferences"])
	if len(tones) != 1 || tones[0] != "soft tone" {
		t.Errorf("tone_preferences = %v, want [soft tone]", tones)
	}

	// Add more — no duplicates
	UpdateLearnedPatterns(profile, []string{"soft tone", "formal"}, nil, nil)
	tones = toStringSlice(patterns["tone_preferences"])
	if len(tones) != 2 {
		t.Errorf("tone_preferences len = %d, want 2", len(tones))
	}
}

func TestMergeUnique(t *testing.T) {
	result := mergeUnique([]string{"a", "b"}, []string{"b", "c"})
	if len(result) != 3 {
		t.Errorf("len = %d, want 3", len(result))
	}
}
