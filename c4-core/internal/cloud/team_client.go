package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TeamClient wraps PostgREST calls for team member management.
type TeamClient struct {
	supabaseURL string
	anonKey     string
	accessToken string
	httpClient  *http.Client
}

// NewTeamClient creates a new TeamClient for the given Supabase project.
func NewTeamClient(supabaseURL, anonKey, accessToken string) *TeamClient {
	return &TeamClient{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		anonKey:     anonKey,
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Member represents a project team member with profile information.
type Member struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// InviteResult holds the result of an invite-or-add operation.
type InviteResult struct {
	Status string // "added" | "invited" | "already_member"
}

// teamMemberRow is a row from c4_project_members.
type teamMemberRow struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// profileRow is a row from c4_profiles.
type profileRow struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// inviteOrPendResponse is the raw RPC response from c4_invite_or_pend.
type inviteOrPendResponse struct {
	Status string `json:"c4_invite_or_pend"`
}

// doTeamRequest performs an HTTP request with standard Supabase headers.
func (c *TeamClient) doTeamRequest(method, url string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
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

// ListMembers returns all members of a project with their profile info.
// Uses a 2-request approach:
//  1. GET /rest/v1/c4_project_members?project_id=eq.{projectID}&select=user_id,role
//  2. GET /rest/v1/c4_profiles?user_id=in.({ids})&select=user_id,email,display_name
//
// Merges results by user_id.
func (c *TeamClient) ListMembers(projectID string) ([]Member, error) {
	// Request 1: get members for this project.
	membersURL := c.supabaseURL + "/rest/v1/c4_project_members?project_id=eq." + projectID + "&select=user_id,role"
	resp, err := c.doTeamRequest(http.MethodGet, membersURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching project members: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing members (HTTP %d): %w", resp.StatusCode, errors.New(string(body)))
	}

	var memberRows []teamMemberRow
	if err := json.NewDecoder(resp.Body).Decode(&memberRows); err != nil {
		return nil, fmt.Errorf("decoding members: %w", err)
	}

	if len(memberRows) == 0 {
		return []Member{}, nil
	}

	// Build role map and collect user IDs.
	roleByUserID := make(map[string]string, len(memberRows))
	ids := make([]string, 0, len(memberRows))
	for _, m := range memberRows {
		if m.UserID != "" {
			roleByUserID[m.UserID] = m.Role
			ids = append(ids, m.UserID)
		}
	}
	if len(ids) == 0 {
		return []Member{}, nil
	}

	// Request 2: fetch profiles by user IDs.
	profilesURL := c.supabaseURL + "/rest/v1/c4_profiles?user_id=in.(" + strings.Join(ids, ",") + ")&select=user_id,email,display_name"
	resp2, err := c.doTeamRequest(http.MethodGet, profilesURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching profiles: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp2.Body)
		return nil, fmt.Errorf("fetching profiles (HTTP %d): %w", resp2.StatusCode, errors.New(string(body)))
	}

	var profiles []profileRow
	if err := json.NewDecoder(resp2.Body).Decode(&profiles); err != nil {
		return nil, fmt.Errorf("decoding profiles: %w", err)
	}

	// Merge profiles with role info.
	profileByUserID := make(map[string]profileRow, len(profiles))
	for _, p := range profiles {
		profileByUserID[p.UserID] = p
	}

	members := make([]Member, 0, len(ids))
	for _, userID := range ids {
		m := Member{
			UserID: userID,
			Role:   roleByUserID[userID],
		}
		if p, ok := profileByUserID[userID]; ok {
			m.Email = p.Email
			m.DisplayName = p.DisplayName
			if m.DisplayName == "" {
				m.DisplayName = p.Email
			}
		}
		members = append(members, m)
	}

	return members, nil
}

// InviteOrAdd calls the c4_invite_or_pend RPC to invite a user by email.
// Returns InviteResult with Status: "added", "invited", or "already_member".
func (c *TeamClient) InviteOrAdd(projectID, email string) (InviteResult, error) {
	payload := map[string]string{
		"p_project_id": projectID,
		"p_email":      email,
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return InviteResult{}, fmt.Errorf("marshaling invite request: %w", err)
	}

	rpcURL := c.supabaseURL + "/rest/v1/rpc/c4_invite_or_pend"
	resp, err := c.doTeamRequest(http.MethodPost, rpcURL, bodyJSON, nil)
	if err != nil {
		return InviteResult{}, fmt.Errorf("calling c4_invite_or_pend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return InviteResult{}, fmt.Errorf("invite RPC (HTTP %d): %w", resp.StatusCode, errors.New(string(body)))
	}

	// The RPC returns a plain JSON string (e.g. "added" or "invited").
	var status string
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return InviteResult{}, fmt.Errorf("decoding invite result: %w", err)
	}

	return InviteResult{Status: status}, nil
}

// RemoveMember removes a user from a project.
// DELETE /rest/v1/c4_project_members?project_id=eq.{projectID}&user_id=eq.{userID}
func (c *TeamClient) RemoveMember(projectID, userID string) error {
	deleteURL := c.supabaseURL + "/rest/v1/c4_project_members?project_id=eq." + projectID + "&user_id=eq." + userID
	resp, err := c.doTeamRequest(http.MethodDelete, deleteURL, nil, nil)
	if err != nil {
		return fmt.Errorf("removing member: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("removing member (HTTP %d): %w", resp.StatusCode, errors.New(string(body)))
	}

	return nil
}
