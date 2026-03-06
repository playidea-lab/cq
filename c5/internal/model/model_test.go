package model

import (
	"encoding/json"
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

// TestArtifactRefMarshal verifies ArtifactRef JSON round-trip.
func TestArtifactRefMarshal(t *testing.T) {
	orig := ArtifactRef{
		Path:      "inputs/data.bin",
		LocalPath: "/local/data.bin",
		Required:  true,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ArtifactRef
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Path != orig.Path {
		t.Errorf("Path: got %q, want %q", got.Path, orig.Path)
	}
	if got.LocalPath != orig.LocalPath {
		t.Errorf("LocalPath: got %q, want %q", got.LocalPath, orig.LocalPath)
	}
	if got.Required != orig.Required {
		t.Errorf("Required: got %v, want %v", got.Required, orig.Required)
	}
}

// TestArtifactRefMarshal_OmitEmpty verifies omitempty fields are absent when zero.
func TestArtifactRefMarshal_OmitEmpty(t *testing.T) {
	ref := ArtifactRef{Path: "outputs/result.bin"}
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	// local_path and required should be omitted
	if contains(s, "local_path") {
		t.Errorf("local_path should be omitted, got: %s", s)
	}
	if contains(s, "required") {
		t.Errorf("required should be omitted, got: %s", s)
	}
}

// TestControlMessageRoundTrip verifies ControlMessage JSON round-trip.
func TestControlMessageRoundTrip(t *testing.T) {
	orig := ControlMessage{Action: "upgrade"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ControlMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != "upgrade" {
		t.Errorf("Action: got %q, want %q", got.Action, "upgrade")
	}
}

// TestLeaseAcquireResponse_ControlOmitEmpty verifies Control is absent when nil.
func TestLeaseAcquireResponse_ControlOmitEmpty(t *testing.T) {
	resp := LeaseAcquireResponse{JobID: "j1", LeaseID: "l1"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(data), "control") {
		t.Errorf("control should be omitted when nil, got: %s", string(data))
	}
}

// TestLeaseAcquireResponse_ControlPresent verifies Control is serialized when set.
func TestLeaseAcquireResponse_ControlPresent(t *testing.T) {
	resp := LeaseAcquireResponse{
		JobID:   "j1",
		LeaseID: "l1",
		Control: &ControlMessage{Action: "shutdown"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !contains(s, "control") {
		t.Errorf("control should be present, got: %s", s)
	}
	if !contains(s, "shutdown") {
		t.Errorf("action should be shutdown, got: %s", s)
	}
}

// TestWorkerRegisterRequest_VersionOmitEmpty verifies Version is absent when empty.
func TestWorkerRegisterRequest_VersionOmitEmpty(t *testing.T) {
	req := WorkerRegisterRequest{Hostname: "host1"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(data), "version") {
		t.Errorf("version should be omitted when empty, got: %s", string(data))
	}
}

// TestWorkerRegisterRequest_VersionPresent verifies Version is serialized when set.
func TestWorkerRegisterRequest_VersionPresent(t *testing.T) {
	req := WorkerRegisterRequest{Hostname: "host1", Version: "v1.2.3"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(string(data), "v1.2.3") {
		t.Errorf("version should be present, got: %s", string(data))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
