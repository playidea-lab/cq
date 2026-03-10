package edgeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// controlPollInterval is the interval between control message polls.
const controlPollInterval = 30 * time.Second

// ControlPoller polls Hub GET /v1/edges/{id}/control every controlPollInterval and executes received actions.
type ControlPoller struct {
	edgeID   string
	hubURL   string
	apiKey   string
	driveURL string
	driveKey string
	workdir  string // base directory allowed for collect actions (path traversal guard)
	client   *http.Client
}

func newControlPoller(edgeID, hubURL, apiKey, driveURL, driveKey, workdir string, client *http.Client) *ControlPoller {
	return &ControlPoller{
		edgeID:   edgeID,
		hubURL:   hubURL,
		apiKey:   apiKey,
		driveURL: driveURL,
		driveKey: driveKey,
		workdir:  workdir,
		client:   client,
	}
}

// Start runs the control poll loop until ctx is done.
func (c *ControlPoller) Start(ctx context.Context) {
	tick := time.NewTicker(controlPollInterval)
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
		// Reject paths that escape the configured workdir (path traversal guard).
		if c.workdir != "" {
			abs, err := filepath.Abs(localPath)
			if err != nil {
				log.Printf("edge-agent: collect rejected unresolvable path: %s: %v", localPath, err)
				return
			}
			base := filepath.Clean(c.workdir) + string(filepath.Separator)
			if !strings.HasPrefix(abs+string(filepath.Separator), base) {
				log.Printf("edge-agent: collect rejected path outside workdir: %s", localPath)
				return
			}
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
	case "exec":
		shellCmd := msg.Params["cmd"]
		if shellCmd == "" {
			log.Printf("edge-agent: exec action missing cmd param")
			return
		}
		timeout := 60 * time.Second
		execCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		pr, pw := io.Pipe()
		execCmd := exec.CommandContext(execCtx, "sh", "-c", shellCmd)
		execCmd.Stdout = pw
		execCmd.Stderr = pw
		if err := execCmd.Start(); err != nil {
			pw.Close()
			log.Printf("edge-agent: exec start failed: %v", err)
			return
		}
		// Close pipe writer when process exits so scanner terminates.
		go func() {
			execCmd.Wait() //nolint:errcheck
			pw.Close()
		}()
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			log.Printf("edge-agent: exec> %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Printf("edge-agent: exec scanner error: %v", err)
		}
		if execCmd.ProcessState.ExitCode() != 0 {
			log.Printf("edge-agent: exec failed (exit %d)", execCmd.ProcessState.ExitCode())
		} else {
			log.Printf("edge-agent: exec done")
		}
	default:
		log.Printf("edge-agent: unknown control action: %s", msg.Action)
	}
}

// uploadToDrive uploads a local file to Supabase Storage.
// driveURL is the Supabase project URL (e.g. https://<ref>.supabase.co).
// driveKey is the JWT or anon key.
// Files are stored at: storage/v1/object/c4-drive/edges/{edgeID}/{filename}
func (c *ControlPoller) uploadToDrive(ctx context.Context, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}

	filename := filepath.Base(localPath)
	storagePath := "edges/" + c.edgeID + "/" + filename
	uploadURL := strings.TrimRight(c.driveURL, "/") + "/storage/v1/object/c4-drive/" + url.PathEscape(storagePath)

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, f)
	if err != nil {
		return err
	}
	req.ContentLength = fi.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	if c.driveKey != "" {
		req.Header.Set("apikey", c.driveKey)
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
