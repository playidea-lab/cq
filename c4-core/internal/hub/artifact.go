package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
)

// PresignedURLRequest is the payload for POST /v1/storage/presigned-url.
type PresignedURLRequest struct {
	Path        string `json:"path"`
	Method      string `json:"method"` // GET or PUT
	TTLSeconds  int    `json:"ttl_seconds,omitempty"`
	Purpose     string `json:"purpose,omitempty"`      // download, upload, log
	ContentType string `json:"content_type,omitempty"` // for PUT
}

// PresignedURLResponse from Hub.
type PresignedURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// ArtifactConfirmRequest is the payload for POST /v1/artifacts/{job_id}/confirm.
type ArtifactConfirmRequest struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ArtifactConfirmResponse from Hub.
type ArtifactConfirmResponse struct {
	ArtifactID string `json:"artifact_id"`
	Confirmed  bool   `json:"confirmed"`
}

// ArtifactURLResponse from GET /v1/artifacts/{job_id}/url/{name}.
type ArtifactURLResponse struct {
	URL string `json:"url"`
}

// =========================================================================
// Presigned URL
// =========================================================================

// GetPresignedURL gets a presigned URL for storage access.
func (c *Client) GetPresignedURL(path, method string, ttl int) (*PresignedURLResponse, error) {
	req := PresignedURLRequest{
		Path:       path,
		Method:     method,
		TTLSeconds: ttl,
	}
	var resp PresignedURLResponse
	if err := c.post("/storage/presigned-url", req, &resp); err != nil {
		return nil, fmt.Errorf("get presigned url: %w", err)
	}
	return &resp, nil
}

// =========================================================================
// Artifact Upload
// =========================================================================

// UploadArtifact uploads a local file to Hub storage and confirms it.
// Steps: get presigned PUT URL → upload file → confirm with hash.
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

	// 3. Get presigned PUT URL
	presigned, err := c.GetPresignedURL(storagePath, "PUT", 3600)
	if err != nil {
		return nil, err
	}

	// 4. Upload via PUT
	if err := c.uploadToPut(presigned.URL, localPath, fi.Size()); err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}

	// 5. Confirm
	confirm, err := c.ConfirmArtifact(jobID, storagePath, "sha256:"+hash, fi.Size())
	if err != nil {
		return nil, err
	}

	return confirm, nil
}

// uploadToPut uploads a file to a presigned PUT URL.
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

// ConfirmArtifact confirms an artifact upload with the Hub.
func (c *Client) ConfirmArtifact(jobID, path, contentHash string, sizeBytes int64) (*ArtifactConfirmResponse, error) {
	req := ArtifactConfirmRequest{
		Path:        path,
		ContentHash: contentHash,
		SizeBytes:   sizeBytes,
	}
	var resp ArtifactConfirmResponse
	if err := c.post(fmt.Sprintf("/artifacts/%s/confirm", jobID), req, &resp); err != nil {
		return nil, fmt.Errorf("confirm artifact: %w", err)
	}
	return &resp, nil
}

// =========================================================================
// Artifact Download
// =========================================================================

// GetArtifactURL gets a presigned download URL for an artifact.
func (c *Client) GetArtifactURL(jobID, name string) (string, error) {
	var resp ArtifactURLResponse
	if err := c.get(fmt.Sprintf("/artifacts/%s/url/%s", jobID, name), &resp); err != nil {
		return "", fmt.Errorf("get artifact url: %w", err)
	}
	return resp.URL, nil
}

// DownloadArtifact downloads an artifact to a local file.
func (c *Client) DownloadArtifact(jobID, name, destPath string) error {
	url, err := c.GetArtifactURL(jobID, name)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Get(url)
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
