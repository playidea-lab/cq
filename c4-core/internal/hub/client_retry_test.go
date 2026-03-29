package hub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// =========================================================================
// parse429Wait
// =========================================================================

func TestParse429Wait_Integer(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", "30")
	got, ok := parse429Wait(resp)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != 30*time.Second {
		t.Errorf("parse429Wait = %v, want 30s", got)
	}
}

func TestParse429Wait_Zero(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", "0")
	got, ok := parse429Wait(resp)
	if !ok {
		t.Fatal("expected ok=true for Retry-After: 0")
	}
	if got != 0 {
		t.Errorf("parse429Wait = %v, want 0", got)
	}
}

func TestParse429Wait_Missing(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	_, ok := parse429Wait(resp)
	if ok {
		t.Error("expected ok=false when header is absent")
	}
}

func TestParse429Wait_HTTPDate(t *testing.T) {
	future := time.Now().Add(45 * time.Second)
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", future.UTC().Format(http.TimeFormat))
	got, ok := parse429Wait(resp)
	if !ok {
		t.Fatal("expected ok=true for HTTP-date header")
	}
	// Allow a generous range due to clock drift in the test.
	if got < 40*time.Second || got > 50*time.Second {
		t.Errorf("parse429Wait = %v, want ~45s", got)
	}
}

func TestParse429Wait_PastDate(t *testing.T) {
	past := time.Now().Add(-10 * time.Second)
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", past.UTC().Format(http.TimeFormat))
	got, ok := parse429Wait(resp)
	if !ok {
		t.Fatal("expected ok=true for past HTTP-date (returns 0 duration)")
	}
	if got != 0 {
		t.Errorf("parse429Wait = %v, want 0 for past date", got)
	}
}

func TestParse429Wait_InvalidValue(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", "not-a-number")
	_, ok := parse429Wait(resp)
	if ok {
		t.Error("expected ok=false for unparseable Retry-After value")
	}
}

// =========================================================================
// doWithRetry — 5xx behaviour (unchanged from before)
// =========================================================================

func TestDoWithRetry_5xx_ExhaustsMaxRetries(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	_, err := client.doWithRetry(req)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if calls.Load() != int32(maxRetries) {
		t.Errorf("calls = %d, want %d", calls.Load(), maxRetries)
	}
}

func TestDoWithRetry_5xx_SucceedsOnRetry(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

// =========================================================================
// doWithRetry — 429 adaptive backoff
// =========================================================================

// TestDoWithRetry_429_RespectsRetryAfterHeader verifies the client retries
// on 429 and ultimately succeeds when the server sends Retry-After.
func TestDoWithRetry_429_RespectsRetryAfterHeader(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0") // instruct 0s wait to keep test fast
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

// TestDoWithRetry_429_ExhaustsMax429Attempts verifies the client gives up
// after max429Attempts consecutive 429 responses and returns an error.
func TestDoWithRetry_429_ExhaustsMax429Attempts(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0") // no real wait
		w.WriteHeader(429)
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	_, err := client.doWithRetry(req)
	if err == nil {
		t.Fatal("expected error after exhausting 429 retries")
	}
	// First attempt + max429Attempts-1 retries = max429Attempts total calls.
	if calls.Load() != int32(max429Attempts) {
		t.Errorf("calls = %d, want %d", calls.Load(), max429Attempts)
	}
}

// TestDoWithRetry_429_IndependentOf5xxRetries verifies that 429 retries
// and 5xx retries are counted independently — a 429 followed by a 500
// should not exhaust 5xx retries prematurely.
func TestDoWithRetry_429_IndependentOf5xxRetries(t *testing.T) {
	responses := []int{429, 503, 200}
	var idx atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(idx.Add(1)) - 1
		if i < len(responses) {
			if responses[i] == 429 {
				w.Header().Set("Retry-After", "0")
			}
			w.WriteHeader(responses[i])
		} else {
			w.WriteHeader(500)
		}
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestDoWithRetry_429_NoRetryAfterHeader verifies exponential backoff is
// used when Retry-After header is absent (the test uses Retry-After "0"
// when it wants instant retry; here we test with no header, using a very
// short base wait via a subtest that overrides the constant indirectly by
// ensuring the code path is exercised and succeeds).
func TestDoWithRetry_429_NoRetryAfterHeader_Succeeds(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// No Retry-After header — client uses exponential backoff.
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()

	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// We patch retry429BaseWait via a local request-level test.
	// Since the constant is package-level, we rely on the default (5s).
	// To keep the test fast, skip if running in short mode.
	if testing.Short() {
		t.Skip("skipping: exponential 429 backoff takes ≥5s")
	}

	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
