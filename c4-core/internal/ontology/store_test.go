package ontology

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTempHome redirects os.UserHomeDir via HOME env var to a temp directory,
// returns cleanup func.
func setupTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestOntologyPath_Default(t *testing.T) {
	home := setupTempHome(t)
	path, err := ontologyPath("alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".c4", "personas", "alice", "ontology.yaml")
	if path != expected {
		t.Errorf("got %s, want %s", path, expected)
	}
}

func TestOntologyPath_EmptyUsernameDefaultsToDefault(t *testing.T) {
	home := setupTempHome(t)
	path, err := ontologyPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".c4", "personas", "default", "ontology.yaml")
	if path != expected {
		t.Errorf("got %s, want %s", path, expected)
	}
}

func TestLoad_FileNotExist_ReturnsDefault(t *testing.T) {
	setupTempHome(t)
	o, err := Load("nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.Version != defaultVersion {
		t.Errorf("expected version %s, got %s", defaultVersion, o.Version)
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	setupTempHome(t)
	o := &Ontology{
		Version:   "1.0.0",
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"task": {Label: "Task", Description: "Unit of work", Tags: []string{"core"}},
			},
		},
	}

	if err := Save("testuser", o); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load("testuser")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Version != "1.0.0" {
		t.Errorf("version mismatch: got %s", loaded.Version)
	}
	node, ok := loaded.Schema.Nodes["task"]
	if !ok {
		t.Fatal("expected task node to be present")
	}
	if node.Label != "Task" {
		t.Errorf("label mismatch: got %s", node.Label)
	}
	if len(node.Tags) != 1 || node.Tags[0] != "core" {
		t.Errorf("tags mismatch: got %v", node.Tags)
	}
}

func TestSave_UpdatesUpdatedAt(t *testing.T) {
	setupTempHome(t)
	before := time.Now().UTC().Add(-time.Second)
	o := &Ontology{Version: "1.0.0"}

	if err := Save("timeuser", o); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if !o.UpdatedAt.After(before) {
		t.Errorf("expected UpdatedAt to be set after save, got %v", o.UpdatedAt)
	}
}

func TestBackup_NoSourceFile_IsNoOp(t *testing.T) {
	setupTempHome(t)
	// Should not error when source file is missing
	if err := Backup("ghost"); err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}

func TestBackupAndRestore_RoundTrip(t *testing.T) {
	home := setupTempHome(t)
	o := &Ontology{
		Version: "2.0.0",
		Schema: CoreSchema{
			Nodes: map[string]Node{"x": {Label: "X"}},
		},
	}

	if err := Save("bkuser", o); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := Backup("bkuser"); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	backupPath := filepath.Join(home, ".c4", "personas", "bkuser", "ontology.yaml.bak")
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not found: %v", err)
	}

	// Overwrite with new data
	o2 := &Ontology{Version: "3.0.0"}
	if err := Save("bkuser", o2); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	// Restore should bring back 2.0.0
	if err := Restore("bkuser"); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	restored, err := Load("bkuser")
	if err != nil {
		t.Fatalf("load after restore failed: %v", err)
	}
	if restored.Version != "2.0.0" {
		t.Errorf("expected restored version 2.0.0, got %s", restored.Version)
	}
}

func TestRestore_NoBackup_ReturnsError(t *testing.T) {
	setupTempHome(t)
	err := Restore("nobody")
	if err == nil {
		t.Error("expected error when no backup exists")
	}
}
