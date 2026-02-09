package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestSoulGetNoSoul(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"testuser","role":"dev"}`
	result, err := reg.Call("c4_soul_get", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["source"] != "none" {
		t.Errorf("source = %v, want 'none'", m["source"])
	}
}

func TestSoulGetPersonaFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a persona file
	personasDir := filepath.Join(tmpDir, ".c4", "personas")
	os.MkdirAll(personasDir, 0755)
	os.WriteFile(filepath.Join(personasDir, "persona-developer.md"),
		[]byte("# Persona: Developer\nGeneric developer persona"), 0644)

	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"testuser","role":"developer"}`
	result, err := reg.Call("c4_soul_get", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["source"] != "persona_fallback" {
		t.Errorf("source = %v, want 'persona_fallback'", m["source"])
	}
	if m["content"] != "# Persona: Developer\nGeneric developer persona" {
		t.Errorf("unexpected content: %v", m["content"])
	}
}

func TestSoulSetCreate(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"alice","role":"ceo","section":"Principles","content":"- Move fast\n- Data first"}`
	result, err := reg.Call("c4_soul_set", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["action"] != "created" {
		t.Errorf("action = %v, want 'created'", m["action"])
	}

	// Verify file exists
	soulPath := filepath.Join(tmpDir, ".c4", "souls", "alice", "soul-ceo.md")
	data, err := os.ReadFile(soulPath)
	if err != nil {
		t.Fatalf("soul file not found: %v", err)
	}
	content := string(data)

	// Check header
	if !containsStr(content, "# Soul: alice — ceo") {
		t.Errorf("missing header in soul file")
	}
	// Check principles
	if !containsStr(content, "- Move fast") {
		t.Errorf("missing principles content")
	}
	// Check other sections exist from template
	if !containsStr(content, "## Preferences") {
		t.Errorf("missing Preferences section from template")
	}
	if !containsStr(content, "## Learned") {
		t.Errorf("missing Learned section from template")
	}
}

func TestSoulSetUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	// Create soul
	args1 := `{"username":"bob","role":"dev","section":"Principles","content":"- TDD first"}`
	_, err := reg.Call("c4_soul_set", json.RawMessage(args1))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	// Update Preferences
	args2 := `{"username":"bob","role":"dev","section":"Preferences","content":"- Vim mode\n- Dark theme"}`
	result, err := reg.Call("c4_soul_set", json.RawMessage(args2))
	if err != nil {
		t.Fatalf("update error: %v", err)
	}

	m := result.(map[string]any)
	if m["action"] != "updated" {
		t.Errorf("action = %v, want 'updated'", m["action"])
	}

	// Verify both sections preserved
	soulPath := filepath.Join(tmpDir, ".c4", "souls", "bob", "soul-dev.md")
	data, err := os.ReadFile(soulPath)
	if err != nil {
		t.Fatalf("soul file not found: %v", err)
	}
	content := string(data)

	if !containsStr(content, "- TDD first") {
		t.Errorf("Principles content lost after Preferences update")
	}
	if !containsStr(content, "- Vim mode") {
		t.Errorf("Preferences content not updated")
	}
}

func TestSoulGetExisting(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	// Create soul
	args1 := `{"username":"carol","role":"designer","section":"Principles","content":"- Simplicity"}`
	_, _ = reg.Call("c4_soul_set", json.RawMessage(args1))

	// Get soul
	args2 := `{"username":"carol","role":"designer"}`
	result, err := reg.Call("c4_soul_get", json.RawMessage(args2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["source"] != "soul" {
		t.Errorf("source = %v, want 'soul'", m["source"])
	}
	if m["username"] != "carol" {
		t.Errorf("username = %v, want 'carol'", m["username"])
	}

	sections, ok := m["sections"].(map[string]string)
	if !ok {
		t.Fatalf("sections type = %T, want map[string]string", m["sections"])
	}
	if !containsStr(sections["Principles"], "Simplicity") {
		t.Errorf("Principles section missing content")
	}
}

func TestSoulListMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	// Create two souls for same user
	args1 := `{"username":"dave","role":"ceo","section":"Principles","content":"- Vision first"}`
	_, _ = reg.Call("c4_soul_set", json.RawMessage(args1))

	args2 := `{"username":"dave","role":"developer","section":"Principles","content":"- Code quality"}`
	_, _ = reg.Call("c4_soul_set", json.RawMessage(args2))

	// List all
	args3 := `{"username":"dave"}`
	result, err := reg.Call("c4_soul_get", json.RawMessage(args3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	count, ok := m["count"].(int)
	if !ok {
		t.Fatalf("count type = %T", m["count"])
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestSoulSetInvalidUsername(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"../evil","role":"dev","section":"Principles","content":"hack"}`
	_, err := reg.Call("c4_soul_set", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for path traversal in username")
	}
}

func TestSoulSetInvalidSection(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"test","role":"dev","section":"Hacking","content":"nope"}`
	_, err := reg.Call("c4_soul_set", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for invalid section name")
	}
}

func TestParseSoulSections(t *testing.T) {
	content := `# Soul: test — dev

## Principles
- Principle 1
- Principle 2

## Preferences
- Pref 1

## Learned
- Lesson 1
`
	sections := parseSoulSections(content)

	if !containsStr(sections["Principles"], "Principle 1") {
		t.Errorf("missing Principle 1 in: %q", sections["Principles"])
	}
	if !containsStr(sections["Preferences"], "Pref 1") {
		t.Errorf("missing Pref 1 in: %q", sections["Preferences"])
	}
	if !containsStr(sections["Learned"], "Lesson 1") {
		t.Errorf("missing Lesson 1 in: %q", sections["Learned"])
	}
}

func TestUpdateSection(t *testing.T) {
	content := `# Soul

## Principles
Old content

## Preferences
Keep this
`
	updated := updateSection(content, "Principles", "New content here")

	if !containsStr(updated, "New content here") {
		t.Errorf("new content not found")
	}
	if containsStr(updated, "Old content") {
		t.Errorf("old content should be replaced")
	}
	if !containsStr(updated, "Keep this") {
		t.Errorf("other sections should be preserved")
	}
}

func TestUpdateSectionAppend(t *testing.T) {
	content := `# Soul

## Principles
Some content
`
	updated := updateSection(content, "NewSection", "Appended content")

	if !containsStr(updated, "## NewSection") {
		t.Errorf("new section header not found")
	}
	if !containsStr(updated, "Appended content") {
		t.Errorf("appended content not found")
	}
	if !containsStr(updated, "Some content") {
		t.Errorf("existing content should be preserved")
	}
}

// --- c4_soul_resolve tests ---

func TestSoulResolvePersonaOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create persona
	personasDir := filepath.Join(tmpDir, ".c4", "personas")
	os.MkdirAll(personasDir, 0755)
	os.WriteFile(filepath.Join(personasDir, "persona-developer.md"),
		[]byte("# Persona: Developer\nGeneric developer."), 0644)

	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"alice","role":"developer"}`
	result, err := reg.Call("c4_soul_resolve", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["has_persona"] != true {
		t.Errorf("has_persona = %v, want true", m["has_persona"])
	}
	if m["has_soul"] != false {
		t.Errorf("has_soul = %v, want false", m["has_soul"])
	}
	merged := m["merged"].(string)
	if !containsStr(merged, "Generic developer") {
		t.Errorf("merged should contain persona content")
	}
}

func TestSoulResolveMerged(t *testing.T) {
	tmpDir := t.TempDir()

	// Create persona
	personasDir := filepath.Join(tmpDir, ".c4", "personas")
	os.MkdirAll(personasDir, 0755)
	os.WriteFile(filepath.Join(personasDir, "persona-developer.md"),
		[]byte("# Persona: Developer\nGeneric developer."), 0644)

	// Create soul
	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	setArgs := `{"username":"bob","role":"developer","section":"Principles","content":"- TDD always\n- Data first"}`
	_, _ = reg.Call("c4_soul_set", json.RawMessage(setArgs))

	// Resolve
	args := `{"username":"bob","role":"developer"}`
	result, err := reg.Call("c4_soul_resolve", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["has_persona"] != true {
		t.Errorf("has_persona = %v, want true", m["has_persona"])
	}
	if m["has_soul"] != true {
		t.Errorf("has_soul = %v, want true", m["has_soul"])
	}
	merged := m["merged"].(string)
	if !containsStr(merged, "Generic developer") {
		t.Errorf("merged should contain persona base")
	}
	if !containsStr(merged, "TDD always") {
		t.Errorf("merged should contain soul principles")
	}
	if !containsStr(merged, "Personal Soul Overrides") {
		t.Errorf("merged should have soul overrides header")
	}
}

func TestSoulResolveSoulOnly(t *testing.T) {
	tmpDir := t.TempDir()

	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	// Create soul with no matching persona
	setArgs := `{"username":"carol","role":"unicorn","section":"Principles","content":"- Be unique"}`
	_, _ = reg.Call("c4_soul_set", json.RawMessage(setArgs))

	args := `{"username":"carol","role":"unicorn"}`
	result, err := reg.Call("c4_soul_resolve", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["has_soul"] != true {
		t.Errorf("has_soul = %v, want true", m["has_soul"])
	}
	if m["has_persona"] != false {
		t.Errorf("has_persona = %v, want false", m["has_persona"])
	}
	merged := m["merged"].(string)
	if !containsStr(merged, "Be unique") {
		t.Errorf("merged should contain soul content")
	}
}

func TestSoulResolveNone(t *testing.T) {
	tmpDir := t.TempDir()

	reg := mcp.NewRegistry()
	RegisterSoulHandlers(reg, tmpDir)

	args := `{"username":"nobody","role":"nothing"}`
	result, err := reg.Call("c4_soul_resolve", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	merged := m["merged"].(string)
	if merged != "" {
		t.Errorf("merged should be empty, got %q", merged)
	}
}

// --- applySuggestionsToSoul tests ---

func TestApplySuggestionsToSoulNewFile(t *testing.T) {
	tmpDir := t.TempDir()

	suggestions := []string{"Test suggestion one", "Test suggestion two"}
	err := applySuggestionsToSoul(tmpDir, "testuser", "dev", suggestions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify soul file created
	soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-dev.md")
	data, err := os.ReadFile(soulPath)
	if err != nil {
		t.Fatalf("soul file not created: %v", err)
	}
	content := string(data)

	if !containsStr(content, "Test suggestion one") {
		t.Errorf("missing suggestion one")
	}
	if !containsStr(content, "Test suggestion two") {
		t.Errorf("missing suggestion two")
	}
	if !containsStr(content, "## Learned") {
		t.Errorf("missing Learned section")
	}
}

func TestApplySuggestionsToSoulDedup(t *testing.T) {
	tmpDir := t.TempDir()

	// Apply once
	suggestions := []string{"Duplicate suggestion"}
	_ = applySuggestionsToSoul(tmpDir, "testuser", "dev", suggestions)

	// Apply again with same suggestion
	_ = applySuggestionsToSoul(tmpDir, "testuser", "dev", suggestions)

	// Verify no duplicate
	soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-dev.md")
	data, _ := os.ReadFile(soulPath)
	content := string(data)

	count := strings.Count(content, "Duplicate suggestion")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of suggestion, got %d", count)
	}
}

func TestApplySuggestionsToSoulAppend(t *testing.T) {
	tmpDir := t.TempDir()

	// Apply first suggestion
	_ = applySuggestionsToSoul(tmpDir, "testuser", "dev", []string{"First suggestion"})

	// Apply second suggestion
	_ = applySuggestionsToSoul(tmpDir, "testuser", "dev", []string{"Second suggestion"})

	// Verify both present
	soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-dev.md")
	data, _ := os.ReadFile(soulPath)
	content := string(data)

	if !containsStr(content, "First suggestion") {
		t.Errorf("missing first suggestion")
	}
	if !containsStr(content, "Second suggestion") {
		t.Errorf("missing second suggestion")
	}
}

// --- GetActiveRolesForStage tests ---

func TestGetActiveRolesForStage(t *testing.T) {
	tests := []struct {
		stage string
		want  []string
	}{
		{"INIT", []string{"ceo"}},
		{"DISCOVERY", []string{"ceo"}},
		{"DESIGN", []string{"ceo", "designer"}},
		{"PLAN", []string{"ceo"}},
		{"EXECUTE", []string{"developer"}},
		{"CHECKPOINT", []string{"developer", "ceo"}},
		{"COMPLETE", []string{"ceo"}},
		{"UNKNOWN", []string{"developer"}}, // fallback
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			got := GetActiveRolesForStage(tt.stage)
			if len(got) != len(tt.want) {
				t.Fatalf("GetActiveRolesForStage(%q) = %v, want %v", tt.stage, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetActiveRolesForStage(%q)[%d] = %q, want %q", tt.stage, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestListUserSoulFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// No souls → empty
	roles := listUserSoulFiles(tmpDir, "nobody")
	if len(roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(roles))
	}

	// Create two soul files
	soulsDir := filepath.Join(tmpDir, ".c4", "souls", "alice")
	os.MkdirAll(soulsDir, 0755)
	os.WriteFile(filepath.Join(soulsDir, "soul-ceo.md"), []byte("# CEO"), 0644)
	os.WriteFile(filepath.Join(soulsDir, "soul-developer.md"), []byte("# Dev"), 0644)
	os.WriteFile(filepath.Join(soulsDir, "not-a-soul.txt"), []byte("ignore"), 0644) // should be ignored

	roles = listUserSoulFiles(tmpDir, "alice")
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d: %v", len(roles), roles)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findStr(s, substr))
}

func findStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
