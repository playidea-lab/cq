package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectClient wraps PostgREST calls for c4_projects + c4_project_members.
type ProjectClient struct {
	supabaseURL string
	anonKey     string
	accessToken string
	httpClient  *http.Client
}

// NewProjectClient creates a new ProjectClient for the given Supabase project.
func NewProjectClient(supabaseURL, anonKey, accessToken string) *ProjectClient {
	return &ProjectClient{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		anonKey:     anonKey,
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Project represents a CQ cloud project.
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OwnerID   string `json:"owner_id"`
	CreatedAt string `json:"created_at"`
}

// projectMemberRow represents a row from c4_project_members.
type projectMemberRow struct {
	ProjectID string `json:"project_id"`
}

// projectErrorResponse is the error body from PostgREST endpoints.
type projectErrorResponse struct {
	Message string `json:"message"`
	Details string `json:"details"`
	Hint    string `json:"hint"`
	Code    string `json:"code"`
}

// doRequest performs an HTTP request with standard Supabase headers and returns
// the response body. The caller is responsible for closing resp.Body.
func (c *ProjectClient) doRequest(method, url string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return c.httpClient.Do(req)
}

// ListProjects returns projects the current user is a member of.
// Uses a 2-request approach to avoid PostgREST FK embed complexity:
//  1. GET /rest/v1/c4_project_members?user_id=eq.{userID}&select=project_id
//  2. GET /rest/v1/c4_projects?id=in.({ids})&order=created_at.desc
func (c *ProjectClient) ListProjects(userID string) ([]Project, error) {
	// Request 1: get project IDs for this user.
	membersURL := c.supabaseURL + "/rest/v1/c4_project_members?user_id=eq." + url.QueryEscape(userID) + "&select=project_id"
	resp, err := c.doRequest(http.MethodGet, membersURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching project memberships: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("listing memberships (HTTP %d): %w", resp.StatusCode, errors.New(string(body)))
	}

	var members []projectMemberRow
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, fmt.Errorf("decoding memberships: %w", err)
	}

	if len(members) == 0 {
		return []Project{}, nil
	}

	// Build comma-separated URL-encoded IDs for the IN clause.
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m.ProjectID != "" {
			ids = append(ids, url.QueryEscape(m.ProjectID))
		}
	}
	if len(ids) == 0 {
		return []Project{}, nil
	}

	// Request 2: fetch projects by IDs.
	projectsURL := c.supabaseURL + "/rest/v1/c4_projects?id=in.(" + strings.Join(ids, ",") + ")&order=created_at.desc"
	resp2, err := c.doRequest(http.MethodGet, projectsURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching projects: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))
		return nil, fmt.Errorf("fetching projects (HTTP %d): %w", resp2.StatusCode, errors.New(string(body)))
	}

	var projects []Project
	if err := json.NewDecoder(resp2.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("decoding projects: %w", err)
	}

	return projects, nil
}

// CreateProject creates a new project owned by the current user.
// Uses POST /rest/v1/c4_projects with Prefer: return=representation.
func (c *ProjectClient) CreateProject(name, ownerID string) (*Project, error) {
	payload := map[string]string{
		"name":     name,
		"owner_id": ownerID,
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling project request: %w", err)
	}

	projectsURL := c.supabaseURL + "/rest/v1/c4_projects"
	resp, err := c.doRequest(http.MethodPost, projectsURL, bodyJSON, map[string]string{
		"Prefer": "return=representation",
	})
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var errResp projectErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp) //nolint:errcheck
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Details
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("creating project: %w", errors.New(msg))
	}

	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("decoding created project: %w", err)
	}
	if len(projects) == 0 {
		return nil, errors.New("no project returned after creation")
	}

	return &projects[0], nil
}

// SetActiveProject writes cloud.active_project_id to .c4/config.yaml.
// Uses yaml.Node round-trip to preserve existing config fields.
func SetActiveProject(projectDir, projectID string) error {
	configPath := filepath.Join(projectDir, ".c4", "config.yaml")

	// Ensure .c4 directory exists.
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("creating .c4 directory: %w", err)
	}

	// Read existing config (or start with empty).
	var root yaml.Node
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config.yaml: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing config.yaml: %w", err)
		}
	}

	// Ensure we have a document node wrapping a mapping node.
	if root.Kind == 0 {
		// Empty file: create a document + mapping node.
		root = yaml.Node{Kind: yaml.DocumentNode}
		root.Content = []*yaml.Node{
			{Kind: yaml.MappingNode, Tag: "!!map"},
		}
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return errors.New("unexpected config.yaml structure")
	}
	docContent := root.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return errors.New("config.yaml root is not a mapping")
	}

	// Find or create the cloud: section.
	cloudSection := findMappingValue(docContent, "cloud")
	if cloudSection == nil {
		// Append cloud: section.
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "cloud", Tag: "!!str"}
		cloudSection = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		docContent.Content = append(docContent.Content, keyNode, cloudSection)
	}

	// Set or update active_project_id within the cloud section.
	existing := findMappingValue(cloudSection, "active_project_id")
	if existing != nil {
		existing.Value = projectID
	} else {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "active_project_id", Tag: "!!str"}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: projectID, Tag: "!!str"}
		cloudSection.Content = append(cloudSection.Content, keyNode, valNode)
	}

	// Atomic write: temp file + rename.
	dir := filepath.Dir(configPath)
	tmp, err := os.CreateTemp(dir, "config.yaml.*")
	if err != nil {
		return fmt.Errorf("creating temp config file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		os.Remove(tmpName) //nolint:errcheck
	}()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		tmp.Close()
		return fmt.Errorf("encoding config.yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		return fmt.Errorf("closing yaml encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, configPath); err != nil {
		return fmt.Errorf("writing config.yaml: %w", err)
	}

	return nil
}
