package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// SupabaseSession is the OAuth session returned after a successful device or link flow.
// It is identical in structure to Session and can be used interchangeably.
type SupabaseSession = Session

// DeviceCloudConfig holds the minimal configuration needed for device/link flows.
type DeviceCloudConfig struct {
	// HubURL is the base URL of the C5 Hub server (e.g. "https://hub.example.com").
	HubURL string
	// SupabaseURL is the Supabase project URL (e.g. "https://xxx.supabase.co").
	SupabaseURL string
}

// pkcePending is persisted to disk so a polling process can resume after restart.
type pkcePending struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	CreatedAt    time.Time `json:"created_at"`
}

// deviceInitResponse is returned by POST /v1/auth/device.
type deviceInitResponse struct {
	State         string `json:"state"`
	UserCode      string `json:"user_code"`
	DeviceAuthURL string `json:"device_auth_url"`
	AuthURL       string `json:"auth_url"` // used by link flow
}

// devicePollResponse is returned by GET /v1/auth/device/{state}.
type devicePollResponse struct {
	Status string `json:"status"` // "pending", "ready", "expired"
}

// deviceTokenResponse is returned by POST /v1/auth/device/{state}/token.
type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	User         User   `json:"user"`
}

// deviceFlowTimeout is the maximum time to wait for the user to complete auth.
const deviceFlowTimeout = 5 * time.Minute

// devicePollInterval is how often to poll the hub for auth status.
// It is a variable (not const) so tests can override it.
var devicePollInterval = 5 * time.Second

// pkceDir returns the directory used for PKCE pending files (~/.c4).
func pkceDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".c4")
}

// pkcePendingPath returns the path for a PKCE pending file for the given state.
func pkcePendingPath(state string) string {
	return filepath.Join(pkceDir(), ".pkce_pending_"+state+".json")
}

// savePKCEPending writes the PKCE pending data to disk (0600).
func savePKCEPending(state, verifier string) error {
	if err := os.MkdirAll(pkceDir(), 0o700); err != nil {
		return fmt.Errorf("creating pkce dir: %w", err)
	}
	data := pkcePending{
		State:        state,
		CodeVerifier: verifier,
		CreatedAt:    time.Now(),
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling pkce pending: %w", err)
	}
	path := pkcePendingPath(state)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("writing pkce pending file: %w", err)
	}
	return nil
}

// removePKCEPending removes the PKCE pending file (best-effort).
func removePKCEPending(state string) {
	_ = os.Remove(pkcePendingPath(state))
}

// httpClient is a package-level variable so tests can replace it.
var deviceHTTPClient = &http.Client{Timeout: 30 * time.Second}

// postJSON sends a POST request with JSON body and decodes the JSON response.
func postJSON(ctx context.Context, url string, body, out interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := deviceHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("POST %s returned %d: %s", url, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// getJSON sends a GET request and decodes the JSON response.
func getJSON(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := deviceHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %d: %s", url, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// pollAndExchange polls the hub until auth is ready, then exchanges the code for a session.
func pollAndExchange(ctx context.Context, hubURL, state, verifier string) (*SupabaseSession, error) {
	pollURL := fmt.Sprintf("%s/v1/auth/device/%s", hubURL, state)
	tokenURL := fmt.Sprintf("%s/v1/auth/device/%s/token", hubURL, state)

	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("authentication timed out: %w", ctx.Err())
		case <-ticker.C:
			var poll devicePollResponse
			if err := getJSON(ctx, pollURL, &poll); err != nil {
				// Transient error — keep polling
				continue
			}
			switch poll.Status {
			case "ready":
				var tok deviceTokenResponse
				if err := postJSON(ctx, tokenURL, map[string]string{
					"code_verifier": verifier,
				}, &tok); err != nil {
					return nil, fmt.Errorf("exchanging token: %w", err)
				}
				return &SupabaseSession{
					AccessToken:  tok.AccessToken,
					RefreshToken: tok.RefreshToken,
					ExpiresAt:    tok.ExpiresAt,
					User:         tok.User,
				}, nil
			case "expired":
				return nil, fmt.Errorf("device authorization expired")
			default:
				// "pending" — keep waiting
			}
		}
	}
}

// LoginWithDevice implements the Device Flow (RFC 8628 inspired) for headless auth.
//
// Flow:
//  1. Generate PKCE verifier + challenge
//  2. POST to hub to start device auth → receive user_code + device_auth_url
//  3. Print instructions for the user to visit the URL and enter the code
//  4. Poll hub until auth is complete, then exchange for a session
func LoginWithDevice(ctx context.Context, cfg DeviceCloudConfig) (*SupabaseSession, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}

	var init deviceInitResponse
	if err := postJSON(ctx, cfg.HubURL+"/v1/auth/device", map[string]string{
		"code_challenge": challenge,
		"supabase_url":   cfg.SupabaseURL,
	}, &init); err != nil {
		return nil, fmt.Errorf("starting device auth: %w", err)
	}

	if err := savePKCEPending(init.State, verifier); err != nil {
		return nil, fmt.Errorf("saving pkce pending: %w", err)
	}
	defer removePKCEPending(init.State)

	fmt.Printf("코드: %s\n브라우저에서 %s 방문 후 코드를 입력하세요\n", init.UserCode, init.DeviceAuthURL)

	pollCtx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()

	return pollAndExchange(pollCtx, cfg.HubURL, init.State, verifier)
}

// LoginWithLink implements the Direct Link Flow for headless auth.
//
// Flow:
//  1. Generate PKCE verifier + challenge
//  2. POST to hub with flow="link" → receive auth_url
//  3. Print the auth_url for the user to open in their browser
//  4. Poll hub until auth is complete, then exchange for a session
func LoginWithLink(ctx context.Context, cfg DeviceCloudConfig) (*SupabaseSession, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}

	var init deviceInitResponse
	if err := postJSON(ctx, cfg.HubURL+"/v1/auth/device", map[string]string{
		"code_challenge": challenge,
		"supabase_url":   cfg.SupabaseURL,
		"flow":           "link",
	}, &init); err != nil {
		return nil, fmt.Errorf("starting link auth: %w", err)
	}

	if err := savePKCEPending(init.State, verifier); err != nil {
		return nil, fmt.Errorf("saving pkce pending: %w", err)
	}
	defer removePKCEPending(init.State)

	fmt.Printf("아래 링크를 브라우저에서 열어주세요:\n%s\n", init.AuthURL)

	pollCtx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()

	return pollAndExchange(pollCtx, cfg.HubURL, init.State, verifier)
}
