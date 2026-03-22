package ontology

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectOntologyPath(t *testing.T) {
	tmp := t.TempDir()
	path, err := projectOntologyPath(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(tmp, ".c4", "project-ontology.yaml")
	if path != expected {
		t.Errorf("got %s, want %s", path, expected)
	}
}

func TestLoadProject_FileNotExist_ReturnsDefault(t *testing.T) {
	tmp := t.TempDir()
	o, err := LoadProject(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.Version != defaultVersion {
		t.Errorf("expected version %s, got %s", defaultVersion, o.Version)
	}
	if o.Schema.Nodes != nil {
		t.Errorf("expected nil nodes, got %v", o.Schema.Nodes)
	}
}

func TestSaveProject_UpdatesUpdatedAt(t *testing.T) {
	tmp := t.TempDir()
	before := time.Now().UTC().Add(-time.Second)
	o := &ProjectOntology{Version: defaultVersion}

	if err := SaveProject(tmp, o); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if !o.UpdatedAt.After(before) {
		t.Errorf("expected UpdatedAt to be set after save, got %v", o.UpdatedAt)
	}
}

func TestSaveAndLoadProject_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	o := &ProjectOntology{
		Version:   "1.0.0",
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"service": {
					Label:      "Service",
					Scope:      "project",
					SourceRole: "user",
				},
			},
		},
	}

	if err := SaveProject(tmp, o); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadProject(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Version != "1.0.0" {
		t.Errorf("version mismatch: got %s", loaded.Version)
	}
	node, ok := loaded.Schema.Nodes["service"]
	if !ok {
		t.Fatal("expected service node to be present")
	}
	if node.Label != "Service" {
		t.Errorf("label mismatch: got %s", node.Label)
	}
	if node.Scope != "project" {
		t.Errorf("scope mismatch: got %s", node.Scope)
	}
	if node.SourceRole != "user" {
		t.Errorf("source_role mismatch: got %s", node.SourceRole)
	}
}

func TestBackupProject_NoSourceFile_IsNoOp(t *testing.T) {
	tmp := t.TempDir()
	if err := BackupProject(tmp); err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}

func TestBackupProject_CreatesBackupFile(t *testing.T) {
	tmp := t.TempDir()
	o := &ProjectOntology{
		Version: "2.0.0",
		Schema: CoreSchema{
			Nodes: map[string]Node{"db": {Label: "Database", Scope: "project"}},
		},
	}

	if err := SaveProject(tmp, o); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := BackupProject(tmp); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	backupPath := filepath.Join(tmp, ".c4", "project-ontology.yaml.bak")
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
}

func TestNode_ScopeAndSourceRole_Omitempty(t *testing.T) {
	// A node without Scope/SourceRole should not error and fields should be zero
	n := Node{Label: "Plain"}
	if n.Scope != "" {
		t.Errorf("expected empty Scope, got %q", n.Scope)
	}
	if n.SourceRole != "" {
		t.Errorf("expected empty SourceRole, got %q", n.SourceRole)
	}
}
