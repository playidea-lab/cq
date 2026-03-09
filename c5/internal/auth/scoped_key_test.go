package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseScope(t *testing.T) {
	tests := []struct {
		key  string
		want Scope
	}{
		{"sk-user-abc123", ScopeUser},
		{"sk-worker-xyz789", ScopeWorker},
		{"c5pk_deadbeef12345678", ScopeFull},       // legacy project key
		{"my-secret-key", ScopeFull},                // plain key
		{"", ScopeFull},                             // empty key
		{"sk-user-", ScopeUser},                     // prefix only
		{"sk-worker-", ScopeWorker},                 // prefix only
		{"SK-USER-abc", ScopeFull},                  // case sensitive
		{"sk-user", ScopeFull},                      // missing trailing dash
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := ParseScope(tt.key)
			if got != tt.want {
				t.Errorf("ParseScope(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestValidScope(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"full", true},
		{"user", true},
		{"worker", true},
		{"admin", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidScope(tt.s); got != tt.want {
			t.Errorf("ValidScope(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestCheckScope_Full(t *testing.T) {
	paths := []string{
		"/v1/jobs/submit",
		"/v1/workers/register",
		"/v1/leases/acquire",
		"/v1/admin/api-keys",
		"/v1/anything",
	}
	for _, p := range paths {
		r := httptest.NewRequest("GET", p, nil)
		if !CheckScope(ScopeFull, r) {
			t.Errorf("ScopeFull should allow %s", p)
		}
	}
}

func TestCheckScope_User(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/jobs/submit"},
		{"GET", "/v1/jobs"},
		{"GET", "/v1/jobs/abc-123"},
		{"GET", "/v1/stats/queue"},
		{"GET", "/v1/capabilities"},
		{"POST", "/v1/capabilities/invoke"},
		{"GET", "/v1/dags"},
		{"GET", "/v1/dags/dag-1"},
		{"GET", "/v1/events/stream"},
		{"GET", "/v1/storage/download/file.tar"},
		{"GET", "/v1/artifacts/abc"},
	}
	for _, tt := range allowed {
		r := httptest.NewRequest(tt.method, tt.path, nil)
		if !CheckScope(ScopeUser, r) {
			t.Errorf("ScopeUser should allow %s %s", tt.method, tt.path)
		}
	}

	denied := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/workers/register"},
		{"POST", "/v1/workers/heartbeat"},
		{"POST", "/v1/leases/acquire"},
		{"POST", "/v1/leases/renew"},
		{"GET", "/v1/admin/api-keys"},
	}
	for _, tt := range denied {
		r := httptest.NewRequest(tt.method, tt.path, nil)
		if CheckScope(ScopeUser, r) {
			t.Errorf("ScopeUser should deny %s %s", tt.method, tt.path)
		}
	}
}

func TestCheckScope_Worker(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/workers/register"},
		{"POST", "/v1/workers/heartbeat"},
		{"GET", "/v1/workers"},
		{"POST", "/v1/leases/acquire"},
		{"POST", "/v1/leases/renew"},
		{"GET", "/v1/jobs/abc-123"},
		{"PUT", "/v1/jobs/abc-123/complete"},
		{"POST", "/v1/metrics/abc-123"},
		{"POST", "/v1/capabilities/update"},
		{"GET", "/v1/storage/download/file.tar"},
		{"GET", "/v1/ws/metrics/abc"},
		{"GET", "/ws/metrics/abc"},
	}
	for _, tt := range allowed {
		r := httptest.NewRequest(tt.method, tt.path, nil)
		if !CheckScope(ScopeWorker, r) {
			t.Errorf("ScopeWorker should allow %s %s", tt.method, tt.path)
		}
	}

	denied := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/jobs/submit"},
		{"GET", "/v1/stats/queue"},
		{"GET", "/v1/admin/api-keys"},
		{"GET", "/v1/dags"},
		{"GET", "/v1/events/stream"},
	}
	for _, tt := range denied {
		r := httptest.NewRequest(tt.method, tt.path, nil)
		if CheckScope(ScopeWorker, r) {
			t.Errorf("ScopeWorker should deny %s %s", tt.method, tt.path)
		}
	}
}

func TestCheckScope_Unknown(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/jobs", nil)
	if CheckScope(Scope("unknown"), r) {
		t.Error("unknown scope should deny all")
	}
}

func TestMatchesAny(t *testing.T) {
	patterns := []string{"/v1/jobs", "/v1/workers/"}

	tests := []struct {
		path string
		want bool
	}{
		{"/v1/jobs", true},
		{"/v1/jobs/abc", true},    // prefix match
		{"/v1/workers/reg", true}, // prefix match
		{"/v1/health", false},
		{"/v1/job", false},        // not a prefix of /v1/jobs
	}
	for _, tt := range tests {
		if got := matchesAny(tt.path, patterns); got != tt.want {
			t.Errorf("matchesAny(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestScopeEndpointConsistency verifies worker can access /v1/jobs/{id}
// (needed to report completion) but user cannot access worker-only paths.
func TestScopeEndpointConsistency(t *testing.T) {
	// Worker completing a job: PUT /v1/jobs/{id}/complete
	r := httptest.NewRequest(http.MethodPut, "/v1/jobs/job-123/complete", nil)
	if !CheckScope(ScopeWorker, r) {
		t.Error("worker must be able to complete jobs via /v1/jobs/{id}/complete")
	}

	// User submitting a job
	r = httptest.NewRequest(http.MethodPost, "/v1/jobs/submit", nil)
	if !CheckScope(ScopeUser, r) {
		t.Error("user must be able to submit jobs")
	}
	if CheckScope(ScopeWorker, r) {
		t.Error("worker should NOT be able to submit jobs")
	}
}
