// Package cloud implements Supabase Auth REST API client with GitHub OAuth flow.
//
// It provides session management with local file-based token storage,
// token refresh, and a temporary localhost callback server for the OAuth
// authorization code exchange.
//
// Usage:
//
//	client := cloud.NewAuthClient(supabaseURL, anonKey)
//	if err := client.LoginWithGitHub(); err != nil { ... }
//	session, _ := client.GetSession()
//	fmt.Println(session.User.Email)
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// defaultCallbackPort is the preferred port for the OAuth callback server.
const defaultCallbackPort = 19823

// defaultSessionDir is the directory under $HOME for storing session data.
// Shared with the C1 Tauri app.
const defaultSessionDir = ".c4"

// defaultSessionFile is the session file name.
const defaultSessionFile = "session.json"

// callbackTimeout is how long we wait for the OAuth callback.
const callbackTimeout = 120 * time.Second

// User represents the authenticated user's profile.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Session holds the current authentication state.
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	User         User   `json:"user"`
}

// UnmarshalJSON implements json.Unmarshaler for Session.
// It handles expires_at as either a numeric Unix timestamp (Go format)
// or an ISO 8601 string (Rust c1 format).
func (s *Session) UnmarshalJSON(data []byte) error {
	type Alias Session
	var raw struct {
		Alias
		ExpiresAt interface{} `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = Session(raw.Alias)
	switch v := raw.ExpiresAt.(type) {
	case float64:
		s.ExpiresAt = int64(v)
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return fmt.Errorf("parsing expires_at %q: %w", v, err)
		}
		s.ExpiresAt = t.Unix()
	case nil:
		s.ExpiresAt = 0
	}
	return nil
}

// openBrowserFunc is a package-level variable so tests can stub browser opening.
var openBrowserFunc = openBrowser

// AuthClient is a Supabase Auth REST API client that handles GitHub OAuth flow.
type AuthClient struct {
	supabaseURL string
	anonKey     string
	sessionPath string
	httpClient  *http.Client
	// NoBrowser disables automatic browser opening. When true, the OAuth URL
	// and SSH port-forwarding hint are printed to stderr instead.
	NoBrowser bool
	// callbackTimeout overrides the default OAuth callback wait duration.
	// Zero means use the package-level callbackTimeout constant.
	// Set in tests to keep them fast.
	callbackTimeout time.Duration
}

// NewAuthClient creates a new AuthClient for the given Supabase project.
func NewAuthClient(supabaseURL, anonKey string) *AuthClient {
	sessionPath := defaultSessionPath()
	return &AuthClient{
		supabaseURL: supabaseURL,
		anonKey:     anonKey,
		sessionPath: sessionPath,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// SetSessionPath overrides the default session file path.
// Useful for testing.
func (c *AuthClient) SetSessionPath(path string) {
	c.sessionPath = path
}

// LoginWithGitHub starts the GitHub OAuth flow by opening the system browser
// and waiting for the authorization code via a localhost callback server.
//
// The flow:
//  1. Start a temporary HTTP server on localhost:19823
//  2. Open browser to Supabase /auth/v1/authorize?provider=github
//  3. User authorizes on GitHub, Supabase redirects to localhost callback
//  4. Callback page extracts tokens from URL fragment via JavaScript
//  5. JavaScript POSTs tokens to /auth/token endpoint
//  6. Save session to ~/.c4/session.json
func (c *AuthClient) LoginWithGitHub() error {
	sessionCh := make(chan *Session, 1)
	errCh := make(chan error, 1)

	srv, port, err := c.startCallbackServer2(sessionCh, errCh)
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}

	// Build the OAuth URL and either open browser or print instructions.
	authURL := c.oauthURL(port)
	if c.NoBrowser {
		fmt.Fprintf(os.Stderr, "Open this URL in your browser:\n  %s\n", authURL)
		fmt.Fprintf(os.Stderr, "Waiting for authorization (http://localhost:%d)...\n", port)
		fmt.Fprintf(os.Stderr, "(Use SSH port forwarding: ssh -L %d:localhost:%d user@host)\n", port, port)
	} else {
		if err := openBrowserFunc(authURL); err != nil {
			srv.Close()
			return fmt.Errorf("opening browser: %w", err)
		}
		fmt.Printf("Waiting for GitHub authorization...\n")
		fmt.Printf("If your browser didn't open, visit:\n  %s\n\n", authURL)
	}

	// Wait for the callback
	var session *Session
	select {
	case session = <-sessionCh:
		// Got the session
	case err := <-errCh:
		srv.Close()
		return fmt.Errorf("OAuth callback error: %w", err)
	case <-time.After(c.effectiveCallbackTimeout()):
		srv.Close()
		return fmt.Errorf("OAuth login timed out after %s", c.effectiveCallbackTimeout())
	}

	// Shut down the callback server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)

	// Persist the session
	if err := c.saveSession(session); err != nil {
		return fmt.Errorf("saving session: %w", err)
	}

	fmt.Printf("Logged in as %s (%s)\n", session.User.Name, session.User.Email)
	return nil
}

// Logout clears the stored session.
func (c *AuthClient) Logout() error {
	if err := os.Remove(c.sessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing session file: %w", err)
	}
	return nil
}

// GetSession returns the current session, or nil if not authenticated.
func (c *AuthClient) GetSession() (*Session, error) {
	return c.loadSession()
}

// IsAuthenticated returns true if a valid, non-expired session exists.
func (c *AuthClient) IsAuthenticated() bool {
	session, err := c.loadSession()
	if err != nil || session == nil {
		return false
	}
	return time.Now().Unix() < session.ExpiresAt
}

// RefreshToken refreshes the access token using the stored refresh token.
// Returns the updated session.
func (c *AuthClient) RefreshToken() (*Session, error) {
	session, err := c.loadSession()
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("no session to refresh")
	}

	// If session on disk already has a valid, unexpired access token (written by
	// another process such as Rust c1 app), reuse it to avoid refresh-token rotation race.
	if session.AccessToken != "" && session.ExpiresAt > 0 && time.Now().Unix() < session.ExpiresAt {
		return session, nil
	}

	if session.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	body := map[string]string{
		"refresh_token": session.RefreshToken,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling refresh request: %w", err)
	}

	url := c.supabaseURL + "/auth/v1/token?grant_type=refresh_token"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp supabaseErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("refresh failed (HTTP %d): %s", resp.StatusCode, errResp.ErrorDescription)
	}

	var tokenResp supabaseTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}

	newSession := tokenResponseToSession(&tokenResp)
	if err := c.saveSession(newSession); err != nil {
		return nil, fmt.Errorf("saving refreshed session: %w", err)
	}

	return newSession, nil
}

// startCallbackServer2 starts a temporary HTTP server that handles Supabase's
// implicit OAuth flow. Supabase returns tokens as URL fragments (#access_token=...),
// so the callback page uses JavaScript to extract them and POST to /auth/token.
func (c *AuthClient) startCallbackServer2(sessionCh chan<- *Session, errCh chan<- error) (*http.Server, int, error) {
	mux := http.NewServeMux()

	// Callback page: extracts fragment tokens via JS and POSTs to /auth/token
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h2>Completing authorization...</h2>
<p id="status">Extracting tokens...</p>
<script>
(function() {
  var hash = window.location.hash.substring(1);
  if (!hash) {
    document.getElementById('status').textContent = 'No tokens in URL. Please try again.';
    return;
  }
  var params = new URLSearchParams(hash);
  var data = {
    access_token: params.get('access_token'),
    refresh_token: params.get('refresh_token'),
    expires_in: parseInt(params.get('expires_in')) || 3600,
    token_type: params.get('token_type') || 'bearer'
  };
  if (!data.access_token) {
    document.getElementById('status').textContent = 'Missing access token. Please try again.';
    return;
  }
  fetch('/auth/token', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(data)
  }).then(function(resp) {
    if (resp.ok) {
      document.getElementById('status').textContent = 'Authorization successful! You can close this window.';
      window.close();
    } else {
      document.getElementById('status').textContent = 'Failed to save token. Check terminal.';
    }
  }).catch(function(err) {
    document.getElementById('status').textContent = 'Error: ' + err.message;
  });
})();
</script>
</body></html>`)
	})

	// Token receiver: accepts POSTed tokens from the callback page JS
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var tokenData struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
			TokenType    string `json:"token_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&tokenData); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		if tokenData.AccessToken == "" {
			http.Error(w, "missing access_token", http.StatusBadRequest)
			return
		}

		// Fetch user info from Supabase
		user, err := c.getUserInfo(tokenData.AccessToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: could not fetch user info: %v\n", err)
			user = &User{}
		}

		session := &Session{
			AccessToken:  tokenData.AccessToken,
			RefreshToken: tokenData.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second).Unix(),
			User:         *user,
		}

		w.WriteHeader(http.StatusOK)
		sessionCh <- session
	})

	// Try the default port first, then fall back to a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(defaultCallbackPort))
	if err != nil {
		// Port in use, pick a random available port
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, 0, fmt.Errorf("listening on localhost: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: mux}

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	return srv, port, nil
}

// effectiveCallbackTimeout returns the configured timeout or the package default.
func (c *AuthClient) effectiveCallbackTimeout() time.Duration {
	if c.callbackTimeout > 0 {
		return c.callbackTimeout
	}
	return callbackTimeout
}

// oauthURL constructs the Supabase OAuth authorization URL for GitHub.
func (c *AuthClient) oauthURL(callbackPort int) string {
	redirectTo := "http://localhost:" + strconv.Itoa(callbackPort) + "/auth/callback"
	return c.supabaseURL + "/auth/v1/authorize" +
		"?provider=github" +
		"&redirect_to=" + redirectTo
}

// saveSession writes the session to disk with restricted permissions.
func (c *AuthClient) saveSession(session *Session) error {
	dir := filepath.Dir(c.sessionPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	if err := os.WriteFile(c.sessionPath, data, 0o600); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// loadSession reads the session from disk. Returns nil, nil if the file
// does not exist.
func (c *AuthClient) loadSession() (*Session, error) {
	data, err := os.ReadFile(c.sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// getUserInfo fetches the authenticated user's profile from Supabase.
func (c *AuthClient) getUserInfo(accessToken string) (*User, error) {
	url := c.supabaseURL + "/auth/v1/user"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating user info request: %w", err)
	}
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user info failed (HTTP %d)", resp.StatusCode)
	}

	var supaUser supabaseUser
	if err := json.NewDecoder(resp.Body).Decode(&supaUser); err != nil {
		return nil, fmt.Errorf("decoding user info: %w", err)
	}

	name := ""
	if supaUser.UserMetadata != nil {
		if n, ok := supaUser.UserMetadata["full_name"].(string); ok {
			name = n
		}
	}

	return &User{
		ID:    supaUser.ID,
		Email: supaUser.Email,
		Name:  name,
	}, nil
}

// --- Supabase API types ---

// supabaseTokenResponse is the response from /auth/v1/token.
type supabaseTokenResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         supabaseUser `json:"user"`
}

// supabaseUser is the user object within a Supabase token response.
type supabaseUser struct {
	ID           string         `json:"id"`
	Email        string         `json:"email"`
	UserMetadata map[string]any `json:"user_metadata"`
}

// supabaseErrorResponse is the error body from Supabase auth endpoints.
type supabaseErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// tokenResponseToSession converts a Supabase token response to a Session.
func tokenResponseToSession(resp *supabaseTokenResponse) *Session {
	name := ""
	if resp.User.UserMetadata != nil {
		if n, ok := resp.User.UserMetadata["full_name"].(string); ok {
			name = n
		}
	}

	return &Session{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Unix(),
		User: User{
			ID:    resp.User.ID,
			Email: resp.User.Email,
			Name:  name,
		},
	}
}

// defaultSessionPath returns the default path for storing the session file.
func defaultSessionPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, defaultSessionDir, defaultSessionFile)
}

// openBrowser opens the given URL in the system's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
