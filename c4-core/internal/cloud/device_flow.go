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

// DeviceFlowConfig holds connection settings for device/link flow.
type DeviceFlowConfig struct {
	HubURL      string
	SupabaseURL string
}

// deviceInitResponse is returned by POST /v1/auth/device.
type deviceInitResponse struct {
	State         string `json:"state"`
	UserCode      string `json:"user_code"`
	DeviceAuthURL string `json:"device_auth_url"`
	AuthURL       string `json:"auth_url"` // for link flow
}

// deviceStatusResponse is returned by GET /v1/auth/device/{state}.
type deviceStatusResponse struct {
	Status string `json:"status"` // "pending", "ready", "expired"
}

// tokenRequest is sent to POST /v1/auth/device/{state}/token.
type tokenRequest struct {
	CodeVerifier string `json:"code_verifier"`
}

const deviceFlowTimeout = 5 * time.Minute

// devicePollInterval is how often to poll the hub for auth status.
// It is a variable (not const) so tests can override it.
var devicePollInterval = 5 * time.Second

// LoginWithDevice initiates a Device Flow: prints a user_code and device_auth_url,
// then polls until the user completes authorization. Returns the session on success.
func LoginWithDevice(ctx context.Context, cfg DeviceFlowConfig) (*Session, error) {
	if cfg.HubURL == "" {
		return nil, fmt.Errorf("C5 Hub URL이 설정되지 않았습니다. cq config set hub.url <URL>")
	}

	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE: %w", err)
	}

	// Step 1: initiate device flow.
	initResp, err := postDeviceInit(ctx, cfg, challenge, "")
	if err != nil {
		return nil, err
	}

	// Save PKCE pending file.
	if err := savePKCEPending(initResp.State, verifier); err != nil {
		// Non-fatal: just log.
		fmt.Fprintf(os.Stderr, "warning: could not save PKCE pending file: %v\n", err)
	}
	defer removePKCEPending(initResp.State)

	// Step 2: print instructions.
	fmt.Printf("코드: %s\n브라우저에서 %s 방문 후 코드를 입력하세요\n", initResp.UserCode, initResp.DeviceAuthURL)

	// Step 3: poll.
	return pollForSession(ctx, cfg, initResp.State, verifier)
}

// LoginWithLink initiates a Direct Link Flow: prints a long auth_url,
// then polls until the user completes authorization. Returns the session on success.
func LoginWithLink(ctx context.Context, cfg DeviceFlowConfig) (*Session, error) {
	if cfg.HubURL == "" {
		return nil, fmt.Errorf("C5 Hub URL이 설정되지 않았습니다. cq config set hub.url <URL>")
	}

	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE: %w", err)
	}

	initResp, err := postDeviceInit(ctx, cfg, challenge, "link")
	if err != nil {
		return nil, err
	}

	if err := savePKCEPending(initResp.State, verifier); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save PKCE pending file: %v\n", err)
	}
	defer removePKCEPending(initResp.State)

	fmt.Printf("아래 링크를 브라우저에서 열어주세요:\n%s\n", initResp.AuthURL)

	return pollForSession(ctx, cfg, initResp.State, verifier)
}

// postDeviceInit sends POST /v1/auth/device to start the flow.
func postDeviceInit(ctx context.Context, cfg DeviceFlowConfig, challenge, flow string) (*deviceInitResponse, error) {
	body := map[string]string{
		"code_challenge": challenge,
		"supabase_url":   cfg.SupabaseURL,
	}
	if flow != "" {
		body["flow"] = flow
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.HubURL+"/v1/auth/device", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("initiating device flow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device init failed (status %d): %s", resp.StatusCode, string(data))
	}

	var initResp deviceInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return nil, fmt.Errorf("decoding device init response: %w", err)
	}
	return &initResp, nil
}

// pollForSession polls GET /v1/auth/device/{state} until ready, then exchanges the token.
func pollForSession(ctx context.Context, cfg DeviceFlowConfig, state, verifier string) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()

	client := &http.Client{Timeout: 15 * time.Second}
	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, context.DeadlineExceeded
		case <-ticker.C:
			status, err := getDeviceStatus(ctx, client, cfg.HubURL, state)
			if err != nil {
				return nil, err
			}
			switch status {
			case "pending":
				// continue polling
			case "ready":
				return exchangeDeviceToken(ctx, client, cfg.HubURL, state, verifier)
			case "expired":
				return nil, fmt.Errorf("인증이 만료되었습니다. 다시 시도해주세요")
			default:
				return nil, fmt.Errorf("unexpected device status: %s", status)
			}
		}
	}
}

func getDeviceStatus(ctx context.Context, client *http.Client, hubURL, state string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hubURL+"/v1/auth/device/"+state, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var statusResp deviceStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return "", fmt.Errorf("decoding status: %w", err)
	}
	return statusResp.Status, nil
}

func exchangeDeviceToken(ctx context.Context, client *http.Client, hubURL, state, verifier string) (*Session, error) {
	b, _ := json.Marshal(tokenRequest{CodeVerifier: verifier})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL+"/v1/auth/device/"+state+"/token", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(data))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decoding session: %w", err)
	}
	return &session, nil
}

// savePKCEPending writes PKCE state to a temp file for process-restart recovery.
func savePKCEPending(state, verifier string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, ".c4"), 0o700); err != nil {
		return err
	}
	path := filepath.Join(home, ".c4", ".pkce_pending_"+state+".json")
	data, _ := json.Marshal(map[string]interface{}{
		"state":         state,
		"code_verifier": verifier,
		"created_at":    time.Now().UTC(),
	})
	return os.WriteFile(path, data, 0o600)
}

// removePKCEPending removes the PKCE pending file (best-effort).
func removePKCEPending(state string) {
	home, _ := os.UserHomeDir()
	_ = os.Remove(filepath.Join(home, ".c4", ".pkce_pending_"+state+".json"))
}
