// Package api implements the C5 REST API server.
//
// It uses net/http stdlib with no external router dependencies,
// following the same pattern as c4-core/internal/daemon/server.go.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"crypto/sha256"

	"github.com/piqsol/c4/c5/internal/conversation"
	"github.com/piqsol/c4/c5/internal/eventpub"
	"github.com/piqsol/c4/c5/internal/knowledge"
	"github.com/piqsol/c4/c5/internal/llmclient"
	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/storage"
	"github.com/piqsol/c4/c5/internal/store"
)

// DoorayChannel holds per-channel routing configuration for Dooray messages.
type DoorayChannel struct {
	ProjectID  string
	WebhookURL string // optional; falls back to Server.doorayWebhookURL
}

// Server is the C5 HTTP API server.
type Server struct {
	store       *store.Store
	storage     storage.Backend
	estimator   *Estimator
	startTime   time.Time
	version     string
	apiKey      string // optional API key for authentication
	llmsTxt     string // llms.txt content
	docsFS      fs.FS  // docs filesystem (may be nil)
	serverURL   string // local server URL (fallback for publicURL)
	publicURL   string // external public URL (for device auth redirects)
	supabaseURL string // Supabase project URL for PKCE token exchange
	supabaseKey string // Supabase anon key for PKCE token exchange (apikey header)
	mux         *http.ServeMux
	done        chan struct{} // closed on shutdown to stop background goroutines
	eventPub         *eventpub.Publisher
	maxArtifactBytes int64 // max upload size for local backend
	gpuWorkerGPUOnly bool  // if true, GPU workers only accept GPU jobs (no CPU fallback)

	// LLM / Dooray server-side processing fields.
	llmClient        *llmclient.Client            // nil = server-side LLM disabled
	knowledgeClient  *knowledge.Client            // nil = knowledge search disabled
	doorayWebhookURL string                       // default Incoming Webhook URL
	doorayCmdToken   string                       // cmd token for slash command auth
	channelMap       map[string]DoorayChannel     // channelID → project routing
	llmSem           chan struct{}                 // semaphore: max concurrent LLM goroutines
	convStore        conversation.Store           // per-channel multi-turn conversation history

	// jobMu protects jobNotify. When a new job is queued, jobNotify is closed
	// (broadcasting to all long-poll waiters) and replaced with a new channel.
	jobMu     sync.Mutex
	jobNotify chan struct{}

	// sseSubs holds active SSE subscriber channels.
	// key: chan string (unique pointer), value: projectID string
	// Master key subscribers store "" and receive all events.
	// Entries are added in handleSSEStream and removed when the connection closes.
	sseSubs sync.Map
}

// Config holds server configuration.
type Config struct {
	Store            *store.Store
	Storage          storage.Backend // if nil, auto-detected from env
	Version          string
	APIKey           string // if non-empty, X-API-Key header is required
	ServerURL        string // server's external URL (for local storage fallback)
	PublicURL        string // external public URL (for device auth redirects, empty = ServerURL)
	LLMSTxt          string // llms.txt content (served at /.well-known/llms.txt)
	DocsFS           fs.FS  // embedded docs filesystem (served at /v1/docs/)
	EventBusURL      string // C3 EventBus base URL (empty = disabled)
	EventBusToken    string // Bearer token for EventBus (optional)
	MaxArtifactBytes int64  // max upload size for local backend (default 10GB)
	GPUWorkerGPUOnly bool   // if true, GPU workers only accept GPU jobs (no CPU fallback)
	SupabaseURL      string // Supabase project URL for PKCE token exchange (optional)
	SupabaseKey      string // Supabase anon key for PKCE token exchange (optional)
	// LLM / Dooray server-side processing (optional).
	LLMClient        *llmclient.Client        // nil = server-side LLM disabled
	KnowledgeClient  *knowledge.Client        // nil = knowledge search disabled
	DoorayWebhookURL string                   // default Incoming Webhook URL for LLM responses
	DoorayCmdToken   string                   // slash command token (overrides env var)
	ChannelMap       map[string]DoorayChannel // channelID → project routing
}

// NewServer creates an HTTP API server.
func NewServer(cfg Config) *Server {
	stor := cfg.Storage
	if stor == nil {
		stor = storage.NewBackend(cfg.ServerURL)
	}

	maxBytes := cfg.MaxArtifactBytes
	if maxBytes <= 0 {
		maxBytes = 10 << 30 // 10GB default
	}

	s := &Server{
		store:            cfg.Store,
		storage:          stor,
		estimator:        NewEstimator(cfg.Store),
		startTime:        time.Now(),
		version:          cfg.Version,
		apiKey:           cfg.APIKey,
		llmsTxt:          cfg.LLMSTxt,
		docsFS:           cfg.DocsFS,
		serverURL:        cfg.ServerURL,
		publicURL:        cfg.PublicURL,
		supabaseURL:      cfg.SupabaseURL,
		supabaseKey:      cfg.SupabaseKey,
		mux:              http.NewServeMux(),
		done:             make(chan struct{}),
		eventPub:         eventpub.New(cfg.EventBusURL, cfg.EventBusToken),
		jobNotify:        make(chan struct{}),
		maxArtifactBytes: maxBytes,
		gpuWorkerGPUOnly: cfg.GPUWorkerGPUOnly,
		llmClient:        cfg.LLMClient,
		knowledgeClient:  cfg.KnowledgeClient,
		doorayWebhookURL: cfg.DoorayWebhookURL,
		doorayCmdToken:   cfg.DoorayCmdToken,
		channelMap: cfg.ChannelMap,
		llmSem:     make(chan struct{}, 16), // max 16 concurrent LLM goroutines
		convStore: func() conversation.Store {
			if ss := conversation.NewSupabaseStore(cfg.SupabaseURL, cfg.SupabaseKey); ss != nil {
				return ss
			}
			return conversation.NewMemoryStore(20, 30*time.Minute)
		}(),
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
	s.mux.HandleFunc("/v1/deploy/target-status", s.handleDeployTargetStatus)
	s.mux.HandleFunc("/v1/deploy/assignments/", s.handleDeployAssignments)
	s.mux.HandleFunc("/v1/deploy", s.handleDeployList) // GET /v1/deploy?limit=50&offset=0
	s.mux.HandleFunc("/v1/deploy/", s.handleDeployStatus)

	// Artifacts & Storage
	s.mux.HandleFunc("/v1/storage/presigned-url", s.handlePresignedURL)
	s.mux.HandleFunc("/v1/storage/upload/", s.handleStoragePut)      // PUT /v1/storage/upload/{path} (local backend)
	s.mux.HandleFunc("/v1/storage/upload", s.handleUploadArtifact)   // POST /v1/storage/upload (legacy JSON)
	s.mux.HandleFunc("/v1/storage/download/", s.handleStorageDownload) // GET /v1/storage/download/{path}
	s.mux.HandleFunc("/v1/artifacts/", s.handleArtifacts)

	// SSE event stream
	s.mux.HandleFunc("/v1/events/stream", s.handleSSEStream)

	// WebSocket (both paths: hub.Client sends to /v1/ws/metrics/, workers use /ws/metrics/)
	s.mux.HandleFunc("/ws/metrics/", s.handleWSMetrics)
	s.mux.HandleFunc("/v1/ws/metrics/", s.handleWSMetrics)

	// Capabilities (MCP-like worker capability registry)
	s.mux.HandleFunc("/v1/capabilities", s.handleCapabilitiesList)
	s.mux.HandleFunc("/v1/capabilities/update", s.handleCapabilitiesUpdate)
	s.mux.HandleFunc("/v1/capabilities/invoke", s.handleCapabilitiesInvoke)

	// MCP server endpoint (Streamable HTTP, JSON-RPC 2.0)
	s.mux.HandleFunc("/v1/mcp", s.handleMCP)

	// Webhooks (public — token auth is self-contained per handler)
	s.mux.HandleFunc("/v1/webhooks/dooray", s.handleDooray)

	// Auth (OAuth PKCE device flow — no API key required)
	s.mux.HandleFunc("/auth/callback", s.handleAuthCallback)
	s.mux.HandleFunc("/auth/activate", s.handleActivate) // GET form, POST validate

	// Admin (requires master key)
	s.mux.HandleFunc("/v1/admin/api-keys", s.handleAdminAPIKeys)
	s.mux.HandleFunc("/v1/admin/api-keys/", s.handleAdminAPIKeyByHash)

	// Device auth (public endpoints)
	// /v1/auth/device  — POST create
	// /v1/auth/device/{state}       — GET poll
	// /v1/auth/device/{state}/token — POST token exchange
	s.mux.HandleFunc("/v1/auth/device", s.handleDeviceAuth)
	s.mux.HandleFunc("/v1/auth/device/", s.handleDeviceAuth)

	// Research state (C9 distributed state sharing)
	s.mux.HandleFunc("/v1/research/state", s.handleResearchState)
	s.mux.HandleFunc("/v1/research/state/lock", s.handleResearchStateLock)

	// LLMs.txt + docs
	s.registerLLMSTxtRoutes()
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public endpoints: health, llms.txt, docs, device auth, auth callback
		switch {
		case r.URL.Path == "/v1/health",
			r.URL.Path == "/llms.txt",
			r.URL.Path == "/.well-known/llms.txt",
			strings.HasPrefix(r.URL.Path, "/v1/docs/"),
			r.URL.Path == "/v1/auth/device",
			strings.HasPrefix(r.URL.Path, "/v1/auth/device/"),
			strings.HasPrefix(r.URL.Path, "/auth/activate"),
			r.URL.Path == "/auth/callback",
			r.URL.Path == "/v1/webhooks/dooray":
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("X-API-Key")
		if key == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		ctx := r.Context()

		// Check master key first
		if key == s.apiKey {
			ctx = context.WithValue(ctx, model.CtxProjectID, "")
			ctx = context.WithValue(ctx, model.CtxIsMaster, true)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Check per-project key
		h := sha256.Sum256([]byte(key))
		keyHash := fmt.Sprintf("%x", h[:])
		projectID, err := s.store.LookupAPIKey(keyHash)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		ctx = context.WithValue(ctx, model.CtxProjectID, projectID)
		ctx = context.WithValue(ctx, model.CtxIsMaster, false)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// projectIDFromContext returns the authenticated project ID from context.
// Empty string means master key (full access).
func projectIDFromContext(r *http.Request) string {
	if v, ok := r.Context().Value(model.CtxProjectID).(string); ok {
		return v
	}
	return ""
}

// isMasterFromContext returns true if the request was made with the master key.
func isMasterFromContext(r *http.Request) bool {
	if v, ok := r.Context().Value(model.CtxIsMaster).(bool); ok {
		return v
	}
	return false
}

// notifyJobAvailable broadcasts to all long-poll waiters that a new job is queued.
// It does this by closing the current jobNotify channel and creating a new one.
// It also broadcasts an SSE event to all SSE subscribers.
func (s *Server) notifyJobAvailable() {
	s.jobMu.Lock()
	old := s.jobNotify
	s.jobNotify = make(chan struct{})
	s.jobMu.Unlock()
	close(old)

	// Broadcast to SSE subscribers (non-blocking, fire-and-forget).
	// "" projectID = worker-level broadcast (all subscribers receive it).
	s.broadcastSSEEvent("", "job.available", nil)
}

// getJobNotifyChan returns the current job notification channel.
// Callers should capture it before blocking, then select on it.
func (s *Server) getJobNotifyChan() <-chan struct{} {
	s.jobMu.Lock()
	ch := s.jobNotify
	s.jobMu.Unlock()
	return ch
}

// Close stops background goroutines.
func (s *Server) Close() {
	close(s.done)
}

func (s *Server) leaseExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
		}

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

		// Cleanup old job logs/metrics (7-day retention)
		cleaned, err := s.store.CleanupOldJobs(7 * 24 * time.Hour)
		if err != nil {
			log.Printf("c5: cleanup old jobs error: %v", err)
		} else if cleaned > 0 {
			log.Printf("c5: cleaned %d old log/metric rows", cleaned)
		}

		// Cleanup expired conversation history entries.
		s.convStore.Cleanup()
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
