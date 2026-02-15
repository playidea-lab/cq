package model

import (
	"testing"
	"time"
)

func TestJobStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status   JobStatus
		terminal bool
	}{
		{StatusQueued, false},
		{StatusRunning, false},
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusCancelled, true},
	}

	for _, tt := range tests {
		if tt.status.IsTerminal() != tt.terminal {
			t.Errorf("%s: expected terminal=%v", tt.status, tt.terminal)
		}
	}
}

func TestJobDurationSec(t *testing.T) {
	now := time.Now()
	later := now.Add(10 * time.Second)

	// No start/finish
	j := &Job{}
	if j.DurationSec() != nil {
		t.Error("expected nil duration for unstarted job")
	}

	// Started but not finished
	j.StartedAt = &now
	if j.DurationSec() != nil {
		t.Error("expected nil duration for unfinished job")
	}

	// Both set
	j.FinishedAt = &later
	dur := j.DurationSec()
	if dur == nil {
		t.Fatal("expected non-nil duration")
	}
	if *dur < 9.9 || *dur > 10.1 {
		t.Errorf("expected ~10s, got %f", *dur)
	}
}

func TestNormalizeCommandHash(t *testing.T) {
	// Same command with different seeds should produce same hash
	h1 := NormalizeCommandHash("python train.py --seed 42")
	h2 := NormalizeCommandHash("python train.py --seed 123")
	if h1 != h2 {
		t.Errorf("hashes should match for same command with different seeds: %s vs %s", h1, h2)
	}

	// Different commands should produce different hashes
	h3 := NormalizeCommandHash("python eval.py")
	if h1 == h3 {
		t.Error("different commands should have different hashes")
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	h1 := NormalizeCommandHash("python run.py --date 2026-01-15T10:30")
	h2 := NormalizeCommandHash("python run.py --date 2026-02-20T14:00")
	if h1 != h2 {
		t.Error("timestamps should be normalized")
	}
}

func TestNormalizeTmpPath(t *testing.T) {
	h1 := NormalizeCommandHash("python run.py --output /tmp/abc123")
	h2 := NormalizeCommandHash("python run.py --output /tmp/xyz789")
	if h1 != h2 {
		t.Error("tmp paths should be normalized")
	}
}

func TestCommandHash(t *testing.T) {
	j := &Job{Command: "echo hello"}
	hash := j.CommandHash()
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if len(hash) != 16 { // 8 bytes hex = 16 chars
		t.Errorf("expected 16 char hash, got %d: %s", len(hash), hash)
	}
}
