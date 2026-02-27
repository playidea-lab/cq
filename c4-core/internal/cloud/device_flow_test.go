package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLoginWithDevice_PollUntilReady simulates: pending×2 → ready → token exchange.
func TestLoginWithDevice_PollUntilReady(t *testing.T) {
	pollCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device":
			// Init: return state + user_code + device_auth_url
			json.NewEncoder(w).Encode(deviceInitResponse{
				State:         "test-state-001",
				UserCode:      "ABC-123",
				DeviceAuthURL: "https://hub.example.com/device",
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/device/test-state-001":
			pollCount++
			if pollCount <= 2 {
				json.NewEncoder(w).Encode(deviceStatusResponse{Status: "pending"})
			} else {
				json.NewEncoder(w).Encode(deviceStatusResponse{Status: "ready"})
			}

		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device/test-state-001/token":
			// Verify code_verifier is present
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["code_verifier"] == "" {
				http.Error(w, "missing code_verifier", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(Session{
				AccessToken:  "access-tok",
				RefreshToken: "refresh-tok",
				ExpiresAt:    time.Now().Add(time.Hour).Unix(),
				User:         User{ID: "u1", Email: "user@example.com"},
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Speed up polling for the test
	origInterval := devicePollInterval
	devicePollInterval = 10 * time.Millisecond
	defer func() { devicePollInterval = origInterval }()

	cfg := DeviceFlowConfig{HubURL: srv.URL, SupabaseURL: "https://test.supabase.co"}
	ctx := context.Background()

	session, err := LoginWithDevice(ctx, cfg)
	if err != nil {
		t.Fatalf("LoginWithDevice() error: %v", err)
	}
	if session.AccessToken != "access-tok" {
		t.Errorf("AccessToken = %q, want %q", session.AccessToken, "access-tok")
	}
	if pollCount < 3 {
		t.Errorf("expected at least 3 polls (2 pending + 1 ready), got %d", pollCount)
	}
}

// TestLoginWithDevice_Expired verifies that status="expired" returns an error.
func TestLoginWithDevice_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device":
			json.NewEncoder(w).Encode(deviceInitResponse{
				State:         "exp-state",
				UserCode:      "EXP-999",
				DeviceAuthURL: "https://hub.example.com/device",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/device/exp-state":
			json.NewEncoder(w).Encode(deviceStatusResponse{Status: "expired"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origInterval := devicePollInterval
	devicePollInterval = 10 * time.Millisecond
	defer func() { devicePollInterval = origInterval }()

	cfg := DeviceFlowConfig{HubURL: srv.URL, SupabaseURL: "https://test.supabase.co"}
	_, err := LoginWithDevice(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for expired status, got nil")
	}
	if !strings.Contains(err.Error(), "expired") && !strings.Contains(err.Error(), "만료") {
		t.Errorf("error = %q, want to contain \"expired\" or \"만료\"", err.Error())
	}
}

// TestLoginWithDevice_Timeout verifies that context deadline causes an error.
func TestLoginWithDevice_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device":
			json.NewEncoder(w).Encode(deviceInitResponse{
				State:         "timeout-state",
				UserCode:      "TO-001",
				DeviceAuthURL: "https://hub.example.com/device",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/device/timeout-state":
			// Always pending — simulate timeout
			json.NewEncoder(w).Encode(deviceStatusResponse{Status: "pending"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origInterval := devicePollInterval
	devicePollInterval = 10 * time.Millisecond
	defer func() { devicePollInterval = origInterval }()

	// Override the flow timeout with a very short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := DeviceFlowConfig{HubURL: srv.URL, SupabaseURL: "https://test.supabase.co"}

	// We need to bypass the internal deviceFlowTimeout and use our ctx directly.
	// LoginWithDevice wraps ctx with deviceFlowTimeout; our cancel fires first.
	_, err := LoginWithDevice(ctx, cfg)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// TestLoginWithLink_PrintsURL verifies that the auth_url is written to stdout.
func TestLoginWithLink_PrintsURL(t *testing.T) {
	const wantURL = "https://hub.example.com/auth/link/abc123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device":
			// Verify flow=link is set
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["flow"] != "link" {
				http.Error(w, "missing flow=link", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(deviceInitResponse{
				State:   "link-state",
				AuthURL: wantURL,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/device/link-state":
			json.NewEncoder(w).Encode(deviceStatusResponse{Status: "ready"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device/link-state/token":
			json.NewEncoder(w).Encode(Session{
				AccessToken:  "link-tok",
				RefreshToken: "link-refresh",
				ExpiresAt:    time.Now().Add(time.Hour).Unix(),
				User:         User{ID: "u2", Email: "link@example.com"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origInterval := devicePollInterval
	devicePollInterval = 10 * time.Millisecond
	defer func() { devicePollInterval = origInterval }()

	// Capture stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := DeviceFlowConfig{HubURL: srv.URL, SupabaseURL: "https://test.supabase.co"}
	session, err := LoginWithLink(context.Background(), cfg)

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("LoginWithLink() error: %v", err)
	}
	if !strings.Contains(output, wantURL) {
		t.Errorf("stdout = %q, want to contain %q", output, wantURL)
	}
	if session.AccessToken != "link-tok" {
		t.Errorf("AccessToken = %q, want %q", session.AccessToken, "link-tok")
	}
}
