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

// AuthClient is a Supabase Auth REST API client that handles GitHub OAuth flow.
type AuthClient struct {
	supabaseURL string
	anonKey     string
	sessionPath string
	httpClient  *http.Client
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
//  4. Exchange authorization code for access token
//  5. Save session to ~/.c4/session.json
func (c *AuthClient) LoginWithGitHub() error {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	srv, port, err := c.startCallbackServer(codeCh, errCh)
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}

	// Build and open the OAuth URL
	authURL := c.oauthURL(port)
	if err := openBrowser(authURL); err != nil {
		srv.Close()
		return fmt.Errorf("opening browser: %w", err)
	}

	fmt.Printf("Waiting for GitHub authorization...\n")
	fmt.Printf("If your browser didn't open, visit:\n  %s\n\n", authURL)

	// Wait for the callback
	var code string
	select {
	case code = <-codeCh:
		// Got the code
	case err := <-errCh:
		srv.Close()
		return fmt.Errorf("OAuth callback error: %w", err)
	case <-time.After(callbackTimeout):
		srv.Close()
		return fmt.Errorf("OAuth login timed out after %s", callbackTimeout)
	}

	// Shut down the callback server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)

	// Exchange code for token
	session, err := c.exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("exchanging code for token: %w", err)
	}

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

// startCallbackServer starts a temporary HTTP server to receive the OAuth callback.
// Returns the server, the actual port it is listening on, and any error.
func (c *AuthClient) startCallbackServer(codeCh chan<- string, errCh chan<- error) (*http.Server, int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing authorization code. Please try again.")
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h2>Authorization successful!</h2>
<p>You can close this window and return to your terminal.</p>
<script>window.close()</script>
</body></html>`)

		codeCh <- code
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

// oauthURL constructs the Supabase OAuth authorization URL for GitHub.
func (c *AuthClient) oauthURL(callbackPort int) string {
	redirectTo := "http://localhost:" + strconv.Itoa(callbackPort) + "/auth/callback"
	return c.supabaseURL + "/auth/v1/authorize" +
		"?provider=github" +
		"&redirect_to=" + redirectTo
}

// exchangeCodeForToken exchanges an authorization code for an access token
// via the Supabase /auth/v1/token endpoint.
func (c *AuthClient) exchangeCodeForToken(code string) (*Session, error) {
	body := map[string]string{
		"auth_code":     code,
		"code_verifier": "",
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling token request: %w", err)
	}

	url := c.supabaseURL + "/auth/v1/token?grant_type=authorization_code"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp supabaseErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, errResp.ErrorDescription)
	}

	var tokenResp supabaseTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	return tokenResponseToSession(&tokenResp), nil
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
