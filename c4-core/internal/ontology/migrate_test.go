package ontology

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeRawPatterns writes patterns to a temp raw_patterns.json and returns
// a stub username that resolves to the temp dir via HOME override.
func writeRawPatternsFile(t *testing.T, patterns []rawPattern) (username string, cleanup func()) {
	t.Helper()

	tmpHome := t.TempDir()
	username = "testuser"
	dir := filepath.Join(tmpHome, ".c4", "personas", username)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create dir: %v", err)
	}

	data, err := json.Marshal(patterns)
	if err != nil {
		t.Fatalf("marshal patterns: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw_patterns.json"), data, 0644); err != nil {
		t.Fatalf("write raw_patterns.json: %v", err)
	}

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	cleanup = func() { os.Setenv("HOME", orig) }
	return username, cleanup
}

func TestMigrateRawPatterns_CoreAxes(t *testing.T) {
	patterns := []rawPattern{
		{Category: "addition", Description: "added something", Frequency: 2, Examples: []string{"ex1"}},
		{Category: "deletion", Description: "removed content", Frequency: 1},
		{Category: "structure", Description: "restructured text", Frequency: 3},
		{Category: "wording", Description: "changed wording", Frequency: 1},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	dst := &Ontology{Version: defaultVersion}
	count, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 patterns migrated, got %d", count)
	}

	// Core axes must map to behavior/* paths.
	for cat, path := range coreCategories {
		if _, ok := dst.Schema.Nodes[path]; !ok {
			t.Errorf("expected node at %q for category %q", path, cat)
		}
	}
}

func TestMigrateRawPatterns_ExtendedNode(t *testing.T) {
	patterns := []rawPattern{
		{Category: "unknown_cat", Description: "some unknown category", Frequency: 1},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	dst := &Ontology{Version: defaultVersion}
	_, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}

	if _, ok := dst.Schema.Nodes["extended/unknown_cat"]; !ok {
		t.Error("expected extended node for unknown category")
	}
}

func TestMigrateRawPatterns_PreservesExistingNodes(t *testing.T) {
	patterns := []rawPattern{
		{Category: "addition", Description: "added content", Frequency: 1},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	// Pre-populate dst with an existing node.
	dst := &Ontology{Version: defaultVersion}
	u := NewUpdater(dst)
	u.AddOrUpdate("custom/node", Node{Label: "Custom", Description: "pre-existing"})

	_, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}

	// Pre-existing node must still be there.
	if _, ok := dst.Schema.Nodes["custom/node"]; !ok {
		t.Error("pre-existing node was removed by migration")
	}
	// Migration node must also exist.
	if _, ok := dst.Schema.Nodes["behavior/addition"]; !ok {
		t.Error("migrated node not found")
	}
}

func TestMigrateRawPatterns_FrequencyMerge(t *testing.T) {
	// Two patterns with the same category merge into one node.
	patterns := []rawPattern{
		{Category: "addition", Description: "first addition", Frequency: 1},
		{Category: "addition", Description: "second addition", Frequency: 2},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	dst := &Ontology{Version: defaultVersion}
	count, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count=2 (raw patterns processed), got %d", count)
	}

	// Both patterns share the same node path; after merge frequency > 1.
	node, ok := dst.Schema.Nodes["behavior/addition"]
	if !ok {
		t.Fatal("expected behavior/addition node")
	}
	if node.Frequency < 2 {
		t.Errorf("expected merged frequency >= 2, got %d", node.Frequency)
	}
}

func TestMigrateRawPatterns_MissingFile_NoError(t *testing.T) {
	tmpHome := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", orig)

	dst := &Ontology{Version: defaultVersion}
	count, err := MigrateRawPatterns("nouser", dst)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count=0 for missing file, got %d", count)
	}
}

func TestMigrateRawPatterns_ExampleStoredAsProperty(t *testing.T) {
	patterns := []rawPattern{
		{Category: "wording", Description: "changed phrasing", Frequency: 1, Examples: []string{"old → new"}},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	dst := &Ontology{Version: defaultVersion}
	_, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}

	node := dst.Schema.Nodes["behavior/wording"]
	if node.Properties["example"] != "old → new" {
		t.Errorf("expected example property 'old → new', got %q", node.Properties["example"])
	}
}

func TestMigrateRawPatterns_ZeroFrequency_DefaultsToOne(t *testing.T) {
	patterns := []rawPattern{
		{Category: "addition", Description: "zero freq", Frequency: 0},
	}

	username, cleanup := writeRawPatternsFile(t, patterns)
	defer cleanup()

	dst := &Ontology{Version: defaultVersion}
	_, err := MigrateRawPatterns(username, dst)
	if err != nil {
		t.Fatalf("MigrateRawPatterns: %v", err)
	}

	node := dst.Schema.Nodes["behavior/addition"]
	if node.Frequency < 1 {
		t.Errorf("expected frequency >= 1, got %d", node.Frequency)
	}
}
