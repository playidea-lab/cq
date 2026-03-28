package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	autoLearnedHeader    = "## Auto-Learned"
	autoLearnedRulesFile = "auto-learned.md"
	hintPrefix           = "- [auto] "
	rulePrefix           = "- "

	hintThreshold = 3
	ruleThreshold = 5

	autoLearnedRulesHeader = "# Auto-Learned Rules\n\n> Rules automatically generated from repeated preferences.\n"
)

// PromoteHints adds hints to the claude.md file for ledger entries with
// Count >= 3 that are not suppressed. It appends to (or creates) the
// "## Auto-Learned" section. Already-present hints are skipped.
// Returns the list of newly added hint keys.
func PromoteHints(ledger PreferenceLedger, claudeMdPath string) ([]string, error) {
	content, err := readOrEmpty(claudeMdPath)
	if err != nil {
		return nil, fmt.Errorf("read claude.md: %w", err)
	}

	content = ensureAutoLearnedSection(content)

	var added []string
	for key, entry := range ledger {
		if entry.Suppressed || entry.Count < hintThreshold {
			continue
		}
		hintLine := hintPrefix + key
		if strings.Contains(content, hintLine) {
			continue
		}
		content = appendAfterMarker(content, autoLearnedHeader, hintLine)
		added = append(added, key)
	}

	if err := writeFile(claudeMdPath, content); err != nil {
		return nil, fmt.Errorf("write claude.md: %w", err)
	}
	return added, nil
}

// PromoteRules adds rules to {rulesDir}/auto-learned.md for ledger entries
// with Count >= 5 that are not suppressed. Creates the file if absent.
// Already-present rules are skipped. Returns the list of newly added rule keys.
func PromoteRules(ledger PreferenceLedger, rulesDir string) ([]string, error) {
	rulesPath := filepath.Join(rulesDir, autoLearnedRulesFile)

	content, err := readOrEmpty(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("read auto-learned.md: %w", err)
	}

	if content == "" {
		content = autoLearnedRulesHeader
	}

	var added []string
	for key, entry := range ledger {
		if entry.Suppressed || entry.Count < ruleThreshold {
			continue
		}
		ruleLine := rulePrefix + key
		if strings.Contains(content, ruleLine) {
			continue
		}
		// Append rule at end of file (with newline guard).
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += ruleLine + "\n"
		added = append(added, key)
	}

	if err := writeFile(rulesPath, content); err != nil {
		return nil, fmt.Errorf("write auto-learned.md: %w", err)
	}
	return added, nil
}

// SuppressKey loads the ledger at ledgerPath, sets Suppressed=true on the
// normalized key, and saves back to disk. It is a no-op if the key is absent.
func SuppressKey(ledgerPath, key string) error {
	key = normalizeKey(key)
	ledger, err := loadLedger(ledgerPath)
	if err != nil {
		return fmt.Errorf("load ledger: %w", err)
	}
	if entry, ok := ledger[key]; ok {
		entry.Suppressed = true
	}
	return saveLedger(ledger, ledgerPath)
}

// --- helpers -----------------------------------------------------------------

// readOrEmpty reads the file at path; returns "" if the file does not exist.
func readOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// ensureAutoLearnedSection appends "## Auto-Learned\n\n" to content if the
// marker is not already present.
func ensureAutoLearnedSection(content string) string {
	if strings.Contains(content, autoLearnedHeader) {
		return content
	}
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	content += "\n" + autoLearnedHeader + "\n\n"
	return content
}

// appendAfterMarker inserts line immediately after the marker line in content.
// If the marker appears multiple times the first occurrence is used.
func appendAfterMarker(content, marker, line string) string {
	idx := strings.Index(content, marker)
	if idx == -1 {
		// Fallback: append at end.
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + line + "\n"
	}
	// Find the end of the marker line.
	rest := content[idx:]
	nlIdx := strings.Index(rest, "\n")
	if nlIdx == -1 {
		// Marker is at the very end with no newline.
		return content + "\n" + line + "\n"
	}
	insertAt := idx + nlIdx + 1 // just after the marker's newline
	return content[:insertAt] + line + "\n" + content[insertAt:]
}
