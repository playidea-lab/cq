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

// LoadRelevant queries the global store for documents matching any of the given domains,
// then inserts into projectStore any documents not already present (checked by doc_id).
// Returns the count of newly injected documents. Errors are non-fatal; callers should log and continue.
func (g *GlobalKnowledgeManager) LoadRelevant(projectStore *Store, domains []string) (int, error) {
	if g == nil || g.globalStore == nil {
		return 0, fmt.Errorf("global knowledge store unavailable")
	}
	if projectStore == nil {
		return 0, fmt.Errorf("project store unavailable")
	}
	if len(domains) == 0 {
		return 0, nil
	}

	injected := 0
	for _, domain := range domains {
		docs, err := g.globalStore.List("", domain, 200)
		if err != nil {
			return injected, fmt.Errorf("list global domain %q: %w", domain, err)
		}
		for _, summary := range docs {
			docID, _ := summary["id"].(string)
			if docID == "" {
				continue
			}
			// Check if already present in project store
			existing, err := projectStore.Get(docID)
			if err != nil {
				continue
			}
			if existing != nil {
				continue
			}
			// Fetch full document from global store
			full, err := g.globalStore.Get(docID)
			if err != nil || full == nil {
				continue
			}
			metadata := map[string]any{
				"id":     full.ID,
				"title":  full.Title,
				"domain": full.Domain,
				"tags":   full.Tags,
			}
			if _, err := projectStore.Create(full.Type, metadata, full.Body); err != nil {
				continue
			}
			injected++
		}
	}
	return injected, nil
}

// DetectProjectDomains inspects dir for language marker files and returns matching domain names.
func DetectProjectDomains(dir string) []string {
	markers := []struct {
		file   string
		domain string
	}{
		{"go.mod", "go"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{"package.json", "ts"},
	}
	seen := map[string]bool{}
	var domains []string
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m.file)); err == nil {
			if !seen[m.domain] {
				seen[m.domain] = true
				domains = append(domains, m.domain)
			}
		}
	}
	return domains
}
