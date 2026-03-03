package pop

import (
	"encoding/json"
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

// Save persists s to path, creating parent directories as needed.
func (s *PopState) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
