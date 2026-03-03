package pop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PopState tracks the timestamps of the last POP lifecycle events.
type PopState struct {
	LastExtractedAt    time.Time `json:"last_extracted_at"`
	LastCrystallizedAt time.Time `json:"last_crystallized_at"`
}

// DefaultStatePath returns the canonical path for state.json under root.
func DefaultStatePath(root string) string {
	return filepath.Join(root, ".c4", "pop", "state.json")
}

// Load reads PopState from path. If the file does not exist a zero-value
// PopState is returned with no error.
func Load(path string) (*PopState, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &PopState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s PopState
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save persists s to path atomically (tmpfile → Rename), creating parent
// directories as needed.
func (s *PopState) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("pop: state tmp create: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("pop: state tmp write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("pop: state tmp close: %w", err)
	}
	return os.Rename(tmpName, path)
}
