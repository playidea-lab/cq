package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// handleDeviceAuth routes POST/GET /v1/auth/device and /v1/auth/device/{state}.
func (s *Server) handleDeviceAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleDeviceAuthCreate(w, r)
	case http.MethodGet:
		s.handleDeviceAuthPoll(w, r)
	default:
		methodNotAllowed(w)
	}
}

// deviceAuthCreateRequest is the body for POST /v1/auth/device.
type deviceAuthCreateRequest struct {
	CodeChallenge       string `json:"code_challenge"`
	SupabaseURL         string `json:"supabase_url"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

// handleDeviceAuthCreate handles POST /v1/auth/device.
// It creates a device session and returns the user_code, auth_url, and activate_url.
func (s *Server) handleDeviceAuthCreate(w http.ResponseWriter, r *http.Request) {
	var req deviceAuthCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.CodeChallenge == "" {
		writeError(w, http.StatusBadRequest, "missing code_challenge")
		return
	}
	if req.SupabaseURL == "" {
		writeError(w, http.StatusBadRequest, "missing supabase_url")
		return
	}

	// Generate state (32 bytes hex)
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Generate user code (8 chars base32)
	userCodeBytes := make([]byte, 5)
	if _, err := rand.Read(userCodeBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate user code")
		return
	}
	// Use uppercase hex for simplicity; store's CreateDeviceSession will retry on collision
	userCode := strings.ToUpper(hex.EncodeToString(userCodeBytes))[:8]

	expiresAt := time.Now().Add(10 * time.Minute)

	if err := s.store.CreateDeviceSession(state, userCode, req.CodeChallenge, req.SupabaseURL, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	publicURL := s.publicURL
	if publicURL == "" {
		publicURL = s.serverURL
	}

	// Build auth_url: supabase_url/auth/v1/authorize?provider=github&code_challenge=...&code_challenge_method=S256&redirect_to={public_url}/auth/callback?state={state}
	redirectTo := fmt.Sprintf("%s/auth/callback?state=%s", publicURL, url.QueryEscape(state))
	authURL := fmt.Sprintf("%s/auth/v1/authorize?provider=github&code_challenge=%s&code_challenge_method=S256&redirect_to=%s",
		strings.TrimRight(req.SupabaseURL, "/"),
		url.QueryEscape(req.CodeChallenge),
		url.QueryEscape(redirectTo),
	)

	activateURL := fmt.Sprintf("%s/auth/activate", publicURL)

	writeJSON(w, map[string]any{
		"state":        state,
		"user_code":    userCode,
		"auth_url":     authURL,
		"activate_url": activateURL,
		"expires_in":   600,
	})
}

// handleDeviceAuthPoll handles GET /v1/auth/device/{state}.
func (s *Server) handleDeviceAuthPoll(w http.ResponseWriter, r *http.Request) {
	// Extract state from path: /v1/auth/device/{state}
	state := strings.TrimPrefix(r.URL.Path, "/v1/auth/device/")
	if state == "" || state == r.URL.Path {
		writeError(w, http.StatusBadRequest, "missing state")
		return
	}

	ds, err := s.store.GetDeviceSession(state)
	if err != nil || ds == nil {
		writeError(w, http.StatusNotFound, "not found or expired")
		return
	}

	writeJSON(w, map[string]string{
		"status": ds.Status,
	})
}

// handleActivateGet handles GET /auth/activate — serves the HTML form.
func (s *Server) handleActivateGet(w http.ResponseWriter, r *http.Request) {
	// Generate CSRF token
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate csrf")
		return
	}
	csrfToken := hex.EncodeToString(csrfBytes)

	// Set CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf",
		Value:    csrfToken,
		Path:     "/auth/activate",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   600, // 10 minutes
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, activatePageHTML, csrfToken)
}

// handleActivatePost handles POST /auth/activate — validates CSRF + user_code, redirects to auth_url.
func (s *Server) handleActivatePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form data")
		return
	}

	userCode := strings.TrimSpace(r.FormValue("user_code"))
	csrfBody := r.FormValue("csrf")

	// Validate CSRF: cookie value must match body value
	csrfCookie, err := r.Cookie("csrf")
	if err != nil || csrfCookie.Value == "" {
		writeError(w, http.StatusForbidden, "missing csrf cookie")
		return
	}
	if csrfCookie.Value != csrfBody {
		writeError(w, http.StatusForbidden, "csrf mismatch")
		return
	}

	if userCode == "" {
		writeError(w, http.StatusBadRequest, "missing user_code")
		return
	}

	ds, err := s.store.GetDeviceSessionByUserCode(strings.ToUpper(userCode))
	if err != nil || ds == nil {
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	// Check if expired
	if ds.Status == "expired" || ds.ExpiresAt.Before(time.Now()) {
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	// Build auth_url
	publicURL := s.publicURL
	if publicURL == "" {
		publicURL = s.serverURL
	}
	redirectTo := fmt.Sprintf("%s/auth/callback?state=%s", publicURL, url.QueryEscape(ds.State))
	authURL := fmt.Sprintf("%s/auth/v1/authorize?provider=github&code_challenge=%s&code_challenge_method=S256&redirect_to=%s",
		strings.TrimRight(ds.SupabaseURL, "/"),
		url.QueryEscape(ds.CodeChallenge),
		url.QueryEscape(redirectTo),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleActivate routes GET/POST /auth/activate.
func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleActivateGet(w, r)
	case http.MethodPost:
		s.handleActivatePost(w, r)
	default:
		methodNotAllowed(w)
	}
}

const activatePageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>CQ Device Authorization</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         display: flex; justify-content: center; align-items: center; min-height: 100vh;
         margin: 0; background: #f5f5f5; }
  .card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);
          max-width: 400px; width: 100%%; text-align: center; }
  h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
  p { color: #666; margin-bottom: 1.5rem; }
  input[type=text] { font-size: 1.5rem; text-align: center; padding: 0.75rem; width: 80%%;
                     border: 2px solid #ddd; border-radius: 6px; letter-spacing: 0.2em;
                     text-transform: uppercase; }
  input[type=text]:focus { outline: none; border-color: #0066ff; }
  button { margin-top: 1rem; padding: 0.75rem 2rem; font-size: 1rem; background: #0066ff;
           color: white; border: none; border-radius: 6px; cursor: pointer; }
  button:hover { background: #0052cc; }
</style>
</head>
<body>
<div class="card">
  <h1>Device Authorization</h1>
  <p>Enter the code shown in your terminal</p>
  <form method="POST" action="/auth/activate">
    <input type="text" name="user_code" placeholder="ABCD1234" maxlength="8" autocomplete="off" autofocus>
    <input type="hidden" name="csrf" value="%s">
    <br>
    <button type="submit">Authorize</button>
  </form>
</div>
</body>
</html>`
