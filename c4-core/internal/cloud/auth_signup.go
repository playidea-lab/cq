package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// AuthSession holds credentials returned after signup/login.
type AuthSession struct {
	UserID       string
	Email        string
	AccessToken  string
	RefreshToken string
}

// signupResponse is the response body from Supabase /auth/v1/signup.
type signupResponse struct {
	User         signupUser `json:"user"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
}

// signupUser is the user object within the signup response.
type signupUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// signupErrorResponse is the error body from Supabase auth endpoints (v2 format).
type signupErrorResponse struct {
	Code    string `json:"error_code"`
	Message string `json:"msg"`
	Error   string `json:"error"`
}

// SignUpWithEmail creates a new Supabase auth user with email and password.
// Returns AuthSession on success.
// Returns an error wrapping "email already in use" on 400 (email_exists),
// or a descriptive error on 422 (weak_password).
func (c *AuthClient) SignUpWithEmail(email, password string) (*AuthSession, error) {
	body := map[string]string{
		"email":    email,
		"password": password,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling signup request: %w", err)
	}

	url := c.supabaseURL + "/auth/v1/signup"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating signup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signup request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		var errResp signupErrorResponse
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errResp) //nolint:errcheck
		if errResp.Code == "email_exists" || errResp.Code == "user_already_exists" {
			return nil, errors.New("email already in use")
		}
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		if msg == "" {
			msg = "bad request"
		}
		return nil, fmt.Errorf("signup failed (HTTP 400): %w", errors.New(msg))
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		var errResp signupErrorResponse
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errResp) //nolint:errcheck
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		if msg == "" {
			msg = "password does not meet requirements"
		}
		return nil, fmt.Errorf("weak password: %w", errors.New(msg))
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp signupErrorResponse
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errResp) //nolint:errcheck
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		return nil, fmt.Errorf("signup failed (HTTP %d): %w", resp.StatusCode, errors.New(msg))
	}

	var signupResp signupResponse
	if err := json.NewDecoder(resp.Body).Decode(&signupResp); err != nil {
		return nil, fmt.Errorf("decoding signup response: %w", err)
	}

	return &AuthSession{
		UserID:       signupResp.User.ID,
		Email:        signupResp.User.Email,
		AccessToken:  signupResp.AccessToken,
		RefreshToken: signupResp.RefreshToken,
	}, nil
}

// UpsertProfile creates or updates a c4_profiles row for the given session,
// then resolves any pending invitations for the user.
// Must be called after SignUpWithEmail with the returned session.
func (c *AuthClient) UpsertProfile(session *AuthSession) error {
	profileBody := map[string]string{
		"user_id": session.UserID,
		"email":   session.Email,
	}
	profileJSON, err := json.Marshal(profileBody)
	if err != nil {
		return fmt.Errorf("marshaling profile request: %w", err)
	}

	profileURL := c.supabaseURL + "/rest/v1/c4_profiles"
	req, err := http.NewRequest(http.MethodPost, profileURL, bytes.NewReader(profileJSON))
	if err != nil {
		return fmt.Errorf("creating profile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Prefer", "resolution=merge-duplicates")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("profile upsert request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var errResp signupErrorResponse
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errResp) //nolint:errcheck
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		return fmt.Errorf("profile upsert failed (HTTP %d): %w", resp.StatusCode, errors.New(msg))
	}

	// Resolve pending invitations for this user.
	if err := c.resolvePendingInvitations(session); err != nil {
		return fmt.Errorf("resolving pending invitations: %w", err)
	}

	return nil
}

// resolvePendingInvitations calls the c4_resolve_pending_invitations RPC
// to link any team invitations sent to this email before signup.
func (c *AuthClient) resolvePendingInvitations(session *AuthSession) error {
	rpcBody := map[string]string{
		"p_user_id": session.UserID,
		"p_email":   session.Email,
	}
	rpcJSON, err := json.Marshal(rpcBody)
	if err != nil {
		return fmt.Errorf("marshaling rpc request: %w", err)
	}

	rpcURL := c.supabaseURL + "/rest/v1/rpc/c4_resolve_pending_invitations"
	req, err := http.NewRequest(http.MethodPost, rpcURL, bytes.NewReader(rpcJSON))
	if err != nil {
		return fmt.Errorf("creating rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var errResp signupErrorResponse
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errResp) //nolint:errcheck
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		return fmt.Errorf("rpc failed (HTTP %d): %w", resp.StatusCode, errors.New(msg))
	}

	return nil
}
