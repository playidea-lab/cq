package pop

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPopState_LoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Load on missing file — zero state, no error.
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if !s.LastExtractedAt.IsZero() {
		t.Fatal("expected zero LastExtractedAt")
	}
	if !s.LastCrystallizedAt.IsZero() {
		t.Fatal("expected zero LastCrystallizedAt")
	}

	// Populate and save.
	now := time.Now().UTC().Truncate(time.Second)
	s.LastExtractedAt = now
	s.LastCrystallizedAt = now.Add(-5 * time.Minute)
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify.
	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if !s2.LastExtractedAt.Equal(now) {
		t.Fatalf("LastExtractedAt: got %v, want %v", s2.LastExtractedAt, now)
	}
	if !s2.LastCrystallizedAt.Equal(now.Add(-5 * time.Minute)) {
		t.Fatalf("LastCrystallizedAt: got %v, want %v", s2.LastCrystallizedAt, now.Add(-5*time.Minute))
	}
}

func TestDefaultStatePath(t *testing.T) {
	got := DefaultStatePath("/home/user/myproject")
	want := "/home/user/myproject/.c4/pop/state.json"
	if got != want {
		t.Fatalf("DefaultStatePath: got %q, want %q", got, want)
	}
}
