package ontology

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CloudStore reads and writes L1/L2 ontology data via Supabase PostgREST.
// Tables: c4_user_ontology (L1), c4_project_ontology (L2).
// Both tables have columns: user_id/project_id (PK), schema_jsonb (JSONB), version (INT), updated_at (TIMESTAMPTZ).
type CloudStore struct {
	baseURL    string // Supabase PostgREST URL (e.g., https://xxx.supabase.co/rest/v1)
	apiKey     string // anon key
	token      func() string // returns current Bearer token
	refreshFn  func() error  // called once on 401 before retry (nil = static)
	httpClient *http.Client
}

// CloudTokenProvider is the minimal interface that CloudStore needs for auth.
type CloudTokenProvider interface {
	Token() string
	Refresh() (string, error)
}

// NewCloudStore creates an ontology CloudStore.
// tp must implement CloudTokenProvider (e.g., *cloud.TokenProvider via adapter).
func NewCloudStore(baseURL, apiKey string, tp CloudTokenProvider) *CloudStore {
	cs := &CloudStore{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		token:   tp.Token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	cs.refreshFn = func() error {
		_, err := tp.Refresh()
		return err
	}
	return cs
}

// =========================================================================
// L1 — user ontology (c4_user_ontology)
// =========================================================================

// cloudUserRow maps to the c4_user_ontology Supabase table.
type cloudUserRow struct {
	UserID     string          `json:"user_id"`
	SchemaJSONB json.RawMessage `json:"schema_jsonb"`
	Version    int             `json:"version"`
	UpdatedAt  string          `json:"updated_at,omitempty"`
}

// CloudLoad fetches the L1 ontology for the given userID from Supabase.
// Returns an empty Ontology if no row exists.
func (cs *CloudStore) CloudLoad(userID string) (*Ontology, error) {
	if userID == "" {
		return nil, fmt.Errorf("ontology.CloudLoad: userID required")
	}

	filter := "user_id=eq." + url.QueryEscape(userID) + "&select=schema_jsonb,version,updated_at&limit=1"

	var rows []cloudUserRow
	if err := cs.get("c4_user_ontology", filter, &rows); err != nil {
		return nil, fmt.Errorf("CloudLoad: %w", err)
	}
	if len(rows) == 0 {
		return &Ontology{Version: defaultVersion}, nil
	}

	row := rows[0]
	var schema CoreSchema
	if err := json.Unmarshal(row.SchemaJSONB, &schema); err != nil {
		return nil, fmt.Errorf("CloudLoad: decode schema_jsonb: %w", err)
	}

	o := &Ontology{
		Version: defaultVersion,
		Schema:  schema,
	}
	if row.Version > 0 {
		o.Version = fmt.Sprintf("1.%d.0", row.Version)
	}
	return o, nil
}

// CloudSave upserts the L1 ontology for the given userID to Supabase.
func (cs *CloudStore) CloudSave(userID string, o *Ontology) error {
	if userID == "" {
		return fmt.Errorf("ontology.CloudSave: userID required")
	}
	if o == nil {
		return fmt.Errorf("ontology.CloudSave: nil ontology")
	}

	schemaJSON, err := json.Marshal(o.Schema)
	if err != nil {
		return fmt.Errorf("CloudSave: marshal schema: %w", err)
	}

	row := cloudUserRow{
		UserID:      userID,
		SchemaJSONB: json.RawMessage(schemaJSON),
		Version:     1,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	return cs.upsert("c4_user_ontology", row)
}

// =========================================================================
// L2 — project ontology (c4_project_ontology)
// =========================================================================

// cloudProjectRow maps to the c4_project_ontology Supabase table.
type cloudProjectRow struct {
	ProjectID   string          `json:"project_id"`
	SchemaJSONB json.RawMessage `json:"schema_jsonb"`
	Version     int             `json:"version"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

// CloudLoadProject fetches the L2 project ontology for the given projectID.
// Returns an empty ProjectOntology if no row exists.
func (cs *CloudStore) CloudLoadProject(projectID string) (*ProjectOntology, error) {
	if projectID == "" {
		return nil, fmt.Errorf("ontology.CloudLoadProject: projectID required")
	}

	filter := "project_id=eq." + url.QueryEscape(projectID) + "&select=schema_jsonb,version,updated_at&limit=1"

	var rows []cloudProjectRow
	if err := cs.get("c4_project_ontology", filter, &rows); err != nil {
		return nil, fmt.Errorf("CloudLoadProject: %w", err)
	}
	if len(rows) == 0 {
		return &ProjectOntology{Version: defaultVersion}, nil
	}

	row := rows[0]
	var schema CoreSchema
	if err := json.Unmarshal(row.SchemaJSONB, &schema); err != nil {
		return nil, fmt.Errorf("CloudLoadProject: decode schema_jsonb: %w", err)
	}

	po := &ProjectOntology{
		Version: defaultVersion,
		Schema:  schema,
	}
	if row.Version > 0 {
		po.Version = fmt.Sprintf("1.%d.0", row.Version)
	}
	return po, nil
}

// CloudSaveProject upserts the L2 project ontology for the given projectID to Supabase.
func (cs *CloudStore) CloudSaveProject(projectID string, o *ProjectOntology) error {
	if projectID == "" {
		return fmt.Errorf("ontology.CloudSaveProject: projectID required")
	}
	if o == nil {
		return fmt.Errorf("ontology.CloudSaveProject: nil ontology")
	}

	schemaJSON, err := json.Marshal(o.Schema)
	if err != nil {
		return fmt.Errorf("CloudSaveProject: marshal schema: %w", err)
	}

	row := cloudProjectRow{
		ProjectID:   projectID,
		SchemaJSONB: json.RawMessage(schemaJSON),
		Version:     1,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	return cs.upsert("c4_project_ontology", row)
}

// =========================================================================
// PostgREST HTTP helpers
// =========================================================================

func (cs *CloudStore) get(table, filter string, dest any) error {
	reqURL := cs.baseURL + "/" + table
	if filter != "" {
		reqURL += "?" + filter
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return err
		}
		cs.setHeaders(req)

		resp, err := cs.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if cs.refreshFn != nil {
				if err := cs.refreshFn(); err == nil {
					continue
				}
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("GET %s: %d %s", table, resp.StatusCode, string(body))
		}

		if dest != nil {
			if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
				return fmt.Errorf("decode %s: %w", table, err)
			}
		}
		return nil
	}
	return nil
}

func (cs *CloudStore) upsert(table string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("POST", cs.baseURL+"/"+table, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		cs.setHeaders(req)
		req.Header.Set("Prefer", "return=minimal,resolution=merge-duplicates")

		resp, err := cs.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("POST %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if cs.refreshFn != nil {
				if err := cs.refreshFn(); err == nil {
					continue
				}
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
		}
		return nil
	}
	return nil
}

func (cs *CloudStore) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", cs.apiKey)
	req.Header.Set("Authorization", "Bearer "+cs.token())
}
