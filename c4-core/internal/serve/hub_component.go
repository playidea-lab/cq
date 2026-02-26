package serve

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// HubComponentConfig holds configuration for the HubComponent.
type HubComponentConfig struct {
	// Binary is the name or path of the c5 hub binary (default: "c5").
	Binary string
	// Port is the port c5 hub will listen on (default: 8585).
	Port int
	// Args are extra CLI arguments passed after "serve --port <port>".
	Args []string
	// ExtractBinary is an optional function that extracts an embedded c5 binary
	// and returns its path. When set, it is called if the binary is not found
	// in PATH (c5_embed build tag scenario).
	ExtractBinary func() (string, error)
}

// HubComponent manages a C5 Hub server subprocess.
// Because c4-core and c5 are separate Go modules, they cannot be imported
// directly. Instead, this component launches the c5 binary as a child process.
type HubComponent struct {
	cfg HubComponentConfig

	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	running bool
	done    chan struct{} // closed by reaper goroutine when process exits
}

// NewHubComponent creates a HubComponent with the given config.
// Sensible defaults are applied for zero-value fields.
func NewHubComponent(cfg HubComponentConfig) *HubComponent {
	if cfg.Binary == "" {
		cfg.Binary = "c5"
	}
	if cfg.Port == 0 {
		cfg.Port = 8585
	}
	return &HubComponent{cfg: cfg}
}

func (h *HubComponent) Name() string { return "hub" }

// Start launches the c5 binary as a subprocess.
// If the binary is not found in PATH, Start logs a WARN and returns nil
// (graceful skip — hub is optional).
func (h *HubComponent) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return fmt.Errorf("hub component already running")
	}

	// Check binary existence before attempting to start.
	binPath, err := exec.LookPath(h.cfg.Binary)
	if err != nil && h.cfg.ExtractBinary != nil {
		// Binary not in PATH but we have an embedded binary — extract it.
		extracted, extractErr := h.cfg.ExtractBinary()
		if extractErr != nil {
			return fmt.Errorf("hub: extract embedded binary: %w", extractErr)
		}
		binPath = extracted
		err = nil
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: hub component: binary %q not found in PATH — skipping (WARN)\n", h.cfg.Binary)
		return nil
	}

	args := append([]string{"serve", "--port", fmt.Sprintf("%d", h.cfg.Port)}, h.cfg.Args...)

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, binPath, args...)
	cmd.Stdout = os.Stderr // route subprocess output to our stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("hub: start subprocess: %w", err)
	}

	done := make(chan struct{})
	h.cmd = cmd
	h.cancel = cancel
	h.running = true
	h.done = done

	fmt.Fprintf(os.Stderr, "cq serve: hub component started (binary: %s, port: %d, pid: %d)\n",
		binPath, h.cfg.Port, cmd.Process.Pid)

	// Single reaper goroutine: calls Wait exactly once and closes done.
	// Stop() listens on done instead of calling Wait again.
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	return nil
}

// Stop sends SIGTERM to the subprocess and waits up to 5 s for it to exit.
// If the process does not exit in time, SIGKILL is sent.
func (h *HubComponent) Stop(ctx context.Context) error {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return nil
	}

	// Copy state under lock, then release so Health() is not blocked during shutdown.
	proc := h.cmd.Process
	done := h.done
	cancel := h.cancel
	h.running = false
	h.mu.Unlock()

	if proc != nil {
		// Graceful: SIGTERM
		_ = proc.Signal(syscall.SIGTERM)

		// Wait for the reaper goroutine (single Wait() caller) to signal exit.
		select {
		case <-done:
			// exited cleanly
		case <-ctx.Done():
			// caller cancelled — force-kill
			_ = proc.Signal(syscall.SIGKILL)
			<-done
		case <-time.After(5 * time.Second):
			// Force-kill if still running.
			_ = proc.Signal(syscall.SIGKILL)
			<-done
		}
	}

	if cancel != nil {
		cancel()
	}

	fmt.Fprintf(os.Stderr, "cq serve: hub component stopped\n")
	return nil
}

// Health checks whether the c5 hub HTTP /health endpoint is reachable.
func (h *HubComponent) Health() ComponentHealth {
	h.mu.Lock()
	running := h.running
	port := h.cfg.Port
	h.mu.Unlock()

	if !running {
		return ComponentHealth{Status: "error", Detail: "not running"}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("health check failed: %v", err)}
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ComponentHealth{Status: "degraded", Detail: fmt.Sprintf("health check status: %d", resp.StatusCode)}
	}
	return ComponentHealth{Status: "ok", Detail: fmt.Sprintf("port %d", port)}
}
