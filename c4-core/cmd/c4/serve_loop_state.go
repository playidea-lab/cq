//go:build research

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// LoopState represents the persisted state of the research loop.
type LoopState struct {
	State               string     `yaml:"state"`                          // "running", "gate_wait", "stopped"
	LoopCount           int        `yaml:"loop_count"`
	CurrentHypothesisID string     `yaml:"current_hypothesis_id,omitempty"`
	LastJobID           string     `yaml:"last_job_id,omitempty"`
	GateDeadline        *time.Time `yaml:"gate_deadline,omitempty"` // nil = no active gate
	LastUpdated         time.Time  `yaml:"last_updated"`
}

// StateYAMLWriter is the single writer for .c9/state.yaml.
// All writes are atomic (tmpfile → os.Rename).
type StateYAMLWriter struct {
	path string
	mu   sync.Mutex
}

// NewStateYAMLWriter creates a writer for the given c9Dir directory.
// The directory is created automatically on first WriteState call.
func NewStateYAMLWriter(c9Dir string) *StateYAMLWriter {
	return &StateYAMLWriter{path: filepath.Join(c9Dir, "state.yaml")}
}

// WriteState atomically writes state to the YAML file.
// The .c9/ directory is created if it does not exist.
func (w *StateYAMLWriter) WriteState(s LoopState) error {
	s.LastUpdated = time.Now().UTC()
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("state marshal: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return atomicWriteFile(w.path, data, 0644)
}

// ReadState reads and parses the YAML file.
// Returns zero LoopState if file does not exist (no error).
func (w *StateYAMLWriter) ReadState() (LoopState, error) {
	data, err := os.ReadFile(w.path)
	if os.IsNotExist(err) {
		return LoopState{}, nil
	}
	if err != nil {
		return LoopState{}, fmt.Errorf("read state: %w", err)
	}
	var s LoopState
	if err := yaml.Unmarshal(data, &s); err != nil {
		return LoopState{}, fmt.Errorf("state unmarshal: %w", err)
	}
	return s, nil
}

// atomicWriteFile writes data to path atomically via a temp file.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// cleanup on failure
		if _, err2 := os.Stat(tmpPath); err2 == nil {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
