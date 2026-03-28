package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LedgerEntry tracks cumulative preference signal for a single normalized key.
type LedgerEntry struct {
	Count      int       `yaml:"count"`
	FirstSeen  time.Time `yaml:"first_seen"`
	LastSeen   time.Time `yaml:"last_seen"`
	Suppressed bool      `yaml:"suppressed"`
}

// PreferenceLedger maps normalized preference keys to their ledger entries.
type PreferenceLedger map[string]*LedgerEntry

// defaultLedgerPath returns ~/.c4/preference_ledger.yaml.
func defaultLedgerPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(homeDir, ".c4", "preference_ledger.yaml"), nil
}

// normalizeKey lowercases and trims whitespace from a preference key.
func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// loadLedger reads the YAML ledger from path. If the file does not exist, an
// empty ledger is returned without error.
func loadLedger(path string) (PreferenceLedger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(PreferenceLedger), nil
		}
		return nil, fmt.Errorf("read ledger %s: %w", path, err)
	}

	var ledger PreferenceLedger
	if err := yaml.Unmarshal(data, &ledger); err != nil {
		return nil, fmt.Errorf("parse ledger: %w", err)
	}
	if ledger == nil {
		return make(PreferenceLedger), nil
	}
	return ledger, nil
}

// saveLedger writes the ledger to path as YAML, creating parent directories
// as needed.
func saveLedger(ledger PreferenceLedger, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}
	data, err := yaml.Marshal(ledger)
	if err != nil {
		return fmt.Errorf("marshal ledger: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write ledger %s: %w", path, err)
	}
	return nil
}

// incrementPreferences increments the count for each key in keys, recording
// first/last seen timestamps. Keys are normalized before lookup. Suppressed
// entries are still updated (callers decide how to handle suppression).
func incrementPreferences(ledger PreferenceLedger, keys []string) {
	now := time.Now().UTC()
	for _, raw := range keys {
		key := normalizeKey(raw)
		if key == "" {
			continue
		}
		entry, exists := ledger[key]
		if !exists {
			entry = &LedgerEntry{
				FirstSeen: now,
			}
			ledger[key] = entry
		}
		entry.Count++
		entry.LastSeen = now
	}
}

// IncrementAndSave is the public convenience function: loads the default ledger,
// increments the given preference keys, and saves back to disk.
func IncrementAndSave(keys []string) error {
	path, err := defaultLedgerPath()
	if err != nil {
		return err
	}
	return IncrementAndSaveAt(keys, path)
}

// IncrementAndSaveAt is the same as IncrementAndSave but accepts an explicit path.
// This is the primary entry point for external callers.
func IncrementAndSaveAt(keys []string, path string) error {
	ledger, err := loadLedger(path)
	if err != nil {
		return err
	}
	incrementPreferences(ledger, keys)
	return saveLedger(ledger, path)
}

// LoadLedgerAt loads the preference ledger at the given path (public wrapper).
func LoadLedgerAt(path string) (PreferenceLedger, error) {
	return loadLedger(path)
}
