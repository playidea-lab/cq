package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const hubBucket = "c4-drive"

// PresignedURLRequest is the payload for requesting a presigned storage URL.
type PresignedURLRequest struct {
	Path        string `json:"path"`
	Method      string `json:"method"` // GET or PUT
	TTLSeconds  int    `json:"ttl_seconds,omitempty"`
	Purpose     string `json:"purpose,omitempty"`      // download, upload, log
	ContentType string `json:"content_type,omitempty"` // for PUT
}

// PresignedURLResponse from storage.
type PresignedURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// ArtifactConfirmRequest is the payload for confirming an artifact upload.
type ArtifactConfirmRequest struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ArtifactConfirmResponse from confirming an artifact.
type ArtifactConfirmResponse struct {
	ArtifactID string `json:"artifact_id"`
	Confirmed  bool   `json:"confirmed"`
}

// ArtifactURLResponse from GET artifact URL.
type ArtifactURLResponse struct {
	URL string `json:"url"`
}

// =========================================================================
// Presigned URL — Supabase Storage
// =========================================================================

// GetPresignedURL generates a Supabase Storage presigned URL for upload or download.
// For PUT: uses the upload endpoint. For GET: uses the download endpoint.
func (c *Client) GetPresignedURL(storagePath, method string, ttl int) (*PresignedURLResponse, error) {
	base := c.supabaseURL
	if base == "" {
		base = c.baseURL
	}

	if strings.ToUpper(method) == "PUT" {
		// Supabase Storage upload: POST /storage/v1/object/{bucket}/{path}
		// Return the direct upload URL — no presign needed for Supabase.
		uploadURL := base + "/storage/v1/object/" + hubBucket + "/" + storagePath
		return &PresignedURLResponse{URL: uploadURL}, nil
	}

	// GET: Supabase Storage signed URL via POST /storage/v1/object/sign/{bucket}/{path}
	bodyData, err := json.Marshal(map[string]any{"expiresIn": ttl})
	if err != nil {
		return nil, fmt.Errorf("get presigned url: marshal: %w", err)
	}
	signURL := base + "/storage/v1/object/sign/" + hubBucket + "/" + storagePath
	req, err := http.NewRequest("POST", signURL, strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, fmt.Errorf("get presigned url: %w", err)
	}
	c.setSupabaseHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("get presigned url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get presigned url: %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("get presigned url: decode: %w", err)
	}
	return &PresignedURLResponse{URL: base + result.SignedURL}, nil
}

// =========================================================================
// Artifact Upload
// =========================================================================

// UploadArtifact uploads a local file to Supabase Storage and records it in hub_artifacts.
func (c *Client) UploadArtifact(jobID, storagePath, localPath string) (*ArtifactConfirmResponse, error) {
	// 1. Get file info
	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// 2. Compute SHA256
	hash, err := sha256File(localPath)
	if err != nil {
		return nil, fmt.Errorf("hash file: %w", err)
	}

	// 3. Get upload URL (Supabase direct upload)
	presigned, err := c.GetPresignedURL(storagePath, "PUT", 3600)
	if err != nil {
		return nil, err
	}

	// 4. Upload via PUT
	if err := c.uploadToPut(presigned.URL, localPath, fi.Size()); err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}

	// 5. Confirm (record in hub_artifacts)
	confirm, err := c.ConfirmArtifact(jobID, storagePath, "sha256:"+hash, fi.Size())
	if err != nil {
		return nil, err
	}

	return confirm, nil
}

// uploadToPut uploads a file to a PUT URL (Supabase Storage direct upload).
func (c *Client) uploadToPut(url, localPath string, size int64) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	req, err := http.NewRequest("PUT", url, f)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	// Add Supabase auth headers for storage upload
	c.setSupabaseHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PUT upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT upload: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

// ConfirmArtifact records an artifact in hub_artifacts via PostgREST.
func (c *Client) ConfirmArtifact(jobID, path, contentHash string, sizeBytes int64) (*ArtifactConfirmResponse, error) {
	artifactID := newID()
	row := map[string]any{
		"id":           artifactID,
		"job_id":       jobID,
		"path":         path,
		"content_hash": contentHash,
		"size_bytes":   sizeBytes,
		"confirmed":    true,
	}
	var rows []struct {
		ID        string `json:"id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := c.supabasePost("/rest/v1/hub_artifacts", row, &rows); err != nil {
		return nil, fmt.Errorf("confirm artifact: %w", err)
	}
	id := artifactID
	confirmed := true
	if len(rows) > 0 {
		id = rows[0].ID
		confirmed = rows[0].Confirmed
	}
	return &ArtifactConfirmResponse{ArtifactID: id, Confirmed: confirmed}, nil
}

// =========================================================================
// Artifact Download
// =========================================================================

// GetArtifactURL returns a Supabase Storage download URL for an artifact.
func (c *Client) GetArtifactURL(jobID, name string) (string, error) {
	// Lookup artifact path in hub_artifacts
	var rows []struct {
		Path string `json:"path"`
	}
	path := fmt.Sprintf("/rest/v1/hub_artifacts?job_id=eq.%s&path=like.*%s*&limit=1", jobID, filepath.Base(name))
	if err := c.supabaseGet(path, &rows); err != nil {
		return "", fmt.Errorf("get artifact url: %w", err)
	}
	storagePath := name
	if len(rows) > 0 {
		storagePath = rows[0].Path
	}

	// Build Supabase Storage public/authenticated URL
	base := c.supabaseURL
	if base == "" {
		base = c.baseURL
	}
	url := base + "/storage/v1/object/" + hubBucket + "/" + storagePath
	return url, nil
}

// DownloadArtifact downloads an artifact from Supabase Storage to a local file.
func (c *Client) DownloadArtifact(jobID, name, destPath string) error {
	url, err := c.GetArtifactURL(jobID, name)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("download: create request: %w", err)
	}
	c.setSupabaseHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download: %d %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// =========================================================================
// Helpers
// =========================================================================

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
