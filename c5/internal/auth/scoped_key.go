// Package auth provides API key scope enforcement for the C5 Hub.
//
// Key prefixes:
//   - sk-user-*   → user scope (submit jobs, query status)
//   - sk-worker-* → worker scope (poll jobs, report results)
//   - no prefix   → full access (backwards compatible)
package auth

import (
	"net/http"
	"strings"
)

// Scope represents the access level of an API key.
type Scope string

const (
	// ScopeFull grants access to all endpoints (legacy keys without prefix).
	ScopeFull Scope = "full"
	// ScopeUser grants access to job submission and query endpoints.
	ScopeUser Scope = "user"
	// ScopeWorker grants access to job polling and completion endpoints.
	ScopeWorker Scope = "worker"
)

const (
	// PrefixUser is the key prefix for user-scoped keys.
	PrefixUser = "sk-user-"
	// PrefixWorker is the key prefix for worker-scoped keys.
	PrefixWorker = "sk-worker-"
)

// ParseScope extracts the scope from an API key based on its prefix.
// Keys without a recognized prefix are treated as full-access (backwards compatible).
func ParseScope(key string) Scope {
	switch {
	case strings.HasPrefix(key, PrefixUser):
		return ScopeUser
	case strings.HasPrefix(key, PrefixWorker):
		return ScopeWorker
	default:
		return ScopeFull
	}
}

// ValidScope returns true if s is a recognized scope value.
func ValidScope(s string) bool {
	switch Scope(s) {
	case ScopeFull, ScopeUser, ScopeWorker:
		return true
	}
	return false
}

// userEndpoints are URL path patterns accessible to user-scoped keys.
// Matched by prefix — e.g., "/v1/jobs/submit" matches a request to that exact path.
var userEndpoints = []string{
	"/v1/jobs/submit",  // POST submit a job
	"/v1/jobs",         // GET list jobs / GET /v1/jobs/{id}
	"/v1/stats/queue",  // GET queue stats
	"/v1/dags",         // DAG operations
	"/v1/capabilities", // GET list capabilities, POST invoke
	"/v1/storage/",     // upload/download artifacts
	"/v1/artifacts/",   // artifact management
	"/v1/events/stream", // SSE stream
}

// workerEndpoints are URL path patterns accessible to worker-scoped keys.
// NOTE: /v1/jobs/submit is NOT included — workers poll via leases, not submit.
// /v1/jobs/ (with trailing slash) allows /v1/jobs/{id} but NOT /v1/jobs/submit
// because we use an explicit deny list for ambiguous patterns.
var workerEndpoints = []string{
	"/v1/workers/register",    // POST register
	"/v1/workers/heartbeat",   // POST heartbeat
	"/v1/workers",             // GET list workers
	"/v1/leases/acquire",      // POST acquire lease (poll for jobs)
	"/v1/leases/renew",        // POST renew lease
	"/v1/metrics/",            // POST metrics
	"/v1/capabilities/update", // POST update capabilities
	"/v1/storage/",            // upload/download artifacts (workers upload results)
	"/v1/ws/metrics/",         // WebSocket metrics
	"/ws/metrics/",            // WebSocket metrics (alt path)
}

// workerJobDenyPrefixes lists job sub-paths that workers CANNOT access.
// Workers need to read job details and report completion, but NOT submit new jobs.
var workerJobDenyPrefixes = []string{
	"/v1/jobs/submit", // workers cannot submit jobs
}

// CheckScope verifies that the given scope is allowed to access the request path.
// Returns true if access is permitted.
func CheckScope(scope Scope, r *http.Request) bool {
	if scope == ScopeFull {
		return true
	}

	path := r.URL.Path

	switch scope {
	case ScopeUser:
		return matchesAny(path, userEndpoints)
	case ScopeWorker:
		// Workers can access /v1/jobs/{id} but NOT /v1/jobs/submit
		if strings.HasPrefix(path, "/v1/jobs/") {
			for _, deny := range workerJobDenyPrefixes {
				if path == deny || strings.HasPrefix(path, deny) {
					return false
				}
			}
			return true // /v1/jobs/{id}, /v1/jobs/{id}/complete, etc.
		}
		return matchesAny(path, workerEndpoints)
	default:
		return false
	}
}

// matchesAny returns true if path matches any of the patterns.
// A pattern matches if the path equals it or starts with it (prefix match).
func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if path == p || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
