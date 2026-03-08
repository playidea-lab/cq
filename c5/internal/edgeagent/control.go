package edgeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// ControlPoller polls Hub GET /v1/edges/{id}/control every 30s and executes received actions.
type ControlPoller struct {
	edgeID     string
	hubURL     string
	apiKey     string
	driveURL   string
	driveKey   string
	client     *http.Client
}

func newControlPoller(edgeID, hubURL, apiKey, driveURL, driveKey string, client *http.Client) *ControlPoller {
	return &ControlPoller{
		edgeID:   edgeID,
		hubURL:   hubURL,
		apiKey:   apiKey,
		driveURL: driveURL,
		driveKey: driveKey,
		client:   client,
	}
}

// Start runs the control poll loop until ctx is done.
func (c *ControlPoller) Start(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			msgs, err := c.Poll(ctx)
			if err != nil {
				log.Printf("edge-agent: control poll: %v", err)
				continue
			}
			for _, msg := range msgs {
				c.handle(ctx, &msg)
			}
		}
	}
}

// Poll fetches control messages from Hub.
func (c *ControlPoller) Poll(ctx context.Context) ([]model.EdgeControlMessage, error) {
	url := fmt.Sprintf("%s/v1/edges/%s/control", strings.TrimRight(c.hubURL, "/"), c.edgeID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("control poll status %d: %s", resp.StatusCode, string(body))
	}
	var msgs []model.EdgeControlMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, fmt.Errorf("decode control messages: %w", err)
	}
	return msgs, nil
}

func (c *ControlPoller) handle(ctx context.Context, msg *model.EdgeControlMessage) {
	switch msg.Action {
	case "collect":
		localPath := msg.Params["local_path"]
		if localPath == "" {
			log.Printf("edge-agent: collect action missing local_path")
			return
		}
		// Reject path traversal attempts from potentially compromised Hub messages.
		if strings.Contains(filepath.Clean(localPath), "..") {
			log.Printf("edge-agent: collect rejected suspicious path: %s", localPath)
			return
		}
		if c.driveURL == "" {
			log.Printf("edge-agent: collect action received but DriveURL not configured; skipping upload (local_path=%s)", localPath)
			return
		}
		if err := c.uploadToDrive(ctx, localPath); err != nil {
			log.Printf("edge-agent: collect upload failed: %v", err)
		} else {
			log.Printf("edge-agent: collect uploaded %s to Drive", localPath)
		}
	default:
		log.Printf("edge-agent: unknown control action: %s", msg.Action)
	}
}

func (c *ControlPoller) uploadToDrive(ctx context.Context, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	mw.Close()

	params := url.Values{}
	params.Set("path", filepath.Base(localPath))
	driveBase := strings.TrimRight(c.driveURL, "/")
	uploadURL := driveBase + "/upload?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.driveKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.driveKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drive upload status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
