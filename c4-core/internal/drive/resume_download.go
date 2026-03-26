//go:build c0_drive

package drive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	resumeMaxRetries     = 3
	resumeAttemptTimeout = 60 * time.Second
)

// resumeDownload downloads a file from downloadURL to destPath with HTTP Range
// resume support. If a partial file exists at destPath+".part", the download
// resumes from where it left off. On success the .part file is renamed to
// destPath atomically.
//
// setHeaders is called on each request to inject authentication or other
// headers (e.g., Supabase apikey / Authorization).
func resumeDownload(ctx context.Context, downloadURL, destPath string, setHeaders func(*http.Request)) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("drive: create dest dir: %w", err)
	}

	partPath := destPath + ".part"

	var lastErr error
	for attempt := range resumeMaxRetries {
		if attempt > 0 {
			delay := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := resumeAttempt(ctx, downloadURL, destPath, partPath, setHeaders); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// resumeAttempt performs a single download attempt with Range support.
func resumeAttempt(ctx context.Context, downloadURL, destPath, partPath string, setHeaders func(*http.Request)) error {
	// Determine current offset from .part file.
	var offset int64
	if fi, err := os.Stat(partPath); err == nil {
		offset = fi.Size()
	}

	// Build GET request.
	attemptCtx, cancel := context.WithTimeout(ctx, resumeAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("drive: create request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	setHeaders(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("drive: GET %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drive: download failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// If we requested a range but the server returned 200, it doesn't support
	// Range — truncate the .part file and start over.
	if offset > 0 && resp.StatusCode == http.StatusOK {
		offset = 0
		if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("drive: truncate stale .part: %w", err)
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("drive: unexpected status %d", resp.StatusCode)
	}

	// Open .part file: append if resuming, create/truncate if fresh.
	flag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flag, 0o644)
	if err != nil {
		return fmt.Errorf("drive: open .part file: %w", err)
	}

	// Track progress when Content-Length is known.
	totalSize := offset + resp.ContentLength
	var downloaded int64

	_, copyErr := io.Copy(&progressWriter{
		w:          f,
		name:       filepath.Base(destPath),
		base:       offset,
		total:      totalSize,
		downloaded: &downloaded,
		known:      resp.ContentLength > 0,
	}, resp.Body)

	closeErr := f.Close()

	if copyErr != nil {
		// Keep .part for next resume attempt.
		return fmt.Errorf("drive: write .part: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("drive: close .part: %w", closeErr)
	}

	// Atomic rename .part → dest.
	if err := os.Rename(partPath, destPath); err != nil {
		return fmt.Errorf("drive: rename .part to dest: %w", err)
	}
	return nil
}

// progressWriter wraps an io.Writer and prints download progress.
type progressWriter struct {
	w          io.Writer
	name       string
	base       int64 // bytes already on disk before this attempt
	total      int64 // total file size (0 if unknown)
	downloaded *int64
	known      bool
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	*pw.downloaded += int64(n)
	if pw.known && pw.total > 0 {
		soFar := pw.base + *pw.downloaded
		pct := soFar * 100 / pw.total
		fmt.Printf("drive: download %s %d/%d (%d%%)\n", pw.name, soFar, pw.total, pct)
	}
	return n, err
}
