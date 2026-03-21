package cqdata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/cqdata"
)

func TestLoad_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	cd, err := cqdata.Load(dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cd == nil {
		t.Fatal("expected non-nil CQData")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	cd := &cqdata.CQData{}
	cd.SetDataset("train", "my-dataset", "v1.0")
	cd.SetDataset("val", "my-dataset", "v1.1")

	if err := cd.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must exist.
	if _, err := os.Stat(filepath.Join(dir, ".cqdata")); err != nil {
		t.Fatalf("expected .cqdata file: %v", err)
	}

	loaded, err := cqdata.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	name, ver, ok := loaded.GetDataset("train")
	if !ok {
		t.Fatal("expected 'train' key")
	}
	if name != "my-dataset" || ver != "v1.0" {
		t.Fatalf("unexpected train dataset: name=%q version=%q", name, ver)
	}

	name, ver, ok = loaded.GetDataset("val")
	if !ok {
		t.Fatal("expected 'val' key")
	}
	if name != "my-dataset" || ver != "v1.1" {
		t.Fatalf("unexpected val dataset: name=%q version=%q", name, ver)
	}
}

func TestGetDataset_Missing(t *testing.T) {
	cd := &cqdata.CQData{}
	_, _, ok := cd.GetDataset("nonexistent")
	if ok {
		t.Fatal("expected ok=false for missing key")
	}
}

func TestSetDataset_Overwrite(t *testing.T) {
	cd := &cqdata.CQData{}
	cd.SetDataset("k", "ds", "v1")
	cd.SetDataset("k", "ds", "v2")

	_, ver, ok := cd.GetDataset("k")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if ver != "v2" {
		t.Fatalf("expected v2, got %q", ver)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cqdata")
	if err := os.WriteFile(path, []byte(":::invalid yaml:::\n\t bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := cqdata.Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
