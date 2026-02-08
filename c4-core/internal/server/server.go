// Package server provides the C4 Cloud HTTP server for team collaboration.
//
// Uses chi router to serve:
//   - POST /api/chat           - LLM proxy (Claude API SSE)
//   - POST /api/workers/spawn  - Cloud Worker creation (Fly.io)
//   - DELETE /api/workers/:id  - Worker deletion
//   - POST /api/webhooks/github - GitHub webhook receiver
//   - GET  /api/c4/status      - C4 status proxy
//   - GET  /health             - Health check
//
// Middleware stack: CORS -> Auth (JWT) -> Plan Gating -> Handler
package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds server configuration.
type Config struct {
	Port            int
	Host            string
	CORSOrigins     []string
	SupabaseURL     string
	SupabaseAnonKey string
	JWTSecret       string
	WebhookSecret   string  // GitHub webhook secret
	ClaudeAPIKey    string
	FlyAPIToken     string
	AllowedPlans    []string // Plans allowed to access (e.g., ["pro", "team", "enterprise"])
	ShutdownTimeout time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Port: 8080,
		Host: "0.0.0.0",
		CORSOrigins: []string{
			"http://localhost:3000",
			"http://localhost:8000",
			"http://127.0.0.1:3000",
		},
		AllowedPlans:    []string{"pro", "team", "enterprise"},
		ShutdownTimeout: 10 * time.Second,
	}
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if port := os.Getenv("PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.Port)
	}
	if host := os.Getenv("HOST"); host != "" {
		cfg.Host = host
	}
	if origins := os.Getenv("CORS_ORIGINS"); origins != "" {
		cfg.CORSOrigins = strings.Split(origins, ",")
	}

	cfg.SupabaseURL = os.Getenv("SUPABASE_URL")
	cfg.SupabaseAnonKey = os.Getenv("SUPABASE_ANON_KEY")
	cfg.JWTSecret = os.Getenv("SUPABASE_JWT_SECRET")
	cfg.WebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	cfg.ClaudeAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	cfg.FlyAPIToken = os.Getenv("FLY_API_TOKEN")

	return cfg
}

// User represents an authenticated user from JWT.
type User struct {
	ID       string `json:"sub"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	TeamID   string `json:"team_id,omitempty"`
	Plan     string `json:"plan,omitempty"` // free, pro, team, enterprise
}

// Router is the main HTTP router interface.
type Router interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// Server is the C4 Cloud HTTP server.
type Server struct {
	config     *Config
	handler    http.Handler
	httpServer *http.Server
	active     atomic.Int64 // active connection count
	logger     *log.Logger
}

// NewServer creates a configured C4 server.
func NewServer(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	s := &Server{
		config: cfg,
		logger: log.New(os.Stdout, "[c4-server] ", log.LstdFlags),
	}

	s.handler = s.buildRouter()
	return s
}

// buildRouter constructs the chi-style route tree with middleware.
func (s *Server) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// Health check (no auth)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /", s.handleRoot)

	// API routes (with auth + gating)
	mux.HandleFunc("POST /api/chat", s.withMiddleware(s.handleChat))
	mux.HandleFunc("POST /api/workers/spawn", s.withMiddleware(s.handleWorkerSpawn))
	mux.HandleFunc("DELETE /api/workers/{id}", s.withMiddleware(s.handleWorkerDelete))
	mux.HandleFunc("GET /api/c4/status", s.withMiddleware(s.handleC4Status))

	// Webhooks (signature verification, no JWT)
	mux.HandleFunc("POST /api/webhooks/github", s.handleGitHubWebhook)

	// Wrap with CORS
	return s.corsMiddleware(mux)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // SSE can be long
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Printf("Starting server on %s", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	s.logger.Printf("Shutting down...")
	return s.httpServer.Shutdown(ctx)
}

// Handler returns the HTTP handler (for testing).
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ActiveConnections returns the count of active connections.
func (s *Server) ActiveConnections() int64 {
	return s.active.Load()
}

// =========================================================================
// Middleware
// =========================================================================

// withMiddleware wraps a handler with auth + gating middleware.
func (s *Server) withMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Track active connections
		s.active.Add(1)
		defer s.active.Add(-1)

		// Auth middleware
		user, err := s.authenticateJWT(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
				"message": err.Error(),
			})
			return
		}

		// Plan gating middleware
		if !s.checkPlanGating(user) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "plan_restricted",
				"message": "This feature requires a paid plan. Upgrade at https://c4.dev/pricing",
			})
			return
		}

		// Store user in context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		handler(w, r.WithContext(ctx))
	}
}

type contextKey string

const userContextKey contextKey = "user"

// getUserFromContext retrieves the authenticated user from request context.
func getUserFromContext(r *http.Request) *User {
	if user, ok := r.Context().Value(userContextKey).(*User); ok {
		return user
	}
	return nil
}

// authenticateJWT validates the Supabase JWT token from Authorization header.
func (s *Server) authenticateJWT(r *http.Request) (*User, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, fmt.Errorf("missing Authorization header")
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("invalid Authorization format")
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}

	// In production, decode and verify JWT using the secret.
	// For now, use a pluggable verifier for testability.
	if s.config.JWTSecret == "" {
		return nil, fmt.Errorf("JWT authentication not configured")
	}

	user, err := verifyJWT(token, s.config.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	return user, nil
}

// JWTVerifier is pluggable for testing.
var verifyJWT = defaultVerifyJWT

// defaultVerifyJWT decodes and validates a Supabase JWT.
// In a real implementation, uses a JWT library. Here we provide
// a simple base64 decode for the test harness.
func defaultVerifyJWT(token, secret string) (*User, error) {
	// Split JWT into parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode payload (part 1) using base64url without padding
	decoded, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var user User
	if err := json.Unmarshal(decoded, &user); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}

	return &user, nil
}

// decodeBase64URL decodes a base64url-encoded string (with or without padding).
func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// checkPlanGating verifies the user's plan allows access.
func (s *Server) checkPlanGating(user *User) bool {
	if len(s.config.AllowedPlans) == 0 {
		return true // no restrictions
	}

	for _, plan := range s.config.AllowedPlans {
		if user.Plan == plan {
			return true
		}
	}

	return false
}

// corsMiddleware adds CORS headers to responses.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range s.config.CORSOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// =========================================================================
// Handlers
// =========================================================================

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "healthy",
		"service": "c4-cloud",
		"active":  s.active.Load(),
	})
}

// handleRoot returns API information.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "C4 Cloud API",
		"version": "0.1.0",
		"endpoints": map[string]string{
			"chat":    "POST /api/chat",
			"workers": "POST /api/workers/spawn",
			"webhook": "POST /api/webhooks/github",
			"status":  "GET /api/c4/status",
			"health":  "GET /health",
		},
	})
}

// ChatRequest is the request body for /api/chat.
type ChatRequest struct {
	Message        string         `json:"message"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Stream         bool           `json:"stream"`
	Context        map[string]any `json:"context,omitempty"`
}

// handleChat proxies chat messages to Claude API with SSE streaming.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "message is required",
		})
		return
	}

	user := getUserFromContext(r)

	if req.Stream {
		s.streamChatResponse(w, r, &req, user)
	} else {
		s.syncChatResponse(w, r, &req, user)
	}
}

// streamChatResponse sends SSE events for a chat response.
func (s *Server) streamChatResponse(w http.ResponseWriter, r *http.Request, req *ChatRequest, user *User) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "streaming not supported",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Send initial event
	writeSSE(w, "start", map[string]any{
		"conversation_id": req.ConversationID,
		"user_id":         user.ID,
	})
	flusher.Flush()

	// In production, proxy to Claude API.
	// For now, send a placeholder response.
	writeSSE(w, "message", map[string]any{
		"content": fmt.Sprintf("Processing: %s", req.Message),
		"done":    false,
	})
	flusher.Flush()

	writeSSE(w, "done", map[string]any{
		"done": true,
	})
	flusher.Flush()
}

// syncChatResponse sends a non-streaming response.
func (s *Server) syncChatResponse(w http.ResponseWriter, r *http.Request, req *ChatRequest, user *User) {
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": req.ConversationID,
		"message": map[string]any{
			"role":    "assistant",
			"content": fmt.Sprintf("Processing: %s", req.Message),
		},
		"done": true,
	})
}

// WorkerSpawnRequest is the request body for /api/workers/spawn.
type WorkerSpawnRequest struct {
	ProjectID string `json:"project_id"`
	Region    string `json:"region,omitempty"` // e.g., "iad", "sjc"
	Model     string `json:"model,omitempty"`  // e.g., "opus", "sonnet"
	Count     int    `json:"count,omitempty"`
}

// handleWorkerSpawn creates cloud workers on Fly.io.
func (s *Server) handleWorkerSpawn(w http.ResponseWriter, r *http.Request) {
	var req WorkerSpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if req.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "project_id is required",
		})
		return
	}

	if req.Count == 0 {
		req.Count = 1
	}

	user := getUserFromContext(r)

	// In production, call Fly.io API to spawn workers.
	// For now, return a placeholder.
	workers := make([]map[string]any, req.Count)
	for i := 0; i < req.Count; i++ {
		workers[i] = map[string]any{
			"id":         fmt.Sprintf("w-%s-%d", req.ProjectID[:8], i+1),
			"project_id": req.ProjectID,
			"region":     req.Region,
			"model":      req.Model,
			"status":     "pending",
			"created_by": user.ID,
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"workers": workers,
		"count":   req.Count,
	})
}

// handleWorkerDelete deletes a cloud worker.
func (s *Server) handleWorkerDelete(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	if workerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "worker ID is required",
		})
		return
	}

	// In production, call Fly.io API to delete the machine.
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      workerID,
		"status":  "deleted",
		"message": "Worker deletion initiated",
	})
}

// handleC4Status proxies to the C4 daemon status.
func (s *Server) handleC4Status(w http.ResponseWriter, r *http.Request) {
	// In production, proxy to the C4 daemon or read from Supabase.
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "execute",
		"tasks": map[string]any{
			"total":       0,
			"pending":     0,
			"in_progress": 0,
			"done":        0,
		},
	})
}

// =========================================================================
// GitHub Webhook Handler
// =========================================================================

// GitHubWebhookPayload represents a minimal GitHub webhook payload.
type GitHubWebhookPayload struct {
	Action     string         `json:"action"`
	Repository map[string]any `json:"repository"`
	Sender     map[string]any `json:"sender"`
}

// handleGitHubWebhook receives and validates GitHub webhooks.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	s.active.Add(1)
	defer s.active.Add(-1)

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read body",
		})
		return
	}

	// Validate signature
	if s.config.WebhookSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !validateGitHubSignature(body, signature, s.config.WebhookSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid webhook signature",
			})
			return
		}
	}

	// Parse event type
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing X-GitHub-Event header",
		})
		return
	}

	// Parse payload
	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
		return
	}

	s.logger.Printf("Received GitHub webhook: event=%s action=%s", eventType, payload.Action)

	// Acknowledge receipt
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"event":   eventType,
		"action":  payload.Action,
	})
}

// validateGitHubSignature checks the X-Hub-Signature-256 header.
func validateGitHubSignature(body []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// Signature format: sha256=hex
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sigHex := strings.TrimPrefix(signature, "sha256=")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(sigBytes, expected)
}

// =========================================================================
// Helpers
// =========================================================================

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeSSE writes a Server-Sent Event.
func writeSSE(w http.ResponseWriter, event string, data any) {
	dataBytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(dataBytes))
}

// =========================================================================
// Concurrent connection tracking for testing
// =========================================================================

// ConnectionTracker helps test concurrent connections.
type ConnectionTracker struct {
	mu      sync.Mutex
	peak    int64
	current int64
}

// Track increments current and updates peak.
func (t *ConnectionTracker) Track() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current++
	if t.current > t.peak {
		t.peak = t.current
	}
}

// Release decrements current.
func (t *ConnectionTracker) Release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current--
}

// Peak returns the peak concurrent connection count.
func (t *ConnectionTracker) Peak() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peak
}
