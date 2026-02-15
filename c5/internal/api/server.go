// Package api implements the C5 REST API server.
//
// It uses net/http stdlib with no external router dependencies,
// following the same pattern as c4-core/internal/daemon/server.go.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/piqsol/c4/c5/internal/storage"
	"github.com/piqsol/c4/c5/internal/store"
)

// Server is the C5 HTTP API server.
type Server struct {
	store     *store.Store
	storage   storage.Backend
	estimator *Estimator
	startTime time.Time
	version   string
	apiKey    string // optional API key for authentication
	mux       *http.ServeMux
}

// Config holds server configuration.
type Config struct {
	Store     *store.Store
	Storage   storage.Backend // if nil, auto-detected from env
	Version   string
	APIKey    string // if non-empty, X-API-Key header is required
	ServerURL string // server's external URL (for local storage fallback)
}

// NewServer creates an HTTP API server.
func NewServer(cfg Config) *Server {
	stor := cfg.Storage
	if stor == nil {
		stor = storage.NewBackend(cfg.ServerURL)
	}

	s := &Server{
		store:     cfg.Store,
		storage:   stor,
		estimator: NewEstimator(cfg.Store),
		startTime: time.Now(),
		version:   cfg.Version,
		apiKey:    cfg.APIKey,
		mux:       http.NewServeMux(),
	}
	s.registerRoutes()

	// Start background lease expiry goroutine
	go s.leaseExpiryLoop()

	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.apiKey != "" {
		return s.authMiddleware(s.mux)
	}
	return s.mux
}

func (s *Server) registerRoutes() {
	// Health & stats
	s.mux.HandleFunc("/v1/health", s.handleHealth)
	s.mux.HandleFunc("/v1/stats/queue", s.handleQueueStats)

	// Jobs
	s.mux.HandleFunc("/v1/jobs/submit", s.handleJobSubmit)
	s.mux.HandleFunc("/v1/jobs", s.handleJobsList)
	s.mux.HandleFunc("/v1/jobs/", s.handleJobByID)

	// Workers
	s.mux.HandleFunc("/v1/workers/register", s.handleWorkerRegister)
	s.mux.HandleFunc("/v1/workers/heartbeat", s.handleWorkerHeartbeat)
	s.mux.HandleFunc("/v1/workers", s.handleWorkersList)

	// Leases
	s.mux.HandleFunc("/v1/leases/acquire", s.handleLeaseAcquire)
	s.mux.HandleFunc("/v1/leases/renew", s.handleLeaseRenew)

	// Metrics
	s.mux.HandleFunc("/v1/metrics/", s.handleMetrics)

	// DAGs
	s.mux.HandleFunc("/v1/dags/from-yaml", s.handleDAGFromYAML)
	s.mux.HandleFunc("/v1/dags", s.handleDAGsList)
	s.mux.HandleFunc("/v1/dags/", s.handleDAGByID)

	// Edges
	s.mux.HandleFunc("/v1/edges/register", s.handleEdgeRegister)
	s.mux.HandleFunc("/v1/edges/heartbeat", s.handleEdgeHeartbeat)
	s.mux.HandleFunc("/v1/edges", s.handleEdgesList)
	s.mux.HandleFunc("/v1/edges/", s.handleEdgeByID)

	// Deploy
	s.mux.HandleFunc("/v1/deploy/rules", s.handleDeployRulesList)
	s.mux.HandleFunc("/v1/deploy/rules/", s.handleDeployRuleByID)
	s.mux.HandleFunc("/v1/deploy/trigger", s.handleDeployTrigger)
	s.mux.HandleFunc("/v1/deploy", s.handleDeployList) // GET /v1/deploy?limit=50&offset=0
	s.mux.HandleFunc("/v1/deploy/", s.handleDeployStatus)

	// Artifacts & Storage
	s.mux.HandleFunc("/v1/storage/presigned-url", s.handlePresignedURL)
	s.mux.HandleFunc("/v1/storage/upload", s.handleUploadArtifact)
	s.mux.HandleFunc("/v1/artifacts/", s.handleArtifacts)

	// WebSocket (both paths: hub.Client sends to /v1/ws/metrics/, workers use /ws/metrics/)
	s.mux.HandleFunc("/ws/metrics/", s.handleWSMetrics)
	s.mux.HandleFunc("/v1/ws/metrics/", s.handleWSMetrics)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health endpoint is always accessible
		if r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		if key != s.apiKey {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) leaseExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		n, err := s.store.ExpireLeases()
		if err != nil {
			log.Printf("c5: lease expiry error: %v", err)
		} else if n > 0 {
			log.Printf("c5: expired %d leases, jobs re-queued", n)
		}

		// Also mark stale workers
		stale, err := s.store.MarkStaleWorkers(2 * time.Minute)
		if err != nil {
			log.Printf("c5: stale worker check error: %v", err)
		} else if stale > 0 {
			log.Printf("c5: marked %d workers as offline", stale)
		}

		// Mark stale edges
		staleEdges, err := s.store.MarkStaleEdges(5 * time.Minute)
		if err != nil {
			log.Printf("c5: stale edge check error: %v", err)
		} else if staleEdges > 0 {
			log.Printf("c5: marked %d edges as offline", staleEdges)
		}
	}
}

// =========================================================================
// Helpers
// =========================================================================

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
