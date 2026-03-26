//go:build c0_drive

package drive

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	tusChunkSize    = 6 << 20 // 6MB
	tusMaxRetries   = 3
	tusChunkTimeout = 30 * time.Second
)

// tusUpload uploads a file using the TUS resumable upload protocol.
// It creates the upload session, then sends the file in 6MB chunks with
// per-chunk retry and progress reporting. On network failure mid-upload it
// issues a HEAD to discover the current server offset and resumes from there.
func tusUpload(ctx context.Context, supabaseURL, bucketName, storagePath, localPath string, setHeaders func(*http.Request)) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("tus: open %s: %w", localPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("tus: stat %s: %w", localPath, err)
	}
	totalSize := fi.Size()

	// --- Step 1: Create upload session (POST) ---
	location, err := tusCreateUpload(ctx, supabaseURL, bucketName, storagePath, totalSize, setHeaders)
	if err != nil {
		return fmt.Errorf("tus: create upload: %w", err)
	}

	// --- Step 2: Upload chunks (PATCH) ---
	return tusUploadChunks(ctx, f, location, totalSize, localPath, setHeaders)
}

// tusCreateUpload creates a TUS upload session and returns the upload Location URL.
func tusCreateUpload(ctx context.Context, supabaseURL, bucketName, storagePath string, totalSize int64, setHeaders func(*http.Request)) (string, error) {
	url := supabaseURL + "/storage/v1/upload/resumable"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	setHeaders(req)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(totalSize, 10))
	req.Header.Set("Upload-Metadata", tusBuildMetadata(bucketName, storagePath))
	req.Header.Set("x-upsert", "true")
	req.Header.Set("Content-Length", "0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("POST returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("POST response missing Location header")
	}
	return location, nil
}

// tusUploadChunks sends file data to the TUS upload URL in 6MB chunks.
func tusUploadChunks(ctx context.Context, f *os.File, location string, totalSize int64, localPath string, setHeaders func(*http.Request)) error {
	client := &http.Client{}
	buf := make([]byte, tusChunkSize)
	var offset int64

	baseName := filepath.Base(localPath)

	for offset < totalSize {
		// Seek to current offset for retry correctness.
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("tus: seek to %d: %w", offset, err)
		}

		n, readErr := io.ReadFull(f, buf)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return fmt.Errorf("tus: read chunk at %d: %w", offset, readErr)
		}
		if n == 0 {
			break
		}
		chunk := buf[:n]

		// Retry loop per chunk.
		var patchErr error
		for attempt := 0; attempt < tusMaxRetries; attempt++ {
			if attempt > 0 {
				// On retry: HEAD to discover actual server offset (resume).
				serverOffset, headErr := tusGetOffset(ctx, location, setHeaders)
				if headErr == nil && serverOffset > offset {
					// Server already has data beyond our local offset — skip ahead.
					offset = serverOffset
					if _, err := f.Seek(offset, io.SeekStart); err != nil {
						return fmt.Errorf("tus: seek after resume to %d: %w", offset, err)
					}
					n, readErr = io.ReadFull(f, buf)
					if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
						return fmt.Errorf("tus: read chunk after resume at %d: %w", offset, readErr)
					}
					if n == 0 {
						goto done
					}
					chunk = buf[:n]
				}
			}

			chunkCtx, cancel := context.WithTimeout(ctx, tusChunkTimeout)
			patchErr = tusPatchChunk(chunkCtx, client, location, chunk, offset, setHeaders)
			cancel()
			if patchErr == nil {
				break
			}
		}
		if patchErr != nil {
			return fmt.Errorf("tus: patch at offset %d: %w", offset, patchErr)
		}

		offset += int64(n)

		pct := int64(100)
		if totalSize > 0 {
			pct = offset * 100 / totalSize
		}
		fmt.Printf("drive: upload %s %d/%d (%d%%)\n", baseName, offset, totalSize, pct)
	}

done:
	return nil
}

// tusPatchChunk sends a single chunk via PATCH.
func tusPatchChunk(ctx context.Context, client *http.Client, location string, chunk []byte, offset int64, setHeaders func(*http.Request)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, location, bytesReader(chunk))
	if err != nil {
		return err
	}
	setHeaders(req)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.Header.Set("Content-Length", strconv.Itoa(len(chunk)))
	req.ContentLength = int64(len(chunk))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// tusGetOffset issues a TUS HEAD request to discover the server's current offset.
func tusGetOffset(ctx context.Context, location string, setHeaders func(*http.Request)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, location, nil)
	if err != nil {
		return 0, err
	}
	setHeaders(req)
	req.Header.Set("Tus-Resumable", "1.0.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return 0, fmt.Errorf("HEAD returned HTTP %d", resp.StatusCode)
	}

	offsetStr := resp.Header.Get("Upload-Offset")
	if offsetStr == "" {
		return 0, fmt.Errorf("HEAD missing Upload-Offset header")
	}
	return strconv.ParseInt(offsetStr, 10, 64)
}

// tusBuildMetadata constructs the TUS Upload-Metadata header value.
// Each field is "key base64value" and fields are comma-separated.
// Standard base64 encoding without padding per TUS spec.
func tusBuildMetadata(bucketName, objectName string) string {
	enc := base64.RawStdEncoding
	return fmt.Sprintf(
		"bucketName %s,objectName %s,contentType %s",
		enc.EncodeToString([]byte(bucketName)),
		enc.EncodeToString([]byte(objectName)),
		enc.EncodeToString([]byte("application/octet-stream")),
	)
}

// bytesReader wraps a []byte as an io.Reader for use in http.Request bodies.
// We use a bytes.Reader to avoid the overhead of io.NopCloser on a raw slice.
type bytesReaderCloser struct {
	data   []byte
	offset int
}

func bytesReader(b []byte) *bytesReaderCloser {
	return &bytesReaderCloser{data: b}
}

func (r *bytesReaderCloser) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *bytesReaderCloser) Close() error { return nil }
