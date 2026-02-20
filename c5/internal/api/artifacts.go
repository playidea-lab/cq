package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/storage"
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

// handleUploadArtifact handles POST /v1/storage/upload — legacy JSON-based upload request.
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

// handleStorageDownload serves file content for GET /v1/storage/download/{path}.
// Only works when the storage backend implements FilePathResolver (local backend).
func (s *Server) handleStorageDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	resolver, ok := s.storage.(storage.FilePathResolver)
	if !ok {
		writeError(w, http.StatusNotImplemented, "download not supported for this storage backend")
		return
	}

	storagePath := strings.TrimPrefix(r.URL.Path, "/v1/storage/download/")
	if storagePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	localPath, err := resolver.FilePath(storagePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fi, err := os.Stat(localPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if fi.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	http.ServeFile(w, r, localPath)
}

// handleStoragePut accepts PUT /v1/storage/upload/{path} — writes request body to local file.
// Only works when the storage backend implements FilePathResolver (local backend).
func (s *Server) handleStoragePut(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		methodNotAllowed(w)
		return
	}

	resolver, ok := s.storage.(storage.FilePathResolver)
	if !ok {
		writeError(w, http.StatusNotImplemented, "direct upload not supported for this storage backend")
		return
	}

	storagePath := strings.TrimPrefix(r.URL.Path, "/v1/storage/upload/")
	if storagePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	localPath, err := resolver.FilePath(storagePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "create directory: "+err.Error())
		return
	}

	f, err := os.Create(localPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create file: "+err.Error())
		return
	}
	defer f.Close()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<30) // 1GB limit
	if _, err := io.Copy(f, r.Body); err != nil {
		f.Close()
		os.Remove(localPath)
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		} else {
			writeError(w, http.StatusInternalServerError, "upload failed: "+err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}
