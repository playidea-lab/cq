package ontology

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// writeTeamYAML writes a minimal team.yaml into <root>/.c4/team.yaml.
func writeTeamYAML(t *testing.T, root string, tc teamConfig) {
	t.Helper()
	dir := filepath.Join(root, ".c4")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir .c4: %v", err)
	}
	data, err := yaml.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal team.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "team.yaml"), data, 0644); err != nil {
		t.Fatalf("write team.yaml: %v", err)
	}
}

func TestExtractHighConfidence_OnlyHighNodesExtracted(t *testing.T) {
	home := setupTempHome(t)
	root := t.TempDir()

	// Build L1 ontology with mixed confidence nodes.
	o := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"api":    {Label: "API", NodeConfidence: ConfidenceHigh, Frequency: 5},
				"db":     {Label: "DB", NodeConfidence: ConfidenceMedium, Frequency: 2},
				"logger": {Label: "Logger", NodeConfidence: ConfidenceLow, Frequency: 1},
			},
		},
	}
	_ = home
	if err := Save("alice", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	n, err := ExtractHighConfidence("alice", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node extracted, got %d", n)
	}

	proj, err := LoadProject(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if _, ok := proj.Schema.Nodes["api"]; !ok {
		t.Error("expected api node in project ontology")
	}
	if _, ok := proj.Schema.Nodes["db"]; ok {
		t.Error("did not expect db (medium) in project ontology")
	}
	if _, ok := proj.Schema.Nodes["logger"]; ok {
		t.Error("did not expect logger (low) in project ontology")
	}
}

func TestExtractHighConfidence_SourceRoleTaggedFromTeamYAML(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	writeTeamYAML(t, root, teamConfig{
		Members: map[string]teamMember{
			"bob": {Role: "backend"},
		},
	})

	o := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"svc": {Label: "Service", NodeConfidence: ConfidenceHigh, Frequency: 3},
			},
		},
	}
	if err := Save("bob", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	_, err := ExtractHighConfidence("bob", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	proj, err := LoadProject(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	node, ok := proj.Schema.Nodes["svc"]
	if !ok {
		t.Fatal("expected svc node in project ontology")
	}
	if node.SourceRole != "backend" {
		t.Errorf("expected source_role=backend, got %q", node.SourceRole)
	}
	if node.Scope != "project" {
		t.Errorf("expected scope=project, got %q", node.Scope)
	}
}

func TestExtractHighConfidence_NoTeamYAML_SourceRoleEmpty(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()
	// No team.yaml written.

	o := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"cache": {Label: "Cache", NodeConfidence: ConfidenceHigh, Frequency: 4},
			},
		},
	}
	if err := Save("carol", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	n, err := ExtractHighConfidence("carol", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node, got %d", n)
	}

	proj, err := LoadProject(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	node := proj.Schema.Nodes["cache"]
	if node.SourceRole != "" {
		t.Errorf("expected empty source_role when no team.yaml, got %q", node.SourceRole)
	}
}

func TestExtractHighConfidence_EmptyL1_IsNoOp(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	// L1 ontology has no nodes.
	o := &Ontology{Version: defaultVersion}
	if err := Save("dave", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	n, err := ExtractHighConfidence("dave", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}

	// Project ontology should still be default (no file written).
	proj, err := LoadProject(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if len(proj.Schema.Nodes) != 0 {
		t.Errorf("expected no project nodes, got %v", proj.Schema.Nodes)
	}
}

func TestExtractHighConfidence_NoHighNodes_IsNoOp(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	o := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"draft": {Label: "Draft", NodeConfidence: ConfidenceLow},
			},
		},
	}
	if err := Save("eve", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	n, err := ExtractHighConfidence("eve", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestExtractHighConfidence_MergesIntoExistingProject(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	// Pre-populate project ontology with one node.
	existing := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"old": {Label: "Old", NodeConfidence: ConfidenceHigh},
			},
		},
	}
	if err := SaveProject(root, existing); err != nil {
		t.Fatalf("save project: %v", err)
	}

	// L1 has a new HIGH node.
	o := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"new": {Label: "New", NodeConfidence: ConfidenceHigh, Frequency: 3},
			},
		},
	}
	if err := Save("frank", o); err != nil {
		t.Fatalf("save L1: %v", err)
	}

	n, err := ExtractHighConfidence("frank", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}

	proj, err := LoadProject(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if _, ok := proj.Schema.Nodes["old"]; !ok {
		t.Error("existing 'old' node should be preserved")
	}
	if _, ok := proj.Schema.Nodes["new"]; !ok {
		t.Error("new HIGH node should be present")
	}
}

func TestExtractHighConfidence_EmptyUsername_ReturnsError(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	_, err := ExtractHighConfidence("", root)
	if err == nil {
		t.Error("expected error for empty username")
	}
}
