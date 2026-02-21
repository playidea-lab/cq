package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/changmin/c4-core/internal/daemon"
)

// GPUComponent wraps the daemon Scheduler and Server as a serve Component.
// It manages the job queue, process execution, and GPU monitoring lifecycle.
// When no GPUs are detected, it operates in CPU-only mode.
type GPUComponent struct {
	mu     sync.Mutex
	store  *daemon.Store
	sched  *daemon.Scheduler
	srv    *daemon.Server
	gpu    *daemon.GpuMonitor
	cancel context.CancelFunc

	// config
	dataDir    string
	maxJobs    int
	version    string

	// state
	running bool
}

// GPUComponentConfig holds configuration for the GPU component.
type GPUComponentConfig struct {
	DataDir    string // default: ~/.c4/daemon
	MaxJobs    int    // max concurrent CPU jobs (0 = unlimited)
	Version    string // build version string
}

// NewGPUComponent creates a new GPU scheduler component.
func NewGPUComponent(cfg GPUComponentConfig) *GPUComponent {
	return &GPUComponent{
		dataDir: cfg.DataDir,
		maxJobs: cfg.MaxJobs,
		version: cfg.Version,
	}
}

func (g *GPUComponent) Name() string { return "gpu" }

func (g *GPUComponent) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return fmt.Errorf("gpu component already running")
	}

	// Resolve data directory
	dataDir := g.dataDir
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".c4", "daemon")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Open store
	dbPath := filepath.Join(dataDir, "daemon.db")
	store, err := daemon.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	// GPU monitor
	gpu := daemon.NewGpuMonitor()
	gpuCount := gpu.GPUCount()

	// Scheduler
	schedCtx, cancel := context.WithCancel(ctx)

	sched := daemon.NewScheduler(store, daemon.SchedulerConfig{
		DataDir:       dataDir,
		GPUCount:      gpuCount,
		MaxConcurrent: g.maxJobs,
	})
	sched.Start(schedCtx)

	// HTTP server (for handler only, not standalone listen)
	srv := daemon.NewServer(daemon.ServerConfig{
		Store:      store,
		Scheduler:  sched,
		GpuMonitor: gpu,
		Version:    g.version,
		CancelFunc: cancel,
	})

	g.store = store
	g.sched = sched
	g.srv = srv
	g.gpu = gpu
	g.cancel = cancel
	g.dataDir = dataDir
	g.running = true

	mode := "CPU-only"
	if gpuCount > 0 {
		mode = fmt.Sprintf("%d GPU(s)", gpuCount)
	}
	fmt.Fprintf(os.Stderr, "cq serve: gpu component started (%s, data: %s)\n", mode, dataDir)

	return nil
}

func (g *GPUComponent) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.running {
		return nil
	}

	if g.sched != nil {
		g.sched.Stop()
	}
	if g.cancel != nil {
		g.cancel()
	}
	if g.store != nil {
		g.store.Close()
	}

	g.running = false
	return nil
}

func (g *GPUComponent) Health() ComponentHealth {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.running {
		return ComponentHealth{Status: "error", Detail: "not running"}
	}

	gpuCount := 0
	if g.gpu != nil {
		gpuCount = g.gpu.GPUCount()
	}

	activeJobs := 0
	if g.sched != nil {
		activeJobs = g.sched.RunningCount()
	}

	mode := "cpu-only"
	if gpuCount > 0 {
		mode = fmt.Sprintf("%d-gpu", gpuCount)
	}

	return ComponentHealth{
		Status: "ok",
		Detail: fmt.Sprintf("%s, %d active jobs", mode, activeJobs),
	}
}

// Handler returns the daemon HTTP handler for mounting on the serve mux.
// Must be called after Start.
func (g *GPUComponent) Handler() http.Handler {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.srv == nil {
		// Return a handler that always returns 503 if not started
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gpu component not started", http.StatusServiceUnavailable)
		})
	}
	return g.srv.Handler()
}
