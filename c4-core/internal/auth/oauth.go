package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// OAuthConfig configures the PKCE OAuth flow.
type OAuthConfig struct {
	SupabaseURL  string // e.g., "https://myproject.supabase.co"
	AnonKey      string // Supabase anon (public) key
	RedirectPort int    // localhost port for callback (default 8765)
	RedirectPath string // callback path (default "/auth/callback")
	Provider     string // OAuth provider (default "github")
	Scopes       []string
}

// DefaultOAuthConfig returns sensible defaults.
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		RedirectPort: 8765,
		RedirectPath: "/auth/callback",
		Provider:     "github",
	}
}

// RedirectURI returns the full redirect URI.
func (c *OAuthConfig) RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s", c.RedirectPort, c.RedirectPath)
}

// OAuthResult holds the outcome of an OAuth flow.
type OAuthResult struct {
	Success      bool
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Error        string
	RawParams    url.Values
}

// PKCEVerifier generates a PKCE code verifier and challenge.
type PKCEVerifier struct {
	Verifier  string
	Challenge string
	Method    string // always "S256"
}

// NewPKCEVerifier generates a fresh PKCE verifier/challenge pair.
func NewPKCEVerifier() (*PKCEVerifier, error) {
	// 32 bytes = 43 base64url characters (RFC 7636 recommends 43-128)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate verifier: %w", err)
	}

	verifier := base64.RawURLEncoding.EncodeToString(b)

	// S256: SHA-256 hash of verifier, then base64url
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEVerifier{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    "S256",
	}, nil
}

// OAuthFlow manages the complete OAuth PKCE flow.
type OAuthFlow struct {
	Config   *OAuthConfig
	State    string // CSRF protection
	PKCE     *PKCEVerifier
}

// NewOAuthFlow creates a new OAuth flow with PKCE and state token.
func NewOAuthFlow(config *OAuthConfig) (*OAuthFlow, error) {
	if config == nil {
		config = DefaultOAuthConfig()
	}
	if config.RedirectPort == 0 {
		config.RedirectPort = 8765
	}
	if config.RedirectPath == "" {
		config.RedirectPath = "/auth/callback"
	}
	if config.Provider == "" {
		config.Provider = "github"
	}

	// Generate state token
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Generate PKCE
	pkce, err := NewPKCEVerifier()
	if err != nil {
		return nil, err
	}

	return &OAuthFlow{
		Config: config,
		State:  state,
		PKCE:   pkce,
	}, nil
}

// AuthorizationURL builds the Supabase authorize URL with PKCE parameters.
func (f *OAuthFlow) AuthorizationURL() string {
	params := url.Values{
		"provider":             {f.Config.Provider},
		"redirect_to":         {f.Config.RedirectURI()},
		"code_challenge":       {f.PKCE.Challenge},
		"code_challenge_method": {f.PKCE.Method},
		"state":                {f.State},
	}

	if len(f.Config.Scopes) > 0 {
		params.Set("scopes", strings.Join(f.Config.Scopes, " "))
	}

	return fmt.Sprintf("%s/auth/v1/authorize?%s", f.Config.SupabaseURL, params.Encode())
}

// ExchangeCodeURL returns the URL for exchanging an auth code for tokens.
func (f *OAuthFlow) ExchangeCodeURL() string {
	return fmt.Sprintf("%s/auth/v1/token?grant_type=pkce", f.Config.SupabaseURL)
}

// Run executes the complete OAuth PKCE flow:
//  1. Start localhost callback server
//  2. Open browser to authorization URL
//  3. Wait for callback with auth code
//  4. Exchange code for tokens
//
// If openBrowser is false, the authorization URL is returned via onURL callback
// for `c4 login --no-browser` mode.
func (f *OAuthFlow) Run(ctx context.Context, openBrowser bool, onURL func(string)) (*OAuthResult, error) {
	// Result channel
	resultCh := make(chan *OAuthResult, 1)
	errCh := make(chan error, 1)

	// Start callback server
	mux := http.NewServeMux()
	var server *http.Server

	mux.HandleFunc(f.Config.RedirectPath, func(w http.ResponseWriter, r *http.Request) {
		result := f.handleCallback(r)
		f.sendHTMLResponse(w, result)
		resultCh <- result
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", f.Config.RedirectPort))
	if err != nil {
		return nil, fmt.Errorf("start callback server: %w", err)
	}

	server = &http.Server{Handler: mux}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Build auth URL
	authURL := f.AuthorizationURL()

	// Open browser or notify
	if openBrowser {
		if err := openInBrowser(authURL); err != nil {
			// Fall back to showing URL
			if onURL != nil {
				onURL(authURL)
			}
		}
	} else {
		if onURL != nil {
			onURL(authURL)
		}
	}

	// Wait for result, error, or context cancellation
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		wg.Wait()
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, fmt.Errorf("callback server: %w", err)
	case <-ctx.Done():
		return &OAuthResult{
			Success: false,
			Error:   "authentication timed out",
		}, nil
	}
}

// handleCallback processes the OAuth callback request.
func (f *OAuthFlow) handleCallback(r *http.Request) *OAuthResult {
	params := r.URL.Query()

	// Check for error
	if errVal := params.Get("error"); errVal != "" {
		desc := params.Get("error_description")
		if desc == "" {
			desc = errVal
		}
		return &OAuthResult{
			Success:   false,
			Error:     desc,
			RawParams: params,
		}
	}

	// Verify state
	if params.Get("state") != f.State {
		return &OAuthResult{
			Success:   false,
			Error:     "state mismatch - possible CSRF attack",
			RawParams: params,
		}
	}

	// Check for auth code (PKCE flow returns a code, not tokens directly)
	code := params.Get("code")
	if code != "" {
		// Exchange code for tokens
		return f.exchangeCode(code)
	}

	// Check for tokens directly (fallback)
	accessToken := params.Get("access_token")
	if accessToken != "" {
		expiresIn := 0
		if v := params.Get("expires_in"); v != "" {
			fmt.Sscanf(v, "%d", &expiresIn)
		}
		return &OAuthResult{
			Success:      true,
			AccessToken:  accessToken,
			RefreshToken: params.Get("refresh_token"),
			ExpiresIn:    expiresIn,
			RawParams:    params,
		}
	}

	return &OAuthResult{
		Success:   false,
		Error:     "no authorization code or access token received",
		RawParams: params,
	}
}

// exchangeCode exchanges an authorization code for tokens using PKCE.
func (f *OAuthFlow) exchangeCode(code string) *OAuthResult {
	exchangeURL := f.ExchangeCodeURL()

	body := map[string]string{
		"auth_code":     code,
		"code_verifier": f.PKCE.Verifier,
	}
	bodyBytes, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", exchangeURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return &OAuthResult{Success: false, Error: fmt.Sprintf("build request: %v", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	if f.Config.AnonKey != "" {
		req.Header.Set("apikey", f.Config.AnonKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &OAuthResult{Success: false, Error: fmt.Sprintf("exchange request: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &OAuthResult{
			Success: false,
			Error:   fmt.Sprintf("token exchange failed: HTTP %d", resp.StatusCode),
		}
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return &OAuthResult{Success: false, Error: fmt.Sprintf("decode response: %v", err)}
	}

	accessToken, _ := data["access_token"].(string)
	refreshToken, _ := data["refresh_token"].(string)
	expiresIn := 0
	if v, ok := data["expires_in"].(float64); ok {
		expiresIn = int(v)
	}

	if accessToken == "" {
		return &OAuthResult{Success: false, Error: "no access_token in exchange response"}
	}

	return &OAuthResult{
		Success:      true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}
}

// sendHTMLResponse writes a user-facing HTML response to the browser.
func (f *OAuthFlow) sendHTMLResponse(w http.ResponseWriter, result *OAuthResult) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	message := "Authentication successful! You can close this window."
	if !result.Success {
		message = fmt.Sprintf("Authentication failed: %s", result.Error)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>C4 Authentication</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f5f5f5}
.c{text-align:center;padding:40px;background:#fff;border-radius:8px;box-shadow:0 2px 10px rgba(0,0,0,.1)}
h1{color:#333;margin-bottom:10px}
p{color:#666}
</style>
</head>
<body><div class="c"><h1>C4</h1><p>%s</p></div></body>
</html>`, message)

	w.Write([]byte(html))
}

// openInBrowser opens a URL in the user's default browser.
func openInBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
