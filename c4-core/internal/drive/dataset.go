package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ManifestEntry represents one file in a dataset version manifest.
type ManifestEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// DatasetUploadResult holds the result of a dataset upload operation.
type DatasetUploadResult struct {
	Name           string `json:"name"`
	VersionHash    string `json:"version_hash"`
	FilesUploaded  int    `json:"files_uploaded"`
	FilesSkipped   int    `json:"files_skipped"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
	Changed        bool   `json:"changed"`
}

// DatasetPullResult holds the result of a dataset pull operation.
type DatasetPullResult struct {
	Name            string `json:"name"`
	VersionHash     string `json:"version_hash"`
	FilesDownloaded int    `json:"files_downloaded"`
	FilesSkipped    int    `json:"files_skipped"`
	Dest            string `json:"dest"`
}

// DatasetVersion represents a version entry in the c4_datasets table.
type DatasetVersion struct {
	Name           string    `json:"name"`
	VersionHash    string    `json:"version_hash"`
	TotalSizeBytes int64     `json:"total_size_bytes"`
	FileCount      int       `json:"file_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// DatasetClient provides dataset upload/pull/list over Supabase.
type DatasetClient struct {
	client *Client
}

// NewDatasetClient creates a DatasetClient wrapping an existing drive Client.
func NewDatasetClient(client *Client) *DatasetClient {
	return &DatasetClient{client: client}
}

// Upload walks localPath, computes a manifest, and uploads changed files to
// Supabase Storage using content-addressed storage (CAS).
// If the manifest hash matches the latest stored version, Changed=false is returned.
func (dc *DatasetClient) Upload(ctx context.Context, localPath, name, extraIgnore string) (*DatasetUploadResult, error) {
	entries, err := WalkDir(localPath, extraIgnore)
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}

	// Compute SHA256 for each file in parallel (up to 4 concurrent).
	type hashResult struct {
		idx  int
		hash string
		err  error
	}

	results := make([]hashResult, len(entries))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		go func(idx int, absPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h, herr := hashFile(absPath)
			results[idx] = hashResult{idx: idx, hash: h, err: herr}
		}(i, e.Path)
	}
	wg.Wait()

	// Build manifest, accumulate total size.
	manifest := make([]ManifestEntry, len(entries))
	var totalSize int64
	for i, e := range entries {
		if results[i].err != nil {
			return nil, fmt.Errorf("hash %s: %w", e.RelPath, results[i].err)
		}
		manifest[i] = ManifestEntry{
			Path: filepath.ToSlash(e.RelPath),
			Hash: results[i].hash,
			Size: e.Size,
		}
		totalSize += e.Size
	}
	// Sort by path for deterministic manifest JSON.
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })

	// Compute version hash from manifest JSON.
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	versionHash := manifestVersionHash(manifestJSON)

	// Check existing latest version.
	existing, err := dc.latestVersion(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.versionHash == versionHash {
		return &DatasetUploadResult{
			Name:           name,
			VersionHash:    versionHash,
			FilesSkipped:   len(manifest),
			TotalSizeBytes: totalSize,
			Changed:        false,
		}, nil
	}

	// Upload each file to CAS storage: {projectID}/cas/{hash[:2]}/{hash}
	type uploadResult struct {
		skipped bool
		err     error
	}
	uploadResults := make([]uploadResult, len(entries))
	for i, e := range entries {
		wg.Add(1)
		go func(idx int, entry WalkEntry, hash string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			skipped, uerr := dc.uploadCAS(ctx, entry.Path, hash)
			uploadResults[idx] = uploadResult{skipped: skipped, err: uerr}
		}(i, e, manifest[i].Hash)
	}
	wg.Wait()

	uploaded, skipped := 0, 0
	for i, r := range uploadResults {
		if r.err != nil {
			return nil, fmt.Errorf("upload %s: %w", manifest[i].Path, r.err)
		}
		if r.skipped {
			skipped++
		} else {
			uploaded++
		}
	}

	// INSERT dataset version row.
	if err := dc.insertVersion(ctx, name, versionHash, manifest, manifestJSON, totalSize); err != nil {
		return nil, err
	}

	return &DatasetUploadResult{
		Name:           name,
		VersionHash:    versionHash,
		FilesUploaded:  uploaded,
		FilesSkipped:   skipped,
		TotalSizeBytes: totalSize,
		Changed:        true,
	}, nil
}

// Pull downloads a dataset version into dest directory, skipping files whose
// local SHA256 already matches the manifest entry.
func (dc *DatasetClient) Pull(ctx context.Context, name, dest, version string) (*DatasetPullResult, error) {
	row, err := dc.queryVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("dataset %q version %q not found", name, version)
	}

	var manifest []ManifestEntry
	if err := json.Unmarshal(row.manifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	type dlResult struct {
		skipped bool
		err     error
	}
	dlResults := make([]dlResult, len(manifest))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, entry := range manifest {
		wg.Add(1)
		go func(idx int, me ManifestEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			localPath := filepath.Join(dest, filepath.FromSlash(me.Path))
			// Check local hash.
			if existing, herr := hashFile(localPath); herr == nil && existing == me.Hash {
				dlResults[idx] = dlResult{skipped: true}
				return
			}
			// Download from CAS.
			dlErr := dc.downloadCAS(ctx, me.Hash, localPath)
			dlResults[idx] = dlResult{err: dlErr}
		}(i, entry)
	}
	wg.Wait()

	downloaded, skipped := 0, 0
	for i, r := range dlResults {
		if r.err != nil {
			return nil, fmt.Errorf("download %s: %w", manifest[i].Path, r.err)
		}
		if r.skipped {
			skipped++
		} else {
			downloaded++
		}
	}

	return &DatasetPullResult{
		Name:            name,
		VersionHash:     row.versionHash,
		FilesDownloaded: downloaded,
		FilesSkipped:    skipped,
		Dest:            dest,
	}, nil
}

// List returns all versions of datasets in the project, optionally filtered by name.
func (dc *DatasetClient) List(ctx context.Context, name string) ([]DatasetVersion, error) {
	c := dc.client
	filter := "project_id=eq." + url.QueryEscape(c.projectID)
	if name != "" {
		filter += "&name=eq." + url.QueryEscape(name)
	}
	filter += "&order=created_at.desc"

	listURL := c.supabaseURL + "/rest/v1/c4_datasets?" + filter
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create list request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list datasets (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var rows []struct {
		Name           string    `json:"name"`
		VersionHash    string    `json:"version_hash"`
		TotalSizeBytes int64     `json:"total_size_bytes"`
		FileCount      int       `json:"file_count"`
		CreatedAt      time.Time `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}

	versions := make([]DatasetVersion, len(rows))
	for i, r := range rows {
		versions[i] = DatasetVersion{
			Name:           r.Name,
			VersionHash:    r.VersionHash,
			TotalSizeBytes: r.TotalSizeBytes,
			FileCount:      r.FileCount,
			CreatedAt:      r.CreatedAt,
		}
	}
	return versions, nil
}

// --- internal helpers ---

// hashFile computes the SHA256 hex digest of a file's contents.
func hashFile(path string) (string, error) {
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

// manifestVersionHash returns the first 16 hex chars of SHA256(manifestJSON).
func manifestVersionHash(manifestJSON []byte) string {
	h := sha256.Sum256(manifestJSON)
	return hex.EncodeToString(h[:])[:16]
}

// casStoragePath returns the Storage path for a given file hash.
func (dc *DatasetClient) casStoragePath(hash string) string {
	return dc.client.projectID + "/cas/" + hash[:2] + "/" + hash
}

// uploadCAS uploads a file to CAS storage. Returns (true, nil) if already exists.
func (dc *DatasetClient) uploadCAS(ctx context.Context, localPath, hash string) (skipped bool, err error) {
	c := dc.client
	storagePath := dc.casStoragePath(hash)
	objectURL := c.supabaseURL + "/storage/v1/object/" + c.bucketName + "/" + storagePath

	// HEAD check: skip if already exists.
	headReq, err := http.NewRequestWithContext(ctx, "HEAD", objectURL, nil)
	if err != nil {
		return false, fmt.Errorf("head request: %w", err)
	}
	c.setHeaders(headReq)
	headResp, err := c.httpClient.Do(headReq)
	if err == nil {
		headResp.Body.Close()
		if headResp.StatusCode == http.StatusOK {
			return true, nil
		}
	}

	f, err := os.Open(localPath)
	if err != nil {
		return false, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat: %w", err)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return false, fmt.Errorf("read: %w", err)
	}

	uploadReq, err := http.NewRequestWithContext(ctx, "POST", objectURL, bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("upload request: %w", err)
	}
	uploadReq.ContentLength = fi.Size()
	uploadReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	c.setHeaders(uploadReq)

	uploadResp, err := c.doWithRetry(uploadReq)
	if err != nil {
		return false, fmt.Errorf("upload: %w", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode >= 400 && uploadResp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(uploadResp.Body)
		return false, fmt.Errorf("upload (HTTP %d): %s", uploadResp.StatusCode, string(body))
	}
	return false, nil
}

// downloadCAS downloads a CAS object to localPath, creating parent dirs as needed.
func (dc *DatasetClient) downloadCAS(ctx context.Context, hash, localPath string) error {
	c := dc.client
	storagePath := dc.casStoragePath(hash)
	objectURL := c.supabaseURL + "/storage/v1/object/" + c.bucketName + "/" + storagePath

	req, err := http.NewRequestWithContext(ctx, "GET", objectURL, nil)
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
		return fmt.Errorf("download (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".dataset-dl-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// versionRow is used internally to carry query results.
type versionRow struct {
	versionHash  string
	manifestJSON []byte
}

// latestVersion queries c4_datasets for the most recent version of name.
// Returns (nil, nil) if no version exists.
func (dc *DatasetClient) latestVersion(ctx context.Context, name string) (*versionRow, error) {
	rows, err := dc.queryVersions(ctx, name, "", 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// queryVersion fetches the latest matching version for (name, version prefix).
// version="" means latest; otherwise version_hash LIKE 'version%'.
func (dc *DatasetClient) queryVersion(ctx context.Context, name, version string) (*versionRow, error) {
	rows, err := dc.queryVersions(ctx, name, version, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (dc *DatasetClient) queryVersions(ctx context.Context, name, version string, limit int) ([]versionRow, error) {
	c := dc.client
	filter := "project_id=eq." + url.QueryEscape(c.projectID) +
		"&name=eq." + url.QueryEscape(name)
	if version != "" {
		filter += "&version_hash=like." + url.QueryEscape(version+"%")
	}
	filter += fmt.Sprintf("&order=created_at.desc&limit=%d", limit)

	queryURL := c.supabaseURL + "/rest/v1/c4_datasets?" + filter
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create query request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("query versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query versions (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		VersionHash string          `json:"version_hash"`
		Manifest    json.RawMessage `json:"manifest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode versions: %w", err)
	}

	rows := make([]versionRow, len(raw))
	for i, r := range raw {
		rows[i] = versionRow{
			versionHash:  r.VersionHash,
			manifestJSON: []byte(r.Manifest),
		}
	}
	return rows, nil
}

// insertVersion inserts a new c4_datasets row. 409 (ON CONFLICT DO NOTHING) is accepted.
func (dc *DatasetClient) insertVersion(ctx context.Context, name, versionHash string, manifest []ManifestEntry, manifestJSON []byte, totalSize int64) error {
	c := dc.client

	row := map[string]any{
		"project_id":       c.projectID,
		"name":             name,
		"version_hash":     versionHash,
		"manifest":         json.RawMessage(manifestJSON),
		"total_size_bytes": totalSize,
		"file_count":       len(manifest),
	}
	body, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("marshal insert: %w", err)
	}

	insertURL := c.supabaseURL + "/rest/v1/c4_datasets"
	req, err := http.NewRequestWithContext(ctx, "POST", insertURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create insert request: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(body))), nil
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}
	defer resp.Body.Close()
	// 409 = ON CONFLICT DO NOTHING — acceptable
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("insert version (HTTP %d): %s", resp.StatusCode, string(b))
	}
	return nil
}
