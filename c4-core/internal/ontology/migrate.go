package ontology

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// coreCategories maps EditPattern categories to ontology node paths.
// These are the 4 core axes of the ontology.
var coreCategories = map[string]string{
	"addition":  "behavior/addition",
	"deletion":  "behavior/deletion",
	"structure": "behavior/structure",
	"wording":   "behavior/wording",
}

// rawPattern is the JSON representation of an EditPattern from raw_patterns.json.
type rawPattern struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Frequency   int      `json:"frequency"`
	Examples    []string `json:"examples"`
}

// rawPatternsPath returns the canonical path for a user's raw_patterns.json.
func rawPatternsPath(username string) (string, error) {
	if username == "" {
		username = "default"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "personas", username, "raw_patterns.json"), nil
}

// MigrateRawPatterns reads raw_patterns.json for the given username, converts
// each EditPattern into an ontology Node using the 4-axis core mapping, and
// merges the results into dst via Updater.AddOrUpdate.
//
// Core 4-axis mapping:
//   - addition  → behavior/addition
//   - deletion  → behavior/deletion
//   - structure → behavior/structure
//   - wording   → behavior/wording
//
// Unknown categories map to extended/{category}.
// All existing ontology data in dst is preserved.
func MigrateRawPatterns(username string, dst *Ontology) (int, error) {
	path, err := rawPatternsPath(username)
	if err != nil {
		return 0, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read raw_patterns: %w", err)
	}

	var patterns []rawPattern
	if err := json.Unmarshal(data, &patterns); err != nil {
		return 0, fmt.Errorf("parse raw_patterns: %w", err)
	}

	u := NewUpdater(dst)
	count := 0

	for _, p := range patterns {
		nodePath, ok := coreCategories[p.Category]
		if !ok {
			nodePath = "extended/" + p.Category
		}

		freq := p.Frequency
		if freq <= 0 {
			freq = 1
		}

		node := Node{
			Label:       p.Category,
			Description: p.Description,
			Frequency:   freq,
		}
		if len(p.Examples) > 0 {
			node.Properties = map[string]string{"example": p.Examples[0]}
		}

		u.AddOrUpdate(nodePath, node)
		count++
	}

	return count, nil
}
