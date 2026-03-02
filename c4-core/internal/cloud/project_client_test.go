package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestListProjects_Success simulates the 2-request flow and verifies projects are returned.
func TestListProjects_Success(t *testing.T) {
	// Track which requests have been made.
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "c4_project_members"):
			// Request 1: memberships
			// user_id=eq.xxx is passed as raw query param (PostgREST filter style)
			if !strings.Contains(r.URL.RawQuery, "user_id=eq.") {
				t.Errorf("expected user_id=eq. filter in query, got: %s", r.URL.RawQuery)
			}
			members := []map[string]string{
				{"project_id": "proj-uuid-1"},
				{"project_id": "proj-uuid-2"},
			}
			json.NewEncoder(w).Encode(members)

		case strings.Contains(r.URL.Path, "c4_projects"):
			// Request 2: projects by IDs
			if !strings.Contains(r.URL.RawQuery, "id=in.") {
				t.Errorf("expected id=in.() filter, got: %s", r.URL.RawQuery)
			}
			projects := []Project{
				{ID: "proj-uuid-1", Name: "Project Alpha", OwnerID: "user-1", CreatedAt: "2024-01-01T00:00:00Z"},
				{ID: "proj-uuid-2", Name: "Project Beta", OwnerID: "user-1", CreatedAt: "2024-01-02T00:00:00Z"},
			}
			json.NewEncoder(w).Encode(projects)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewProjectClient(srv.URL, "anon-key", "access-token")
	projects, err := client.ListProjects("user-id-123")
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "Project Alpha" {
		t.Errorf("expected 'Project Alpha', got %q", projects[0].Name)
	}
	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}
}

// TestListProjects_Empty verifies that zero memberships returns empty slice (not error).
func TestListProjects_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "c4_project_members") {
			// Return empty array.
			w.Write([]byte("[]"))
			return
		}
		t.Errorf("unexpected request to: %s (should not fetch projects when no memberships)", r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewProjectClient(srv.URL, "anon-key", "access-token")
	projects, err := client.ListProjects("user-no-projects")
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(projects))
	}
}

// TestCreateProject_Success verifies POST with Prefer: return=representation.
func TestCreateProject_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Prefer") != "return=representation" {
			t.Errorf("expected Prefer: return=representation, got %q", r.Header.Get("Prefer"))
		}
		if r.Header.Get("apikey") == "" {
			t.Error("missing apikey header")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		if body["name"] != "My Project" {
			t.Errorf("expected name 'My Project', got %q", body["name"])
		}
		if body["owner_id"] != "owner-uuid" {
			t.Errorf("expected owner_id 'owner-uuid', got %q", body["owner_id"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		projects := []Project{
			{ID: "new-proj-uuid", Name: "My Project", OwnerID: "owner-uuid", CreatedAt: "2024-01-01T00:00:00Z"},
		}
		json.NewEncoder(w).Encode(projects)
	}))
	defer srv.Close()

	client := NewProjectClient(srv.URL, "anon-key", "access-token")
	project, err := client.CreateProject("My Project", "owner-uuid")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if project.ID != "new-proj-uuid" {
		t.Errorf("expected ID 'new-proj-uuid', got %q", project.ID)
	}
	if project.Name != "My Project" {
		t.Errorf("expected Name 'My Project', got %q", project.Name)
	}
}

// TestSetActiveProject_Roundtrip verifies that existing config fields are preserved
// and active_project_id is correctly set within the cloud section.
func TestSetActiveProject_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o700); err != nil {
		t.Fatalf("creating .c4 dir: %v", err)
	}

	// Write an existing config with pre-existing fields.
	existing := `workers: 3
cloud:
  url: https://example.supabase.co
  anon_key: test-anon-key
  mode: local-first
hub:
  enabled: false
`
	configPath := filepath.Join(c4Dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("writing existing config: %v", err)
	}

	if err := SetActiveProject(dir, "proj-uuid-123"); err != nil {
		t.Fatalf("SetActiveProject error: %v", err)
	}

	// Read back and verify.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config after update: %v", err)
	}

	content := string(data)

	// Verify active_project_id was written.
	if !strings.Contains(content, "active_project_id: proj-uuid-123") {
		t.Errorf("active_project_id not found in config:\n%s", content)
	}

	// Verify existing fields are preserved.
	if !strings.Contains(content, "url: https://example.supabase.co") {
		t.Errorf("url field missing after update:\n%s", content)
	}
	if !strings.Contains(content, "anon_key: test-anon-key") {
		t.Errorf("anon_key field missing after update:\n%s", content)
	}
	if !strings.Contains(content, "mode: local-first") {
		t.Errorf("mode field missing after update:\n%s", content)
	}
	if !strings.Contains(content, "workers: 3") {
		t.Errorf("workers field missing after update:\n%s", content)
	}

	// Verify valid YAML structure.
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parsed config is invalid YAML: %v", err)
	}

	cloud, ok := parsed["cloud"].(map[string]interface{})
	if !ok {
		t.Fatalf("cloud section missing or invalid type")
	}
	if cloud["active_project_id"] != "proj-uuid-123" {
		t.Errorf("active_project_id = %v, want 'proj-uuid-123'", cloud["active_project_id"])
	}
}

// TestSetActiveProject_EmptyConfig verifies behavior when no config.yaml exists.
func TestSetActiveProject_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o700); err != nil {
		t.Fatalf("creating .c4 dir: %v", err)
	}

	if err := SetActiveProject(dir, "new-project-id"); err != nil {
		t.Fatalf("SetActiveProject on empty config: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(c4Dir, "config.yaml"))
	if err != nil {
		t.Fatalf("reading created config: %v", err)
	}

	if !strings.Contains(string(data), "active_project_id: new-project-id") {
		t.Errorf("active_project_id not set in new config:\n%s", string(data))
	}
}
