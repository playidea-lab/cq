// Package drive implements a Supabase Storage client for C4 Drive file operations.
//
// It provides upload, download, list, delete, and mkdir operations using
// the Supabase Storage REST API for file content and PostgREST for metadata.
package drive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// FileInfo represents metadata about a file in the drive.
type FileInfo struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	StoragePath string          `json:"storage_path"`
	SizeBytes   int64           `json:"size_bytes"`
	ContentHash string          `json:"content_hash"`
	ContentType string          `json:"content_type"`
	IsFolder    bool            `json:"is_folder"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// tokenProvider abstracts JWT token management for Supabase auth.
// This local interface avoids importing the cloud package.
type tokenProvider interface {
	Token() string
	Refresh() (string, error)
}

// Client provides access to C4 Drive (Supabase Storage + PostgREST metadata).
type Client struct {
	supabaseURL string // e.g. https://xxx.supabase.co
	apiKey      string // anon key
	tp          tokenProvider
	projectID   string // cloud project ID (UUID or name)
	httpClient  *http.Client
}

const driveMaxRetries = 2

// NewClient creates a new Drive client.
func NewClient(supabaseURL, apiKey string, tp tokenProvider, projectID string) *Client {
	return &Client{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		apiKey:      apiKey,
		tp:          tp,
		projectID:   projectID,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Upload uploads a local file to the drive at the given drive path.
// The storage path is derived from the content hash to enable deduplication.
func (c *Client) Upload(localPath, drivePath string, metadata json.RawMessage) (*FileInfo, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// Compute SHA256
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("hash file: %w", err)
	}
	contentHash := "sha256:" + hex.EncodeToString(h.Sum(nil))

	// Reset file for upload
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek file: %w", err)
	}

	// Storage path: {projectID}/{hash_prefix}/{filename}
	hashHex := hex.EncodeToString(h.Sum(nil))
	storagePath := c.projectID + "/" + hashHex[:8] + "/" + filepath.Base(localPath)

	// Upload to Supabase Storage
	uploadURL := c.supabaseURL + "/storage/v1/object/c4-drive/" + storagePath
	req, err := http.NewRequest("POST", uploadURL, f)
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	req.ContentLength = fi.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	c.setHeaders(req)
	// Supabase Storage uses x-upsert for overwrite
	req.Header.Set("x-upsert", "true")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Normalize drive path
	drivePath = normalizePath(drivePath)

	// Insert metadata via PostgREST (upsert on path)
	meta := map[string]any{
		"project_id":   c.projectID,
		"name":         path.Base(drivePath),
		"path":         drivePath,
		"storage_path": storagePath,
		"size_bytes":   fi.Size(),
		"content_hash": contentHash,
		"content_type": "application/octet-stream",
		"is_folder":    false,
	}
	if len(metadata) > 0 {
		meta["metadata"] = metadata
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	metaURL := c.supabaseURL + "/rest/v1/c4_drive_files"
	metaReq, err := http.NewRequest("POST", metaURL, strings.NewReader(string(metaJSON)))
	if err != nil {
		return nil, fmt.Errorf("create metadata request: %w", err)
	}
	c.setHeaders(metaReq)
	metaReq.Header.Set("Prefer", "return=representation,resolution=merge-duplicates")
	metaReq.Header.Set("Content-Type", "application/json")

	metaResp, err := c.doWithRetry(metaReq)
	if err != nil {
		return nil, fmt.Errorf("metadata request: %w", err)
	}
	defer metaResp.Body.Close()

	if metaResp.StatusCode >= 400 {
		body, _ := io.ReadAll(metaResp.Body)
		return nil, fmt.Errorf("metadata insert failed (HTTP %d): %s", metaResp.StatusCode, string(body))
	}

	var rows []FileInfo
	if err := json.NewDecoder(metaResp.Body).Decode(&rows); err != nil {
		// Return basic info even if decode fails
		return &FileInfo{
			Name:        path.Base(drivePath),
			Path:        drivePath,
			StoragePath: storagePath,
			SizeBytes:   fi.Size(),
			ContentHash: contentHash,
		}, nil
	}
	if len(rows) > 0 {
		return &rows[0], nil
	}

	return &FileInfo{
		Name:        path.Base(drivePath),
		Path:        drivePath,
		StoragePath: storagePath,
		SizeBytes:   fi.Size(),
		ContentHash: contentHash,
	}, nil
}

// Download downloads a file from the drive to a local path.
func (c *Client) Download(drivePath, destPath string) error {
	drivePath = normalizePath(drivePath)

	// Look up storage path from metadata
	info, err := c.Info(drivePath)
	if err != nil {
		return fmt.Errorf("lookup file: %w", err)
	}
	if info.IsFolder {
		return fmt.Errorf("%s is a folder", drivePath)
	}

	// Download from Supabase Storage
	downloadURL := c.supabaseURL + "/storage/v1/object/c4-drive/" + info.StoragePath
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// List returns files and folders in the given folder path.
// Use "/" or "" for the root folder.
func (c *Client) List(folder string) ([]FileInfo, error) {
	folder = normalizePath(folder)

	// Query immediate children only (server-side depth filtering)
	filter := "project_id=eq." + url.QueryEscape(c.projectID)
	if folder == "/" {
		// Root: exclude nested paths (only top-level entries)
		filter += "&path=not.like." + url.QueryEscape("/*/*")
	} else {
		// Non-root: match folder/* but exclude folder/*/*
		filter += fmt.Sprintf("&and=(path.like.%s,path.not.like.%s)",
			url.QueryEscape(folder+"/*"), url.QueryEscape(folder+"/*/*"))
	}
	filter += "&order=is_folder.desc,name.asc"

	listURL := c.supabaseURL + "/rest/v1/c4_drive_files?" + filter
	req, err := http.NewRequest("GET", listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create list request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var files []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}

	return files, nil
}

// Delete removes a file or folder from the drive.
func (c *Client) Delete(drivePath string) error {
	drivePath = normalizePath(drivePath)

	// Look up metadata to get storage path
	info, err := c.Info(drivePath)
	if err != nil {
		return fmt.Errorf("lookup file: %w", err)
	}

	// Delete from Supabase Storage (only for non-folders)
	if !info.IsFolder && info.StoragePath != "" {
		deleteURL := c.supabaseURL + "/storage/v1/object/c4-drive/" + info.StoragePath
		req, err := http.NewRequest("DELETE", deleteURL, nil)
		if err != nil {
			return fmt.Errorf("create storage delete request: %w", err)
		}
		c.setHeaders(req)

		resp, err := c.doWithRetry(req)
		if err != nil {
			return fmt.Errorf("storage delete request: %w", err)
		}
		resp.Body.Close()
	}

	// Delete metadata row
	metaURL := c.supabaseURL + "/rest/v1/c4_drive_files?project_id=eq." + url.QueryEscape(c.projectID) + "&path=eq." + url.QueryEscape(drivePath)
	req, err := http.NewRequest("DELETE", metaURL, nil)
	if err != nil {
		return fmt.Errorf("create metadata delete request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("metadata delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("metadata delete failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// Mkdir creates a folder entry in the drive.
func (c *Client) Mkdir(folderPath string, metadata json.RawMessage) (*FileInfo, error) {
	folderPath = normalizePath(folderPath)

	meta := map[string]any{
		"project_id":   c.projectID,
		"name":         path.Base(folderPath),
		"path":         folderPath,
		"storage_path": "",
		"size_bytes":   0,
		"content_hash": "",
		"content_type": "inode/directory",
		"is_folder":    true,
	}
	if len(metadata) > 0 {
		meta["metadata"] = metadata
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	mkdirURL := c.supabaseURL + "/rest/v1/c4_drive_files"
	req, err := http.NewRequest("POST", mkdirURL, strings.NewReader(string(metaJSON)))
	if err != nil {
		return nil, fmt.Errorf("create mkdir request: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Prefer", "return=representation,resolution=merge-duplicates")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("mkdir request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mkdir failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var rows []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil || len(rows) == 0 {
		return &FileInfo{
			Name:     path.Base(folderPath),
			Path:     folderPath,
			IsFolder: true,
		}, nil
	}

	return &rows[0], nil
}

// Info returns metadata about a file or folder at the given path.
func (c *Client) Info(drivePath string) (*FileInfo, error) {
	drivePath = normalizePath(drivePath)

	infoURL := c.supabaseURL + "/rest/v1/c4_drive_files?project_id=eq." + url.QueryEscape(c.projectID) + "&path=eq." + url.QueryEscape(drivePath)
	req, err := http.NewRequest("GET", infoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create info request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("info failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var rows []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode info: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("file not found: %s", drivePath)
	}

	return &rows[0], nil
}

// setHeaders adds standard Supabase headers to the request.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.tp.Token())
}

// doWithRetry executes an HTTP request with retry on 5xx/network errors.
// 401 is NOT retried here — callers handle token refresh separately.
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := range driveMaxRetries {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
			c.setHeaders(req) // refresh token on retry
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// normalizePath ensures a consistent path format: leading /, no trailing /.
func normalizePath(p string) string {
	p = path.Clean("/" + p)
	if p == "." {
		return "/"
	}
	return p
}
