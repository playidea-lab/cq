package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	deviceSessionTTL    = 10 * time.Minute
	tokenAttemptLimit   = 5
	pkceHTTPTimeout     = 30 * time.Second // prevents goroutine leak if Supabase hangs
)

// handleAuthCallback handles GET /auth/callback?state=...&code=...
// It stores the auth_code in the device session and returns a success HTML page.
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	// MEDIUM: cap query param lengths to prevent oversized DB writes.
	if len(state) > 128 || len(code) > 2048 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, htmlPage("오류", "잘못된 요청입니다."))
		return
	}

	if state == "" || code == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, htmlPage("오류", "state 또는 code 파라미터가 누락되었습니다."))
		return
	}

	if err := s.store.SetDeviceSessionAuthCode(state, code); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, htmlPage("오류", "세션을 찾을 수 없거나 만료되었습니다."))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, authSuccessPageHTML)
}

// handleAuthDeviceToken handles POST /v1/auth/device/{state}/token
// It exchanges the stored auth_code with Supabase for an access token.
func (s *Server) handleAuthDeviceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	// Extract state from path: /v1/auth/device/{state}/token
	path := strings.TrimPrefix(r.URL.Path, "/v1/auth/device/")
	path = strings.TrimSuffix(path, "/token")
	state := strings.Trim(path, "/")
	if state == "" {
		writeError(w, http.StatusBadRequest, "missing state in path")
		return
	}

	// Use PeekDeviceSession to avoid incrementing poll_count on token exchange path (MEDIUM #4).
	ds, err := s.store.PeekDeviceSession(state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "session not found or expired")
			return
		}
		writeError(w, http.StatusNotFound, "session not found or expired")
		return
	}

	// Check expiry
	if time.Now().UTC().After(ds.ExpiresAt) || ds.Status == "expired" {
		writeError(w, http.StatusNotFound, "session not found or expired")
		return
	}

	// Increment token_attempts BEFORE checking ready status to prevent timing attacks.
	// This uses a dedicated counter separate from any job poll_count.
	attempts, err := s.store.IncrementTokenAttempts(state, tokenAttemptLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// HIGH #2: >= prevents an extra attempt when session is marked expired at the limit boundary.
	if attempts >= tokenAttemptLimit {
		writeError(w, http.StatusNotFound, "session not found or expired")
		return
	}

	// MEDIUM #3: Re-read session to get fresh status after IncrementTokenAttempts
	// (avoids TOCTOU: ds.Status may be stale if IncrementTokenAttempts just expired the session).
	ds, err = s.store.PeekDeviceSession(state)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found or expired")
		return
	}

	if ds.Status != "ready" {
		writeError(w, http.StatusBadRequest, "not ready")
		return
	}

	var req struct {
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CodeVerifier == "" {
		writeError(w, http.StatusBadRequest, "code_verifier is required")
		return
	}

	// Use server-configured supabaseURL; fall back to the value stored in the device session
	// (set at creation time from the client-supplied supabase_url when server has no config).
	supabaseURL := s.supabaseURL
	if supabaseURL == "" {
		supabaseURL = ds.SupabaseURL
	}
	if supabaseURL == "" {
		writeError(w, http.StatusBadRequest, "supabase not configured")
		return
	}

	session, apiErr, statusCode := exchangePKCEToken(supabaseURL, s.supabaseKey, ds.AuthCode, req.CodeVerifier)
	if apiErr != "" {
		writeError(w, statusCode, apiErr)
		return
	}

	writeJSON(w, session)
}

// exchangePKCEToken calls Supabase to exchange an auth_code + code_verifier for session tokens.
// supabaseKey is the project's anon key (sent as apikey header). May be empty for self-hosted Supabase.
// Returns (session JSON object, error message, HTTP status code for error).
// Uses a 30s HTTP client timeout to prevent goroutine leaks if Supabase hangs.
func exchangePKCEToken(supabaseURL, supabaseKey, authCode, codeVerifier string) (map[string]any, string, int) {
	body, _ := json.Marshal(map[string]string{
		"auth_code":     authCode,
		"code_verifier": codeVerifier,
	})

	url := strings.TrimRight(supabaseURL, "/") + "/auth/v1/token?grant_type=pkce"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return nil, fmt.Sprintf("creating request: %v", err), http.StatusInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	if supabaseKey != "" {
		req.Header.Set("apikey", supabaseKey)
		req.Header.Set("Authorization", "Bearer "+supabaseKey)
	}

	client := &http.Client{Timeout: pkceHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Sprintf("supabase unreachable: %v", err), http.StatusBadGateway
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode != http.StatusOK {
		// Try to extract Supabase error message
		var errBody struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(rawBody, &errBody)
		detail := errBody.ErrorDescription
		if detail == "" {
			detail = errBody.Error
		}
		if detail == "" {
			detail = string(rawBody)
		}
		return nil, fmt.Sprintf("token exchange failed: %s", detail), http.StatusBadRequest
	}

	var session map[string]any
	if err := json.Unmarshal(rawBody, &session); err != nil {
		return nil, "invalid response from supabase", http.StatusBadGateway
	}

	// Remove auth_code from session response (security: must not be exposed to client)
	delete(session, "auth_code")

	return session, "", 0
}

// htmlPage renders a minimal HTML page. title is HTML-escaped; body is trusted HTML (caller's responsibility).
func htmlPage(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ko">
<head><meta charset="utf-8"><title>%s</title></head>
<body><h1>%s</h1></body>
</html>`, html.EscapeString(title), body)
}

const authSuccessPageHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>인증 완료 — CQ</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         display: flex; justify-content: center; align-items: center; min-height: 100vh;
         margin: 0; background: #f0faf4; }
  .card { background: white; padding: 2.5rem 2rem; border-radius: 12px;
          box-shadow: 0 2px 12px rgba(0,0,0,0.1); max-width: 380px; width: 100%;
          text-align: center; }
  .icon { width: 72px; height: 72px; margin: 0 auto 1.5rem; }
  h1 { font-size: 1.5rem; color: #1a1a1a; margin: 0 0 0.5rem; }
  p { color: #555; font-size: 0.95rem; margin: 0; line-height: 1.5; }
  .hint { margin-top: 1.5rem; font-size: 0.8rem; color: #999; }
</style>
</head>
<body>
<div class="card">
  <svg class="icon" viewBox="0 0 72 72" fill="none" xmlns="http://www.w3.org/2000/svg">
    <circle cx="36" cy="36" r="36" fill="#e8f5e9"/>
    <path d="M20 36l12 12 20-24" stroke="#2e7d32" stroke-width="4"
          stroke-linecap="round" stroke-linejoin="round"/>
  </svg>
  <h1>인증 완료!</h1>
  <p>터미널로 돌아가세요.<br>이 창은 닫아도 됩니다.</p>
  <p class="hint">3초 후 자동으로 닫힙니다.</p>
</div>
<script>setTimeout(function(){ window.close(); }, 3000);</script>
</body>
</html>`
