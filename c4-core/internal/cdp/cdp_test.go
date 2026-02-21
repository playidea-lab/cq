package cdp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// --- Unit tests (no browser required) ---

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner() returned nil")
	}
}

func TestResolveTimeout(t *testing.T) {
	tests := []struct {
		name string
		opts *RunOptions
		want time.Duration
	}{
		{"nil opts", nil, DefaultTimeout},
		{"zero timeout", &RunOptions{TimeoutSeconds: 0}, DefaultTimeout},
		{"negative timeout", &RunOptions{TimeoutSeconds: -5}, DefaultTimeout},
		{"normal timeout", &RunOptions{TimeoutSeconds: 60}, 60 * time.Second},
		{"max clamp", &RunOptions{TimeoutSeconds: 999}, MaxTimeout},
		{"exactly max", &RunOptions{TimeoutSeconds: 300}, MaxTimeout},
		{"below max", &RunOptions{TimeoutSeconds: 299}, 299 * time.Second},
		{"one second", &RunOptions{TimeoutSeconds: 1}, 1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTimeout(tt.opts)
			if got != tt.want {
				t.Errorf("resolveTimeout(%v) = %v, want %v", tt.opts, got, tt.want)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty url", "", true},
		{"localhost http", "http://localhost:9222", false},
		{"localhost https", "https://localhost:9222", false},
		{"127.0.0.1", "http://127.0.0.1:9222", false},
		{"ipv6 loopback", "http://[::1]:9222", false},
		{"remote host", "http://example.com:9222", true},
		{"remote ip", "http://192.168.1.1:9222", true},
		{"no scheme", "://bad", true},
		{"localhost no port", "http://localhost", false},
		{"ws scheme localhost", "ws://localhost:9222", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestExecute_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty url", func(t *testing.T) {
		_, err := r.Execute(ctx, "", "1+1", nil)
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})

	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.Execute(ctx, "http://evil.com:9222", "1+1", nil)
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})

	t.Run("empty script", func(t *testing.T) {
		_, err := r.Execute(ctx, "http://localhost:9222", "", nil)
		if err == nil {
			t.Fatal("expected error for empty script")
		}
	})
}

func TestListTargets_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty url", func(t *testing.T) {
		_, err := r.ListTargets(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})

	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.ListTargets(ctx, "http://remote.host:9222")
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})
}

func TestConstants(t *testing.T) {
	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want 30s", DefaultTimeout)
	}
	if MaxTimeout != 300*time.Second {
		t.Errorf("MaxTimeout = %v, want 300s", MaxTimeout)
	}
	if ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout = %v, want 5s", ConnectTimeout)
	}
}

// --- Element-ref unit tests ---

func TestValidateRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{"valid c4r-0", "c4r-0", false},
		{"valid c4r-42", "c4r-42", false},
		{"valid c4r-999", "c4r-999", false},
		{"empty", "", true},
		{"wrong prefix", "ref-0", true},
		{"no number", "c4r-", true},
		{"injection attempt", `c4r-0"]; alert(1);//`, true},
		{"spaces", "c4r- 0", true},
		{"old format", "cdp-ref-0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRef(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

func TestScanElements_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	t.Run("empty url", func(t *testing.T) {
		_, err := r.ScanElements(ctx, "", "", 0)
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})
	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.ScanElements(ctx, "http://remote.host:9222", "", 0)
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})
}

func TestClickByRef_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	t.Run("empty url", func(t *testing.T) {
		_, err := r.ClickByRef(ctx, "", "c4r-0", "", 0)
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})
	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.ClickByRef(ctx, "http://remote.host:9222", "c4r-0", "", 0)
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})
	t.Run("invalid ref", func(t *testing.T) {
		_, err := r.ClickByRef(ctx, "http://localhost:9222", "bad-ref", "", 0)
		if err == nil {
			t.Fatal("expected error for invalid ref")
		}
	})
}

func TestTypeByRef_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	t.Run("empty url", func(t *testing.T) {
		_, err := r.TypeByRef(ctx, "", "c4r-0", "text", "", 0)
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})
	t.Run("invalid ref", func(t *testing.T) {
		_, err := r.TypeByRef(ctx, "http://localhost:9222", "bad-ref", "text", "", 0)
		if err == nil {
			t.Fatal("expected error for invalid ref")
		}
	})
	t.Run("empty text", func(t *testing.T) {
		_, err := r.TypeByRef(ctx, "http://localhost:9222", "c4r-0", "", "", 0)
		if err == nil || !strings.Contains(err.Error(), "text is required") {
			t.Fatalf("expected 'text is required' error, got: %v", err)
		}
	})
	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.TypeByRef(ctx, "http://remote.host:9222", "c4r-0", "text", "", 0)
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})
}

func TestGetTextByRef_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	t.Run("empty url", func(t *testing.T) {
		_, err := r.GetTextByRef(ctx, "", "c4r-0", "", 0)
		if err == nil {
			t.Fatal("expected error for empty url")
		}
	})
	t.Run("remote url rejected", func(t *testing.T) {
		_, err := r.GetTextByRef(ctx, "http://remote.host:9222", "c4r-0", "", 0)
		if err == nil {
			t.Fatal("expected error for remote url")
		}
	})
	t.Run("invalid ref", func(t *testing.T) {
		_, err := r.GetTextByRef(ctx, "http://localhost:9222", "bad-ref", "", 0)
		if err == nil {
			t.Fatal("expected error for invalid ref")
		}
	})
}

// --- Integration tests (require a running browser with --remote-debugging-port) ---

// cdpDebugURL returns the CDP debug URL if a browser is available, or skips the test.
func cdpDebugURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CDP_DEBUG_URL")
	if url == "" {
		url = "http://localhost:9222"
	}
	// Quick check: try to reach the /json endpoint.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/json/version", url))
	if err != nil {
		t.Skipf("No browser at %s: %v", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Browser at %s returned status %d", url, resp.StatusCode)
	}
	return url
}

func TestIntegration_ListTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	debugURL := cdpDebugURL(t)
	r := NewRunner()
	ctx := context.Background()

	targets, err := r.ListTargets(ctx, debugURL)
	if err != nil {
		t.Fatalf("ListTargets() error: %v", err)
	}
	// A running browser should have at least one target.
	if len(targets) == 0 {
		t.Log("Warning: browser has no open targets")
	}
	for _, tgt := range targets {
		t.Logf("Target: id=%s type=%s title=%q url=%s", tgt.ID, tgt.Type, tgt.Title, tgt.URL)
		if tgt.ID == "" {
			t.Error("target has empty ID")
		}
	}
}

func TestIntegration_Execute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	debugURL := cdpDebugURL(t)
	r := NewRunner()
	ctx := context.Background()

	t.Run("simple expression", func(t *testing.T) {
		result, err := r.Execute(ctx, debugURL, "1 + 1", nil)
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result == nil {
			t.Fatal("Execute() returned nil result")
		}
		// chromedp returns numbers as float64.
		val, ok := result.Result.(float64)
		if !ok {
			t.Fatalf("expected float64 result, got %T: %v", result.Result, result.Result)
		}
		if val != 2 {
			t.Errorf("1+1 = %v, want 2", val)
		}
		if result.ElapsedMs < 0 {
			t.Error("negative elapsed time")
		}
	})

	t.Run("with navigate", func(t *testing.T) {
		result, err := r.Execute(ctx, debugURL, "document.title", &RunOptions{
			TargetURL:      "about:blank",
			TimeoutSeconds: 10,
		})
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result.TargetURL != "about:blank" {
			t.Errorf("TargetURL = %q, want %q", result.TargetURL, "about:blank")
		}
	})

	t.Run("custom timeout", func(t *testing.T) {
		result, err := r.Execute(ctx, debugURL, "42", &RunOptions{
			TimeoutSeconds: 5,
		})
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		val, ok := result.Result.(float64)
		if !ok {
			t.Fatalf("expected float64 result, got %T", result.Result)
		}
		if val != 42 {
			t.Errorf("result = %v, want 42", val)
		}
	})
}
