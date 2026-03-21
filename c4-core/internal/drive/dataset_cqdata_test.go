package drive

import (
	"testing"

	"github.com/changmin/c4-core/internal/cqdata"
)

func TestApplyCQData_CreatesEntry(t *testing.T) {
	dir := t.TempDir()

	if err := ApplyCQData(dir, "training", "a3f8c1e2d4b69f72"); err != nil {
		t.Fatalf("ApplyCQData: %v", err)
	}

	cd, err := cqdata.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	name, ver, ok := cd.GetDataset("training")
	if !ok {
		t.Fatal("expected 'training' key in .cqdata")
	}
	if name != "training" {
		t.Errorf("expected name=%q, got %q", "training", name)
	}
	if ver != "a3f8c1e2d4b69f72" {
		t.Errorf("expected version=%q, got %q", "a3f8c1e2d4b69f72", ver)
	}
}

func TestApplyCQData_UpdatesExistingEntry(t *testing.T) {
	dir := t.TempDir()

	if err := ApplyCQData(dir, "training", "oldversion1234567"); err != nil {
		t.Fatalf("first ApplyCQData: %v", err)
	}
	if err := ApplyCQData(dir, "training", "newversion8901234"); err != nil {
		t.Fatalf("second ApplyCQData: %v", err)
	}

	cd, err := cqdata.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, ver, ok := cd.GetDataset("training")
	if !ok {
		t.Fatal("expected 'training' key")
	}
	if ver != "newversion8901234" {
		t.Errorf("expected updated version, got %q", ver)
	}
}
