package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

// =========================================================================
// Presigned URL
// =========================================================================

func (s *Server) handlePresignedURL(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest[model.PresignedURLRequest](w, r, "POST")
	if !ok {
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.Method != "GET" && req.Method != "PUT" {
		writeError(w, http.StatusBadRequest, "method must be GET or PUT")
		return
	}

	url, expiresAt, err := s.storage.PresignedURL(req.Path, req.Method, req.TTLSeconds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, model.PresignedURLResponse{
		URL:       url,
		ExpiresAt: expiresAt.Format("2006-01-02T15:04:05Z"),
	})
}

// =========================================================================
// Artifacts
// =========================================================================

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	// /v1/artifacts/{job_id}/...
	path := strings.TrimPrefix(r.URL.Path, "/v1/artifacts/")
	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]

	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}

	if len(parts) > 1 {
		sub := parts[1]

		// POST /v1/artifacts/{job_id}/confirm
		if sub == "confirm" {
			s.handleArtifactConfirm(w, r, jobID)
			return
		}

		// GET /v1/artifacts/{job_id}/url/{name}
		if strings.HasPrefix(sub, "url/") {
			name := strings.TrimPrefix(sub, "url/")
			s.handleArtifactURL(w, r, jobID, name)
			return
		}

		writeError(w, http.StatusNotFound, "unknown sub-path: "+sub)
		return
	}

	// GET /v1/artifacts/{job_id} — list artifacts
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	artifacts, err := s.store.ListArtifacts(jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if artifacts == nil {
		artifacts = []model.Artifact{}
	}
	writeJSON(w, artifacts)
}

func (s *Server) handleArtifactConfirm(w http.ResponseWriter, r *http.Request, jobID string) {
	req, ok := decodeRequest[model.ArtifactConfirmRequest](w, r, "POST")
	if !ok {
		return
	}

	if req.Path == "" || req.ContentHash == "" {
		writeError(w, http.StatusBadRequest, "path and content_hash are required")
		return
	}

	// Ensure artifact record exists (create if needed)
	_, err := s.store.GetArtifact(jobID, req.Path)
	if err != nil {
		// Auto-create artifact record
		if _, err := s.store.CreateArtifact(jobID, req.Path); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	resp, err := s.store.ConfirmArtifact(jobID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, resp)
}

func (s *Server) handleArtifactURL(w http.ResponseWriter, r *http.Request, jobID, name string) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	// Find the artifact
	artifact, err := s.store.GetArtifact(jobID, name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if !artifact.Confirmed {
		writeError(w, http.StatusBadRequest, "artifact not yet confirmed")
		return
	}

	// Generate presigned download URL
	url, _, err := s.storage.PresignedURL(artifact.Path, "GET", 3600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, model.ArtifactURLResponse{
		URL: url,
	})
}

// handleUploadArtifact handles PUT upload for presigned URLs (local backend only).
func (s *Server) handleUploadArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	// Parse job_id and storage_path from request
	var req struct {
		JobID       string `json:"job_id"`
		StoragePath string `json:"storage_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.JobID == "" || req.StoragePath == "" {
		writeError(w, http.StatusBadRequest, "job_id and storage_path are required")
		return
	}

	// Create artifact record
	artifact, err := s.store.CreateArtifact(req.JobID, req.StoragePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Generate upload URL
	url, expiresAt, err := s.storage.PresignedURL(req.StoragePath, "PUT", 3600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"artifact_id": artifact.ID,
		"upload_url":  url,
		"expires_at":  expiresAt.Format("2006-01-02T15:04:05Z"),
	})
}
