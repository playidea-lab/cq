package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleAuthCallback handles GET /auth/callback?state={state}&code={auth_code}.
// It receives the OAuth redirect from Supabase, stores the auth_code, and shows
// a completion page. This endpoint must be publicly accessible (no API key).
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
		fmt.Fprint(w, callbackHTML("오류", "state 또는 code 파라미터가 누락되었습니다."))
		return
	}

	ds, err := s.store.GetDeviceSession(state)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, callbackHTML("오류", "세션을 찾을 수 없거나 만료되었습니다."))
		return
	}

	// If already ready, idempotent 200.
	if ds.Status == "ready" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, callbackHTML("인증 완료", "인증 완료! 터미널로 돌아가세요."))
		return
	}

	if err := s.store.SetDeviceSessionAuthCode(state, code); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, callbackHTML("오류", "인증 코드 저장 중 오류가 발생했습니다."))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, callbackHTML("인증 완료", "인증 완료! 터미널로 돌아가세요."))
}

// handleDeviceToken handles POST /v1/auth/device/{state}/token.
// The CLI sends code_verifier; C5 exchanges it with Supabase and returns the session.
func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	// Extract {state} from /v1/auth/device/{state}/token
	state := extractDeviceState(r.URL.Path)
	if state == "" {
		writeError(w, http.StatusBadRequest, "missing state in path")
		return
	}

	var body struct {
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.CodeVerifier == "" {
		writeError(w, http.StatusBadRequest, "code_verifier is required")
		return
	}

	ds, err := s.store.GetDeviceSession(state)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "session not found or expired")
			return
		}
		writeError(w, http.StatusNotFound, "session not found or expired")
		return
	}

	// GetDeviceSession increments poll_count; if it hit the 20-poll expire limit,
	// ds.Status is now "expired". But we use a stricter 5-attempt limit for /token.
	// We track attempts via poll_count. After the merge: pending→20 generic polls expire;
	// but for /token we apply a separate 5-call limit via attempt tracking.
	// Since GetDeviceSession increments poll_count and sets expired at >20,
	// we enforce 5 attempts by checking poll_count directly on the returned session.
	if ds.PollCount > 5 {
		// Mark as expired due to token attempt exhaustion.
		s.store.SetDeviceSessionAuthCode(state, "") //nolint:errcheck — best effort status change
		writeError(w, http.StatusBadRequest, "not ready")
		return
	}

	if ds.Status != "ready" {
		writeError(w, http.StatusBadRequest, "not ready")
		return
	}

	// Exchange auth_code + code_verifier with Supabase PKCE endpoint.
	session, err := exchangePKCEToken(ds.SupabaseURL, ds.AuthCode, body.CodeVerifier)
	if err != nil {
		if strings.Contains(err.Error(), "unreachable") {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "token exchange failed: "+err.Error())
		return
	}

	writeJSON(w, session)
}

// deviceTokenResponse is the response returned to the CLI after successful exchange.
type deviceTokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresAt    int64       `json:"expires_at"`
	User         sessionUser `json:"user"`
}

type sessionUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// exchangePKCEToken calls Supabase POST /auth/v1/token?grant_type=pkce and returns the session.
func exchangePKCEToken(supabaseURL, authCode, codeVerifier string) (*deviceTokenResponse, error) {
	url := strings.TrimRight(supabaseURL, "/") + "/auth/v1/token?grant_type=pkce"

	payload, _ := json.Marshal(map[string]string{
		"auth_code":     authCode,
		"code_verifier": codeVerifier,
	})

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload)) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("supabase unreachable: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("supabase unreachable: read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to extract Supabase error message.
		var errBody struct {
			Msg string `json:"msg"`
			Err string `json:"error_description"`
		}
		if jerr := json.Unmarshal(body, &errBody); jerr == nil {
			if errBody.Msg != "" {
				return nil, fmt.Errorf("%s", errBody.Msg)
			}
			if errBody.Err != "" {
				return nil, fmt.Errorf("%s", errBody.Err)
			}
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Parse Supabase session response.
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
		User         struct {
			ID       string                 `json:"id"`
			Email    string                 `json:"email"`
			Metadata map[string]interface{} `json:"user_metadata"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %v", err)
	}

	name := ""
	if n, ok := raw.User.Metadata["full_name"].(string); ok {
		name = n
	} else if n, ok := raw.User.Metadata["name"].(string); ok {
		name = n
	}

	return &deviceTokenResponse{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    raw.ExpiresAt,
		User: sessionUser{
			ID:    raw.User.ID,
			Email: raw.User.Email,
			Name:  name,
		},
	}, nil
}

// extractDeviceState extracts {state} from /v1/auth/device/{state}/token.
func extractDeviceState(path string) string {
	// path: /v1/auth/device/{state}/token
	const prefix = "/v1/auth/device/"
	const suffix = "/token"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if !strings.HasSuffix(rest, suffix) {
		return ""
	}
	state := rest[:len(rest)-len(suffix)]
	if strings.Contains(state, "/") {
		return "" // nested path — not valid
	}
	return state
}

// callbackHTML returns a minimal HTML page for the OAuth callback.
func callbackHTML(title, message string) string {
	return `<!DOCTYPE html>
<html lang="ko">
<head><meta charset="utf-8"><title>` + title + `</title>
<style>body{font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{text-align:center;padding:2rem;border-radius:8px;box-shadow:0 2px 8px rgba(0,0,0,.1)}</style>
</head>
<body><div class="box"><h2>` + message + `</h2></div>
<script>setTimeout(function(){window.close()},2000);</script>
</body></html>`
}
