package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	fmt.Fprint(w, htmlPage("인증 완료", "인증 완료! 터미널로 돌아가세요.<script>window.close();</script>"))
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

	ds, err := s.store.GetDeviceSession(state)
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
	if attempts > tokenAttemptLimit {
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

	supabaseURL := s.supabaseURL
	if supabaseURL == "" {
		writeError(w, http.StatusBadRequest, "supabase not configured")
		return
	}

	session, apiErr, statusCode := exchangePKCEToken(supabaseURL, ds.AuthCode, req.CodeVerifier)
	if apiErr != "" {
		writeError(w, statusCode, apiErr)
		return
	}

	writeJSON(w, session)
}

// exchangePKCEToken calls Supabase to exchange an auth_code + code_verifier for session tokens.
// Returns (session JSON object, error message, HTTP status code for error).
// Uses a 30s HTTP client timeout to prevent goroutine leaks if Supabase hangs.
func exchangePKCEToken(supabaseURL, authCode, codeVerifier string) (map[string]any, string, int) {
	body, _ := json.Marshal(map[string]string{
		"auth_code":     authCode,
		"code_verifier": codeVerifier,
	})

	url := strings.TrimRight(supabaseURL, "/") + "/auth/v1/token?grant_type=pkce"

	client := &http.Client{Timeout: pkceHTTPTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body)) //nolint:noctx
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

// htmlPage renders a minimal HTML page with a title and body text.
func htmlPage(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ko">
<head><meta charset="utf-8"><title>%s</title></head>
<body><h1>%s</h1></body>
</html>`, title, body)
}
