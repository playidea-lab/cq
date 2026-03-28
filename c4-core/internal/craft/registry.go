package craft

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RegistrySkill represents a skill in the Supabase registry.
type RegistrySkill struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	AuthorID       string   `json:"author_id,omitempty"`
	AuthorName     string   `json:"author_name"`
	LatestVersion  string   `json:"latest_version"`
	DownloadCount  int64    `json:"download_count"`
	Status         string   `json:"status"`
	Tags           []string `json:"tags"`
	CreatedAt      string   `json:"created_at,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// RegistryVersion represents a versioned snapshot of a skill.
type RegistryVersion struct {
	ID         string          `json:"id"`
	SkillID    string          `json:"skill_id"`
	Version    string          `json:"version"`
	Content    string          `json:"content"`
	ExtraFiles json.RawMessage `json:"extra_files,omitempty"`
	Changelog  string          `json:"changelog"`
	CreatedAt  string          `json:"created_at,omitempty"`
}

// TokenFunc returns a JWT token for authenticated requests.
type TokenFunc func() string

// RegistryClient communicates with the Supabase skill_registry tables.
type RegistryClient struct {
	baseURL    string // e.g., https://xxx.supabase.co/rest/v1
	apiKey     string
	tokenFn    TokenFunc
	httpClient *http.Client
}

// NewRegistryClient creates a new registry client.
// tokenFn may be nil for unauthenticated (anon) access.
func NewRegistryClient(supabaseURL, apiKey string, tokenFn TokenFunc) *RegistryClient {
	restURL := strings.TrimRight(supabaseURL, "/")
	if !strings.HasSuffix(restURL, "/rest/v1") {
		restURL += "/rest/v1"
	}
	return &RegistryClient{
		baseURL: restURL,
		apiKey:  apiKey,
		tokenFn: tokenFn,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Search finds a skill by exact name match. Returns nil if not found.
func (r *RegistryClient) Search(name string) (*RegistrySkill, error) {
	filter := "name=eq." + url.QueryEscape(name) + "&status=eq.active&limit=1"
	var skills []RegistrySkill
	if err := r.get("skill_registry", filter, &skills); err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, nil
	}
	return &skills[0], nil
}

// SearchFTS performs full-text search on name and description.
func (r *RegistryClient) SearchFTS(query string) ([]RegistrySkill, error) {
	tsQuery := strings.ReplaceAll(strings.TrimSpace(query), " ", "+")
	filter := "status=eq.active&or=(name.ilike.*" + url.QueryEscape(query) + "*,description.ilike.*" + url.QueryEscape(query) + "*)" +
		"&order=download_count.desc&limit=50"
	// Fallback: if FTS doesn't work well with PostgREST, use ilike pattern matching.
	_ = tsQuery
	var skills []RegistrySkill
	if err := r.get("skill_registry", filter, &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

// FetchVersion retrieves a specific version's content.
func (r *RegistryClient) FetchVersion(skillID, version string) (*RegistryVersion, error) {
	filter := "skill_id=eq." + url.QueryEscape(skillID) + "&version=eq." + url.QueryEscape(version) + "&limit=1"
	var versions []RegistryVersion
	if err := r.get("skill_registry_versions", filter, &versions); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("version %s not found for skill %s", version, skillID)
	}
	return &versions[0], nil
}

// LatestVersion returns the latest_version field for a skill by name.
func (r *RegistryClient) LatestVersion(name string) (string, error) {
	skill, err := r.Search(name)
	if err != nil {
		return "", err
	}
	if skill == nil {
		return "", fmt.Errorf("skill %q not found in registry", name)
	}
	return skill.LatestVersion, nil
}

// Publish uploads a new skill or new version to the registry.
// Requires admin authentication.
func (r *RegistryClient) Publish(skill RegistrySkill, version RegistryVersion) error {
	// Check if skill already exists.
	existing, err := r.Search(skill.Name)
	if err != nil {
		return fmt.Errorf("registry: check existing: %w", err)
	}

	if existing == nil {
		// New skill — insert into skill_registry.
		if err := r.post("skill_registry", skill); err != nil {
			return fmt.Errorf("registry: insert skill: %w", err)
		}
		// Re-fetch to get the generated ID.
		existing, err = r.Search(skill.Name)
		if err != nil || existing == nil {
			return fmt.Errorf("registry: refetch after insert failed")
		}
	} else {
		// Existing skill — update latest_version and updated_at.
		update := map[string]any{
			"latest_version": version.Version,
			"updated_at":     time.Now().UTC().Format(time.RFC3339),
		}
		if skill.Description != "" {
			update["description"] = skill.Description
		}
		if err := r.patch("skill_registry", "name=eq."+url.QueryEscape(skill.Name), update); err != nil {
			return fmt.Errorf("registry: update skill: %w", err)
		}
	}

	// Insert version.
	version.SkillID = existing.ID
	if err := r.post("skill_registry_versions", version); err != nil {
		return fmt.Errorf("registry: insert version: %w", err)
	}
	return nil
}

// IncrementDownload atomically increments the download count via RPC.
func (r *RegistryClient) IncrementDownload(name string) error {
	body := map[string]string{"p_skill_name": name}
	return r.rpc("increment_skill_download", body)
}

// --- HTTP helpers (mirrors cloud.CloudStore pattern) ---

func (r *RegistryClient) get(table, filter string, dest any) error {
	u := r.baseURL + "/" + table
	if filter != "" {
		u += "?" + filter
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	r.setHeaders(req)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", table, resp.StatusCode, string(body))
	}
	if dest != nil {
		return json.NewDecoder(resp.Body).Decode(dest)
	}
	return nil
}

func (r *RegistryClient) post(table string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest("POST", r.baseURL+"/"+table, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	r.setHeaders(req)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

func (r *RegistryClient) patch(table, filter string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	u := r.baseURL + "/" + table + "?" + filter
	req, err := http.NewRequest("PATCH", u, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	r.setHeaders(req)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

func (r *RegistryClient) rpc(funcName string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	u := r.baseURL + "/rpc/" + funcName
	req, err := http.NewRequest("POST", u, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	r.setHeaders(req)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("RPC %s: %w", funcName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("RPC %s: %d %s", funcName, resp.StatusCode, string(respBody))
	}
	return nil
}

func (r *RegistryClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", r.apiKey)
	if r.tokenFn != nil {
		token := r.tokenFn()
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

// IsCQSource reports whether a source string is a CQ registry source.
// e.g., "cq:code-review@1.2.0"
func IsCQSource(source string) bool {
	return strings.HasPrefix(source, "cq:")
}

// ParseCQSource extracts name and version from "cq:<name>@<version>".
func ParseCQSource(source string) (name, version string, err error) {
	s := strings.TrimPrefix(source, "cq:")
	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid cq source format %q (expected cq:<name>@<version>)", source)
	}
	return parts[0], parts[1], nil
}

// FormatCQSource creates a source string from name and version.
func FormatCQSource(name, version string) string {
	return "cq:" + name + "@" + version
}
