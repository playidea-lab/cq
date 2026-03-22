package ontology

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTempRule writes a markdown rule file in rulesDir with the given content.
func writeTempRule(t *testing.T, rulesDir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("create rules dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write rule file: %v", err)
	}
}

// makeProjectOntology builds a ProjectOntology with the given nodes.
func makeProjectOntology(nodes map[string]Node) *ProjectOntology {
	return &ProjectOntology{
		Version: "1.0.0",
		Schema:  CoreSchema{Nodes: nodes},
	}
}

func TestPikiFrontMatter_ExtractsBullets(t *testing.T) {
	content := `# Security Rules

## Secret Management

- Do not hardcode API keys.
- Use environment variables.

Non-bullet line.
`
	rules := pikiFrontMatter(content)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d: %v", len(rules), rules)
	}
	if rules[0] != "Do not hardcode API keys." {
		t.Errorf("unexpected rule[0]: %q", rules[0])
	}
	if rules[1] != "Use environment variables." {
		t.Errorf("unexpected rule[1]: %q", rules[1])
	}
}

func TestPikiFrontMatter_EmptyFile(t *testing.T) {
	rules := pikiFrontMatter("")
	if len(rules) != 0 {
		t.Errorf("expected no rules for empty file, got %d", len(rules))
	}
}

func TestLoadPikiRules_ReadsMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	rulesDir := filepath.Join(root, ".claude", "rules")

	writeTempRule(t, rulesDir, "security.md", "# Security\n\n- No hardcoded secrets.\n- Use vault.\n")
	writeTempRule(t, rulesDir, "testing.md", "# Testing\n\n- All tests must be independent.\n")
	// Non-markdown file should be ignored.
	_ = os.WriteFile(filepath.Join(rulesDir, "notes.txt"), []byte("- not a rule"), 0644)

	rules, err := LoadPikiRules(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
}

func TestLoadPikiRules_NoDirReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	rules, err := LoadPikiRules(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules when dir absent, got %d", len(rules))
	}
}

func TestDetectConflicts_BelowThreshold_NoConflict(t *testing.T) {
	proj := makeProjectOntology(map[string]Node{
		"auth/no-validation": {
			Label:     "no validation bypass",
			Frequency: 5, // below EvidenceThreshold
		},
	})
	rules := []PikiRule{
		{File: "security.md", Text: "Always validate user input."},
	}
	conflicts := detectConflicts(proj, rules)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts below threshold, got %d", len(conflicts))
	}
}

func TestDetectConflicts_AboveThreshold_DetectsConflict(t *testing.T) {
	proj := makeProjectOntology(map[string]Node{
		"auth/bypass": {
			Label:       "bypass authentication",
			Description: "not checking auth for internal endpoints",
			Frequency:   12,
		},
	})
	rules := []PikiRule{
		{File: "security.md", Text: "All API endpoints default to authentication required."},
	}
	conflicts := detectConflicts(proj, rules)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].NodePath != "auth/bypass" {
		t.Errorf("unexpected node path: %q", conflicts[0].NodePath)
	}
	if conflicts[0].EvidenceCount != 12 {
		t.Errorf("unexpected evidence count: %d", conflicts[0].EvidenceCount)
	}
}

func TestDetectConflicts_NoConflictWhenNodeHarmless(t *testing.T) {
	proj := makeProjectOntology(map[string]Node{
		"caching/redis": {
			Label:     "Redis caching layer",
			Frequency: 15,
		},
	})
	rules := []PikiRule{
		{File: "security.md", Text: "All API endpoints default to authentication required."},
	}
	conflicts := detectConflicts(proj, rules)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for harmless node, got %d", len(conflicts))
	}
}

func TestBuildProposal_StructuresCorrectly(t *testing.T) {
	now := time.Now().UTC()
	conflicts := []Conflict{
		{
			NodePath:      "api/skip-auth",
			NodeLabel:     "skip auth",
			EvidenceCount: 11,
			ConflictingRules: []PikiRule{
				{File: "security.md", Text: "Validate inputs."},
				{File: "security.md", Text: "Auth required."},
			},
		},
	}
	proposal := buildProposal(conflicts, now)
	if !proposal.GeneratedAt.Equal(now) {
		t.Errorf("GeneratedAt mismatch")
	}
	if len(proposal.Conflicts) != 1 {
		t.Fatalf("expected 1 proposal conflict, got %d", len(proposal.Conflicts))
	}
	pc := proposal.Conflicts[0]
	if pc.NodePath != "api/skip-auth" {
		t.Errorf("unexpected node path: %q", pc.NodePath)
	}
	if pc.EvidenceCount != 11 {
		t.Errorf("unexpected evidence count: %d", pc.EvidenceCount)
	}
	// Duplicate sources should be deduplicated.
	if len(pc.ConflictSources) != 1 || pc.ConflictSources[0] != "security.md" {
		t.Errorf("unexpected conflict sources: %v", pc.ConflictSources)
	}
	if pc.SuggestedUpdate == "" {
		t.Error("expected non-empty SuggestedUpdate")
	}
}

func TestDetectPikiConflicts_CooldownPreventsRerun(t *testing.T) {
	root := t.TempDir()

	// Seed a last-run timestamp that is recent (1 hour ago).
	recentTime := time.Now().UTC().Add(-1 * time.Hour)
	if err := saveLastRun(root, recentTime); err != nil {
		t.Fatalf("saveLastRun: %v", err)
	}

	n, path, err := DetectPikiConflicts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 || path != "" {
		t.Errorf("expected cooldown to skip run, got n=%d path=%q", n, path)
	}
}

func TestDetectPikiConflicts_NoConflicts_NoProposal(t *testing.T) {
	root := t.TempDir()

	// Write a harmless ontology with high-frequency nodes.
	proj := makeProjectOntology(map[string]Node{
		"caching/lru": {Label: "LRU cache", Frequency: 20},
	})
	if err := SaveProject(root, proj); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	// Write a rule that won't conflict.
	writeTempRule(t, filepath.Join(root, ".claude", "rules"), "testing.md",
		"# Testing\n\n- Tests must be independent.\n")

	n, path, err := DetectPikiConflicts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 conflicts, got %d", n)
	}
	if path != "" {
		t.Errorf("expected no proposal path, got %q", path)
	}
}

func TestDetectPikiConflicts_WritesProposal(t *testing.T) {
	root := t.TempDir()

	proj := makeProjectOntology(map[string]Node{
		"auth/no-check": {
			Label:     "no auth check",
			Frequency: EvidenceThreshold + 2,
		},
	})
	if err := SaveProject(root, proj); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	writeTempRule(t, filepath.Join(root, ".claude", "rules"), "security.md",
		"# Security\n\n- Authentication required by default.\n- Validate user inputs.\n")

	n, path, err := DetectPikiConflicts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 conflict, got %d", n)
	}
	if path == "" {
		t.Fatal("expected proposal path, got empty")
	}

	// Verify file exists and is non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat proposal: %v", err)
	}
	if info.Size() == 0 {
		t.Error("proposal file is empty")
	}

	// Verify last-run was updated.
	last, err := loadLastRun(root)
	if err != nil {
		t.Fatalf("loadLastRun: %v", err)
	}
	if last.IsZero() {
		t.Error("last-run should be set after run")
	}

	// Second run within cooldown should be skipped.
	n2, path2, err := DetectPikiConflicts(root)
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	if n2 != 0 || path2 != "" {
		t.Errorf("second run should be skipped by cooldown, got n=%d path=%q", n2, path2)
	}
}

func TestDetectPikiConflicts_EmptyOntology_NoProposal(t *testing.T) {
	root := t.TempDir()
	// Don't save any ontology — LoadProject will return empty.
	writeTempRule(t, filepath.Join(root, ".claude", "rules"), "security.md",
		"# Security\n\n- No hardcoded secrets.\n")

	n, path, err := DetectPikiConflicts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 || path != "" {
		t.Errorf("empty ontology should yield no conflicts, got n=%d path=%q", n, path)
	}
}

func TestKeywordOverlap_Detects(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"bypass authentication checks", "authentication required", true},
		{"redis cache layer", "authentication required", false},
		{"no", "authentication", false}, // word too short
	}
	for _, tc := range cases {
		got := keywordOverlap(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("keywordOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
