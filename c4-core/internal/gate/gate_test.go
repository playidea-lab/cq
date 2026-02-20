package gate_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/gate"
)

// ---------------------------------------------------------------------------
// TestWebhookDispatch — event → HTTP POST to registered endpoint
// ---------------------------------------------------------------------------

func TestWebhookDispatch(t *testing.T) {
	var received []byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := gate.NewWebhookManager(gate.WebhookConfig{
		DefaultTimeout: 5 * time.Second,
		MaxRetries:     0,
	})

	ep := mgr.RegisterEndpoint("test-ep", srv.URL, "", []string{"task.completed"})
	if ep.Name != "test-ep" {
		t.Fatalf("expected name=test-ep, got %s", ep.Name)
	}

	event := gate.Event{
		ID:   "evt-001",
		Type: "task.completed",
		Data: json.RawMessage(`{"task_id":"T-001"}`),
	}

	err := mgr.Dispatch(event)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected HTTP body to be non-empty")
	}

	var payload map[string]any
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	if payload["type"] != "task.completed" {
		t.Errorf("expected type=task.completed, got %v", payload["type"])
	}
	if payload["id"] != "evt-001" {
		t.Errorf("expected id=evt-001, got %v", payload["id"])
	}
}

func TestWebhookDispatchEventFiltering(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := gate.NewWebhookManager(gate.WebhookConfig{
		DefaultTimeout: 5 * time.Second,
	})

	// Only subscribe to task.completed
	mgr.RegisterEndpoint("ep", srv.URL, "", []string{"task.completed"})

	// Dispatch unrelated event — should NOT call the endpoint
	_ = mgr.Dispatch(gate.Event{ID: "1", Type: "task.blocked", Data: json.RawMessage(`{}`)})

	mu.Lock()
	if callCount != 0 {
		t.Errorf("expected 0 calls, got %d", callCount)
	}
	mu.Unlock()

	// Dispatch matching event — should call
	_ = mgr.Dispatch(gate.Event{ID: "2", Type: "task.completed", Data: json.RawMessage(`{}`)})

	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// TestWebhookHMAC — HMAC-SHA256 signature verification
// ---------------------------------------------------------------------------

func TestWebhookHMAC(t *testing.T) {
	secret := "super-secret-key"
	var sigHeader string
	var bodyReceived []byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigHeader = r.Header.Get("X-Gate-Signature")
		bodyReceived, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := gate.NewWebhookManager(gate.WebhookConfig{
		DefaultTimeout: 5 * time.Second,
	})
	mgr.RegisterEndpoint("secure-ep", srv.URL, secret, []string{"task.completed"})

	err := mgr.Dispatch(gate.Event{
		ID:   "evt-hmac",
		Type: "task.completed",
		Data: json.RawMessage(`{"result":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if sigHeader == "" {
		t.Fatal("expected X-Gate-Signature header to be set")
	}
	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Errorf("expected sha256= prefix, got %s", sigHeader)
	}

	// Verify the HMAC signature ourselves
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bodyReceived)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if sigHeader != expected {
		t.Errorf("HMAC mismatch: got %s, want %s", sigHeader, expected)
	}
}

func TestWebhookHMACAbsentWhenNoSecret(t *testing.T) {
	var sigHeader string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigHeader = r.Header.Get("X-Gate-Signature")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := gate.NewWebhookManager(gate.WebhookConfig{
		DefaultTimeout: 5 * time.Second,
	})
	mgr.RegisterEndpoint("public-ep", srv.URL, "", []string{"task.completed"})

	_ = mgr.Dispatch(gate.Event{ID: "e", Type: "task.completed", Data: json.RawMessage(`{}`)})

	mu.Lock()
	defer mu.Unlock()
	if sigHeader != "" {
		t.Errorf("expected no signature header when secret is empty, got %s", sigHeader)
	}
}

// ---------------------------------------------------------------------------
// TestSchedulerCron — cron schedule execution
// ---------------------------------------------------------------------------

func TestSchedulerCron(t *testing.T) {
	store := gate.NewMemoryJobStore()
	sched := gate.NewScheduler(store)
	defer sched.Stop()

	called := make(chan struct{}, 5)
	action := func() { called <- struct{}{} }

	// Every second cron expression
	job, err := sched.Schedule("@every 100ms", action)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Wait for at least 2 executions within 1 second
	count := 0
	timeout := time.After(1 * time.Second)
loop:
	for {
		select {
		case <-called:
			count++
			if count >= 2 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}
}

func TestSchedulerJobCancel(t *testing.T) {
	store := gate.NewMemoryJobStore()
	sched := gate.NewScheduler(store)
	defer sched.Stop()

	called := 0
	var mu sync.Mutex
	action := func() {
		mu.Lock()
		called++
		mu.Unlock()
	}

	job, err := sched.Schedule("@every 50ms", action)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	time.Sleep(120 * time.Millisecond)
	sched.Cancel(job.ID)
	mu.Lock()
	countAfterCancel := called
	mu.Unlock()

	// Wait and ensure no more calls happen
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if called != countAfterCancel {
		t.Errorf("job fired after cancel: before=%d, after=%d", countAfterCancel, called)
	}
}

// ---------------------------------------------------------------------------
// TestSlackConnector — Slack webhook send
// ---------------------------------------------------------------------------

func TestSlackConnector(t *testing.T) {
	var received map[string]any
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(body, &received)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	slack := gate.NewSlackConnector(srv.URL)
	err := slack.SendMessage("#general", "Hello from C4 gate!")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if received["channel"] != "#general" {
		t.Errorf("expected channel=#general, got %v", received["channel"])
	}
	if received["text"] != "Hello from C4 gate!" {
		t.Errorf("expected text='Hello from C4 gate!', got %v", received["text"])
	}
}

func TestSlackConnectorHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	slack := gate.NewSlackConnector(srv.URL)
	err := slack.SendMessage("#ch", "msg")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestGitHubConnector — GitHub API call (httptest)
// ---------------------------------------------------------------------------

func TestGitHubConnectorIssueComment(t *testing.T) {
	var received map[string]any
	var receivedPath string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedPath = r.URL.Path
		json.Unmarshal(body, &received)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "html_url": "https://github.com/example"})
	}))
	defer srv.Close()

	gh := gate.NewGitHubConnector(gate.GitHubConfig{
		PAT:     "ghp_test123",
		BaseURL: srv.URL,
	})

	err := gh.PostIssueComment("owner", "repo", 1, "Test comment from gate")
	if err != nil {
		t.Fatalf("PostIssueComment: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedPath != "/repos/owner/repo/issues/1/comments" {
		t.Errorf("unexpected path: %s", receivedPath)
	}
	if received["body"] != "Test comment from gate" {
		t.Errorf("expected body='Test comment from gate', got %v", received["body"])
	}
}

func TestGitHubConnectorPRComment(t *testing.T) {
	var receivedPath string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 99})
	}))
	defer srv.Close()

	gh := gate.NewGitHubConnector(gate.GitHubConfig{
		PAT:     "ghp_test456",
		BaseURL: srv.URL,
	})

	err := gh.PostPRComment("myorg", "myrepo", 42, "PR comment")
	if err != nil {
		t.Fatalf("PostPRComment: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// GitHub Issues and PRs share the same comments endpoint
	if receivedPath != "/repos/myorg/myrepo/issues/42/comments" {
		t.Errorf("unexpected path: %s", receivedPath)
	}
}

func TestGitHubConnectorAuthHeader(t *testing.T) {
	var authHeader string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeader = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	defer srv.Close()

	gh := gate.NewGitHubConnector(gate.GitHubConfig{
		PAT:     "my-pat-token",
		BaseURL: srv.URL,
	})

	_ = gh.PostIssueComment("o", "r", 1, "auth test")

	mu.Lock()
	defer mu.Unlock()
	if authHeader != "Bearer my-pat-token" {
		t.Errorf("expected Bearer auth, got %s", authHeader)
	}
}

func TestGitHubConnectorHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]any{"message": "Validation Failed"})
	}))
	defer srv.Close()

	gh := gate.NewGitHubConnector(gate.GitHubConfig{
		PAT:     "tok",
		BaseURL: srv.URL,
	})

	err := gh.PostIssueComment("o", "r", 1, "bad comment")
	if err == nil {
		t.Fatal("expected error on HTTP 422, got nil")
	}
}
