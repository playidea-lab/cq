package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
)

// globalDomains lists domains whose knowledge is considered cross-project.
var globalDomains = map[string]bool{
	"go":        true,
	"python":    true,
	"ts":        true,
	"debugging": true,
	"testing":   true,
}

// IsGlobalDomain reports whether domain should be stored in the global knowledge store.
func IsGlobalDomain(domain string) bool {
	return globalDomains[domain]
}

// GlobalKnowledgeManager wraps a second *Store rooted at ~/.c4/knowledge/.
// Initialization failures are non-fatal: manager is nil and callers skip global storage.
type GlobalKnowledgeManager struct {
	globalStore *Store
}

// NewGlobalKnowledgeManager opens (or creates) the global knowledge store at ~/.c4/knowledge/.
// Returns nil (non-fatal) if the home directory cannot be determined or the store fails to open.
func NewGlobalKnowledgeManager() *GlobalKnowledgeManager {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: global knowledge: cannot determine home dir: %v\n", err)
		return nil
	}
	globalDir := filepath.Join(home, ".c4", "knowledge")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "c4: global knowledge: cannot create dir: %v\n", err)
		return nil
	}
	gs, err := NewStore(globalDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: global knowledge: store init failed (skipped): %v\n", err)
		return nil
	}
	return &GlobalKnowledgeManager{globalStore: gs}
}

// RecordGlobal saves a document to the global knowledge store.
// metadata and body mirror the parameters accepted by knowledgeRecordNativeHandler.
// Errors are non-fatal; callers should log and continue.
func (g *GlobalKnowledgeManager) RecordGlobal(docType DocumentType, metadata map[string]any, body string) (string, error) {
	if g == nil || g.globalStore == nil {
		return "", fmt.Errorf("global knowledge store unavailable")
	}
	docID, err := g.globalStore.Create(docType, metadata, body)
	if err != nil {
		return "", fmt.Errorf("global knowledge record: %w", err)
	}
	return docID, nil
}
