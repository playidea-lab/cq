package api

import (
	"encoding/json"
	"net/http"
)

// decodeRequest checks that r.Method matches method, decodes the JSON body into T,
// and writes the appropriate error response on failure.
// Returns (nil, false) on any error so callers can return immediately.
func decodeRequest[T any](w http.ResponseWriter, r *http.Request, method string) (*T, bool) {
	if r.Method != method {
		methodNotAllowed(w)
		return nil, false
	}
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return nil, false
	}
	return &req, true
}
