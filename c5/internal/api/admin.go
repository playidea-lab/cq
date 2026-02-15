package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

func (s *Server) handleAdminAPIKeys(w http.ResponseWriter, r *http.Request) {
	// Admin endpoints require master key
	if !isMasterFromContext(r) {
		writeError(w, http.StatusForbidden, "admin endpoints require master API key")
		return
	}

	switch r.Method {
	case "POST":
		s.handleAdminCreateAPIKey(w, r)
	case "GET":
		s.handleAdminListAPIKeys(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	rawKey, err := s.store.CreateAPIKey(req.ProjectID, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	keyHash := model.SHA256Hex(rawKey)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.CreateAPIKeyResponse{
		Key:       rawKey,
		KeyHash:   keyHash,
		ProjectID: req.ProjectID,
	})
}

func (s *Server) handleAdminListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []model.APIKeyInfo{}
	}
	writeJSON(w, keys)
}

func (s *Server) handleAdminAPIKeyByHash(w http.ResponseWriter, r *http.Request) {
	if !isMasterFromContext(r) {
		writeError(w, http.StatusForbidden, "admin endpoints require master API key")
		return
	}

	hash := strings.TrimPrefix(r.URL.Path, "/v1/admin/api-keys/")
	if hash == "" {
		writeError(w, http.StatusBadRequest, "key hash required")
		return
	}

	if r.Method != "DELETE" {
		methodNotAllowed(w)
		return
	}

	if err := s.store.DeleteAPIKey(hash); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"deleted":  true,
		"key_hash": hash,
	})
}
