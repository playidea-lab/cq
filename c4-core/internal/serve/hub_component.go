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

	h.cmd = cmd
	h.cancel = cancel
	h.running = true

	fmt.Fprintf(os.Stderr, "cq serve: hub component started (binary: %s, port: %d, pid: %d)\n",
		binPath, h.cfg.Port, cmd.Process.Pid)

	// Reap the child process in a goroutine so it doesn't become a zombie.
	go func() {
		_ = cmd.Wait()
	}()

	return nil
}

// Stop sends SIGTERM to the subprocess and waits up to 5 s for it to exit.
// If the process does not exit in time, SIGKILL is sent.
func (h *HubComponent) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return nil
	}

	if h.cmd != nil && h.cmd.Process != nil {
		// Graceful: SIGTERM
		_ = h.cmd.Process.Signal(syscall.SIGTERM)

		// Wait up to 5 s for the process to exit.
		done := make(chan struct{})
		go func() {
			_ = h.cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			// exited cleanly
		case <-time.After(5 * time.Second):
			// Force-kill if still running.
			_ = h.cmd.Process.Signal(syscall.SIGKILL)
		}
	}

	if h.cancel != nil {
		h.cancel()
	}

	h.running = false
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

	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
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
