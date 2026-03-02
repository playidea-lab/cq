package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	"gopkg.in/yaml.v3"
)

// =========================================================================
// Persona Handler Tests
// =========================================================================

func TestPersonaStats_ListAll(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterPersonaHandlers(reg, store)

	result, err := reg.Call("c4_persona_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	personas := m["personas"].([]map[string]any)
	if personas == nil {
		t.Fatal("personas should not be nil")
	}
}

func TestPersonaStats_Specific(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Seed a completed task with persona
	task := &Task{ID: "T-100-0", Title: "test task", DoD: "done", Status: "done"}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	store.db.Exec(`INSERT INTO persona_stats (persona_id, task_id, outcome, created_at)
		VALUES ('code-reviewer', 'T-100-0', 'approved', datetime('now'))`)

	reg := mcp.NewRegistry()
	RegisterPersonaHandlers(reg, store)

	result, err := reg.Call("c4_persona_stats", json.RawMessage(`{"persona_id": "code-reviewer"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if stats["persona_id"] != "code-reviewer" {
		t.Errorf("persona_id = %v, want code-reviewer", stats["persona_id"])
	}
}

func TestPersonaStats_PathTraversal(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterPersonaHandlers(reg, store)

	_, err := reg.Call("c4_persona_stats", json.RawMessage(`{"persona_id": "../etc/passwd"}`))
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "invalid persona_id") {
		t.Errorf("error = %v, want 'invalid persona_id'", err)
	}
}

func TestPersonaEvolve_NoHistory(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterPersonaHandlers(reg, store)

	result, err := reg.Call("c4_persona_evolve", json.RawMessage(`{"persona_id": "test-persona"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	suggestions := m["suggestions"].([]string)
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for persona with no history, got %d", len(suggestions))
	}
	if m["message"] != "No task history found. Complete some tasks first." {
		t.Errorf("message = %v", m["message"])
	}
}

func TestPersonaEvolve_PathTraversal(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	reg := mcp.NewRegistry()
	RegisterPersonaHandlers(reg, store)

	_, err := reg.Call("c4_persona_evolve", json.RawMessage(`{"persona_id": "../../hack"}`))
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

// =========================================================================
// Team Handler Tests (c4_whoami)
// =========================================================================

func TestWhoami_EmptyTeam(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterTeamHandlers(reg, tmpDir)

	result, err := reg.Call("c4_whoami", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	members := m["members"].([]map[string]any)
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestWhoami_RegisterUser(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	reg := mcp.NewRegistry()
	RegisterTeamHandlers(reg, tmpDir)

	result, err := reg.Call("c4_whoami", json.RawMessage(`{
		"username": "changmin", "role": "developer",
		"roles": ["developer", "ceo"], "active_persona": "code-reviewer"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["username"] != "changmin" {
		t.Errorf("username = %v, want changmin", m["username"])
	}
	if m["role"] != "developer" {
		t.Errorf("role = %v, want developer", m["role"])
	}
	if m["active_persona"] != "code-reviewer" {
		t.Errorf("active_persona = %v, want code-reviewer", m["active_persona"])
	}

	// Verify file was written
	teamPath := filepath.Join(tmpDir, ".c4", "team.yaml")
	if _, err := os.Stat(teamPath); os.IsNotExist(err) {
		t.Error("team.yaml was not created")
	}
}

func TestWhoami_InvalidUsername(t *testing.T) {
	tmpDir := t.TempDir()
	reg := mcp.NewRegistry()
	RegisterTeamHandlers(reg, tmpDir)

	_, err := reg.Call("c4_whoami", json.RawMessage(`{"username": "../hack"}`))
	if err == nil {
		t.Fatal("expected error for invalid username")
	}
}

// =========================================================================
// Helper Function Tests
// =========================================================================

func TestAnalyzePatternsForSuggestions(t *testing.T) {
	tests := []struct {
		name     string
		stats    map[string]any
		total    int
		wantMin  int
		contains string
	}{
		{
			name: "high rejection",
			stats: map[string]any{
				"outcomes": map[string]int{"approved": 5, "rejected": 5},
			},
			total:    10,
			wantMin:  1,
			contains: "rejection rate",
		},
		{
			name: "low review score",
			stats: map[string]any{
				"avg_review_score": 0.5,
				"outcomes":         map[string]int{},
			},
			total:    5,
			wantMin:  1,
			contains: "review score",
		},
		{
			name: "experienced persona",
			stats: map[string]any{
				"avg_review_score": 0.9,
				"outcomes":         map[string]int{"approved": 15},
			},
			total:    15,
			wantMin:  1,
			contains: "specializing",
		},
		{
			name: "good stats, low count",
			stats: map[string]any{
				"avg_review_score": 0.95,
				"outcomes":         map[string]int{"approved": 3},
			},
			total:   3,
			wantMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := analyzePatternsForSuggestions(tt.stats, tt.total)
			if len(suggestions) < tt.wantMin {
				t.Errorf("got %d suggestions, want >= %d", len(suggestions), tt.wantMin)
			}
			if tt.contains != "" {
				found := false
				for _, s := range suggestions {
					if strings.Contains(strings.ToLower(s), strings.ToLower(tt.contains)) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no suggestion containing %q: %v", tt.contains, suggestions)
				}
			}
		})
	}
}

func TestListUserSoulFiles_Persona(t *testing.T) {
	tmpDir := t.TempDir()

	// Create soul files
	soulsDir := filepath.Join(tmpDir, ".c4", "souls", "testuser")
	os.MkdirAll(soulsDir, 0755)
	os.WriteFile(filepath.Join(soulsDir, "soul-developer.md"), []byte("# Soul"), 0644)
	os.WriteFile(filepath.Join(soulsDir, "soul-ceo.md"), []byte("# Soul"), 0644)
	os.WriteFile(filepath.Join(soulsDir, "other.txt"), []byte("not a soul"), 0644)

	roles := listUserSoulFiles(tmpDir, "testuser")
	if len(roles) != 2 {
		t.Errorf("got %d roles, want 2: %v", len(roles), roles)
	}
}

func TestListUserSoulFiles_NoDir_Persona(t *testing.T) {
	tmpDir := t.TempDir()
	roles := listUserSoulFiles(tmpDir, "nonexistent")
	if len(roles) != 0 {
		t.Errorf("got %d roles, want 0", len(roles))
	}
}

func TestGetActiveUsername(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

	teamYAML := `members:
  alice:
    role: developer
`
	os.WriteFile(filepath.Join(tmpDir, ".c4", "team.yaml"), []byte(teamYAML), 0644)

	name := getActiveUsername(tmpDir)
	if name != "alice" {
		t.Errorf("username = %q, want alice", name)
	}
}

func TestGetActiveUsername_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	name := getActiveUsername(tmpDir)
	if name != "" {
		t.Errorf("username = %q, want empty", name)
	}
}

func TestGetActiveUsername_ConfigKey(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

	// team.yaml with two members
	teamYAML := `members:
  alice:
    role: developer
  bob:
    role: backend
`
	os.WriteFile(filepath.Join(tmpDir, ".c4", "team.yaml"), []byte(teamYAML), 0644)

	// config.yaml with explicit active_username
	configYAML := "active_username: bob\n"
	os.WriteFile(filepath.Join(tmpDir, ".c4", "config.yaml"), []byte(configYAML), 0644)

	name := getActiveUsername(tmpDir)
	if name != "bob" {
		t.Errorf("username = %q, want bob (from config.yaml)", name)
	}
}

func TestGetActiveUsername_SingleMember(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

	// No config.yaml, single member in team.yaml
	teamYAML := `members:
  carol:
    role: developer
`
	os.WriteFile(filepath.Join(tmpDir, ".c4", "team.yaml"), []byte(teamYAML), 0644)

	name := getActiveUsername(tmpDir)
	if name != "carol" {
		t.Errorf("username = %q, want carol (single-member fallback)", name)
	}
}

func TestGetActiveUsername_MultiMember_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

	// Multiple members, no config.yaml active_username
	teamYAML := `members:
  alice:
    role: developer
  bob:
    role: backend
`
	os.WriteFile(filepath.Join(tmpDir, ".c4", "team.yaml"), []byte(teamYAML), 0644)

	name := getActiveUsername(tmpDir)
	if name != "" {
		t.Errorf("username = %q, want empty for multi-member without config key", name)
	}
}

func TestTeamMemberCloudUID_RoundTrip(t *testing.T) {
	original := TeamMember{
		Role:          "developer",
		Roles:         []string{"developer", "ceo"},
		Personas:      []string{"code-reviewer"},
		ActivePersona: "code-reviewer",
		CloudUID:      "supabase-uid-12345",
	}

	data, err := yamlMarshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var result TeamMember
	if err := yamlUnmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.CloudUID != original.CloudUID {
		t.Errorf("CloudUID = %q, want %q", result.CloudUID, original.CloudUID)
	}
	if result.Role != original.Role {
		t.Errorf("Role = %q, want %q", result.Role, original.Role)
	}
	if len(result.Roles) != len(original.Roles) {
		t.Errorf("Roles len = %d, want %d", len(result.Roles), len(original.Roles))
	}
	if result.ActivePersona != original.ActivePersona {
		t.Errorf("ActivePersona = %q, want %q", result.ActivePersona, original.ActivePersona)
	}
}

// yamlMarshal/yamlUnmarshal are thin wrappers to allow test package to use yaml
// without importing gopkg.in/yaml.v3 directly (already imported in persona.go).
func yamlMarshal(v any) ([]byte, error) {
	team := TeamConfig{Members: map[string]TeamMember{"testuser": v.(TeamMember)}}
	data, err := yaml.Marshal(team)
	return data, err
}

func yamlUnmarshal(data []byte, v *TeamMember) error {
	var team TeamConfig
	if err := yaml.Unmarshal(data, &team); err != nil {
		return err
	}
	if m, ok := team.Members["testuser"]; ok {
		*v = m
	}
	return nil
}

func TestApplySuggestionsToSoul(t *testing.T) {
	tmpDir := t.TempDir()

	err := applySuggestionsToSoul(tmpDir, "testuser", "developer",
		[]string{"Improve test coverage", "Add documentation"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-developer.md")
	data, err := os.ReadFile(soulPath)
	if err != nil {
		t.Fatalf("read soul: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Improve test coverage") {
		t.Error("soul file should contain 'Improve test coverage'")
	}
	if !strings.Contains(content, "Add documentation") {
		t.Error("soul file should contain 'Add documentation'")
	}
}

func TestApplySuggestionsToSoul_Dedup(t *testing.T) {
	tmpDir := t.TempDir()

	// First application
	err := applySuggestionsToSoul(tmpDir, "testuser", "developer", []string{"Be consistent"})
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Second application with same suggestion
	err = applySuggestionsToSoul(tmpDir, "testuser", "developer", []string{"Be consistent"})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}

	soulPath := filepath.Join(tmpDir, ".c4", "souls", "testuser", "soul-developer.md")
	data, _ := os.ReadFile(soulPath)
	count := strings.Count(string(data), "Be consistent")
	if count != 1 {
		t.Errorf("suggestion appears %d times, want 1 (dedup)", count)
	}
}

func TestApplySuggestionsToSoul_InvalidUsername(t *testing.T) {
	tmpDir := t.TempDir()

	err := applySuggestionsToSoul(tmpDir, "../hack", "developer", []string{"test"})
	if err == nil {
		t.Fatal("expected error for path traversal username")
	}
}

func TestGetActivePersonaForUser(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

	teamYAML := `members:
  bob:
    role: backend
    active_persona: architect
`
	os.WriteFile(filepath.Join(tmpDir, ".c4", "team.yaml"), []byte(teamYAML), 0644)

	persona := getActivePersonaForUser(tmpDir, "bob")
	if persona != "architect" {
		t.Errorf("persona = %q, want architect", persona)
	}

	persona = getActivePersonaForUser(tmpDir, "unknown")
	if persona != "" {
		t.Errorf("persona = %q, want empty for unknown user", persona)
	}
}
