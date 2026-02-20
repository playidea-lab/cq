// Package storage provides artifact storage backends for C5.
//
// It supports Supabase Storage (presigned URLs) with a local filesystem fallback
// when Supabase credentials are not configured.
package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultBucket = "c5-artifacts"
	defaultTTL    = 3600 // 1 hour
)

// Backend provides artifact storage operations.
type Backend interface {
	// PresignedURL generates a presigned URL for upload (PUT) or download (GET).
	PresignedURL(path, method string, ttlSeconds int) (url string, expiresAt time.Time, err error)
}

// BucketManager is an optional interface implemented by backends that support
// bucket/directory initialization. Separate from Backend to avoid breaking
// existing implementations.
type BucketManager interface {
	// EnsureBucket verifies the storage bucket/directory exists, creating it if absent.
	EnsureBucket() error
}

// SupabaseBackend uses Supabase Storage for artifact storage.
type SupabaseBackend struct {
	supabaseURL string
	apiKey      string
	bucket      string
	httpClient  *http.Client
}

// NewSupabase creates a new Supabase storage backend.
func NewSupabase(supabaseURL, apiKey string) *SupabaseBackend {
	return &SupabaseBackend{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		apiKey:      apiKey,
		bucket:      defaultBucket,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// PresignedURL generates a Supabase Storage presigned URL.
func (s *SupabaseBackend) PresignedURL(path, method string, ttlSeconds int) (string, time.Time, error) {
	if ttlSeconds == 0 {
		ttlSeconds = defaultTTL
	}
	// Sanitize path to prevent traversal attacks.
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", time.Time{}, fmt.Errorf("invalid storage path: %s", path)
	}
	path = cleaned

	expiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)

	if method == "PUT" {
		// Create a signed upload URL so the worker can PUT without auth headers.
		// Supabase Storage: POST /storage/v1/object/upload/sign/{bucket}/{path}
		signURL := fmt.Sprintf("%s/storage/v1/object/upload/sign/%s/%s", s.supabaseURL, s.bucket, path)
		body := fmt.Sprintf(`{"expiresIn":%d}`, ttlSeconds)

		req, err := http.NewRequest("POST", signURL, strings.NewReader(body))
		if err != nil {
			return "", time.Time{}, fmt.Errorf("create upload sign request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apikey", s.apiKey)
		req.Header.Set("Authorization", "Bearer "+s.apiKey)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("upload sign request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			return "", time.Time{}, fmt.Errorf("upload sign failed (status %d): %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", time.Time{}, fmt.Errorf("decode upload sign response: %w", err)
		}

		// URL is relative (e.g. "/object/upload/sign/{bucket}/{path}?token=xxx"), prepend base.
		fullURL := s.supabaseURL + "/storage/v1" + result.URL
		return fullURL, expiresAt, nil
	}

	// For downloads, create a signed URL
	// Supabase Storage: POST /storage/v1/object/sign/{bucket}/{path}
	signURL := fmt.Sprintf("%s/storage/v1/object/sign/%s/%s", s.supabaseURL, s.bucket, path)
	body := fmt.Sprintf(`{"expiresIn":%d}`, ttlSeconds)

	req, err := http.NewRequest("POST", signURL, strings.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", time.Time{}, fmt.Errorf("sign failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode sign response: %w", err)
	}

	// signedURL is relative, prepend base
	fullURL := s.supabaseURL + "/storage/v1" + result.SignedURL
	return fullURL, expiresAt, nil
}

// EnsureBucket checks whether the c5-artifacts bucket exists in Supabase Storage.
// If not found (HTTP 404), it creates the bucket. Non-fatal by design; callers
// should log and continue if this returns an error.
func (s *SupabaseBackend) EnsureBucket() error {
	checkURL := fmt.Sprintf("%s/storage/v1/bucket/%s", s.supabaseURL, s.bucket)

	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("ensure bucket: create check request: %w", err)
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ensure bucket: check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	// Supabase may return 400 with "Bucket not found" instead of 404.
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		bodyStr := string(body)
		if !strings.Contains(strings.ToLower(bodyStr), "not found") {
			return fmt.Errorf("ensure bucket: unexpected status %d: %s", resp.StatusCode, bodyStr)
		}
	}

	// Bucket not found — create it.
	createURL := fmt.Sprintf("%s/storage/v1/bucket", s.supabaseURL)
	payload := fmt.Sprintf(`{"id":%q,"public":false}`, s.bucket)

	createReq, err := http.NewRequest("POST", createURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("ensure bucket: create bucket request: %w", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("apikey", s.apiKey)
	createReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	createResp, err := s.httpClient.Do(createReq)
	if err != nil {
		return fmt.Errorf("ensure bucket: create request: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(createResp.Body, 1<<16))
		return fmt.Errorf("ensure bucket: create failed (status %d): %s", createResp.StatusCode, string(body))
	}
	return nil
}

// LocalBackend stores artifacts on the local filesystem.
// Used as fallback when Supabase is not configured.
type LocalBackend struct {
	baseDir string
	baseURL string // server's own URL for download links
}

// NewLocal creates a local filesystem storage backend.
func NewLocal(baseDir, baseURL string) *LocalBackend {
	os.MkdirAll(baseDir, 0755)
	return &LocalBackend{
		baseDir: baseDir,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// PresignedURL returns a local file path for uploads or a download URL.
func (l *LocalBackend) PresignedURL(path, method string, ttlSeconds int) (string, time.Time, error) {
	if ttlSeconds == 0 {
		ttlSeconds = defaultTTL
	}
	expiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)

	// Sanitize path to prevent directory traversal
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", time.Time{}, fmt.Errorf("invalid storage path: %s", path)
	}

	fullPath := filepath.Join(l.baseDir, cleaned)
	absBase, _ := filepath.Abs(l.baseDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", time.Time{}, fmt.Errorf("path escapes base directory: %s", path)
	}

	os.MkdirAll(filepath.Dir(fullPath), 0755)

	if method == "PUT" {
		// Return the local file path — client must handle local upload
		return "file://" + fullPath, expiresAt, nil
	}

	// Return a download URL via the C5 server
	url := fmt.Sprintf("%s/v1/storage/download/%s", l.baseURL, cleaned)
	return url, expiresAt, nil
}

// FilePath returns the local file path for a storage path.
// Returns an error if the path escapes the base directory.
func (l *LocalBackend) FilePath(storagePath string) (string, error) {
	cleaned := filepath.Clean(storagePath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid storage path: %s", storagePath)
	}
	fullPath := filepath.Join(l.baseDir, cleaned)
	absBase, _ := filepath.Abs(l.baseDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", fmt.Errorf("path escapes base directory: %s", storagePath)
	}
	return fullPath, nil
}

// EnsureBucket ensures the local base directory exists.
func (l *LocalBackend) EnsureBucket() error {
	return os.MkdirAll(l.baseDir, 0755)
}

// NewBackend creates the appropriate storage backend based on environment.
func NewBackend(serverURL string) Backend {
	supabaseURL := os.Getenv("C5_SUPABASE_URL")
	supabaseKey := os.Getenv("C5_SUPABASE_KEY")

	if supabaseURL != "" && supabaseKey != "" {
		return NewSupabase(supabaseURL, supabaseKey)
	}

	// Fallback to local storage
	dataDir := os.Getenv("C5_DATA_DIR")
	if dataDir == "" {
		dataDir = "."
	}
	return NewLocal(filepath.Join(dataDir, "artifacts"), serverURL)
}
