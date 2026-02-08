package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// =========================================================================
// Fly.io Machine Models
// =========================================================================

// Machine represents a Fly.io machine (Cloud Worker).
type Machine struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	State     string            `json:"state"` // "created", "started", "stopped", "destroyed"
	Region    string            `json:"region"`
	ImageRef  string            `json:"image_ref"`
	TaskID    string            `json:"task_id,omitempty"` // C4 task assigned
	CreatedAt time.Time         `json:"created_at"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// MachineConfig configures a Fly.io machine to create.
type MachineConfig struct {
	Name     string            `json:"name"`
	Region   string            `json:"region,omitempty"`
	Image    string            `json:"image"`
	CPUs     int               `json:"cpus,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// DefaultMachineConfig returns the default config for a C4 Cloud Worker.
func DefaultMachineConfig() *MachineConfig {
	return &MachineConfig{
		Image:    "c4-worker:latest",
		CPUs:     2,
		MemoryMB: 2048,
		Env:      map[string]string{},
		Labels: map[string]string{
			"managed-by": "c4-cloud",
		},
	}
}

// ScaleConfig configures auto-scaling behavior.
type ScaleConfig struct {
	MinWorkers     int           // Minimum workers to keep running (default 0)
	MaxWorkers     int           // Maximum workers allowed (default 10)
	ScaleUpAt      int           // Queue depth to trigger scale-up (default 3)
	ScaleDownDelay time.Duration // Wait before scaling down idle (default 5min)
	CooldownPeriod time.Duration // Min time between scaling events (default 1min)
}

// DefaultScaleConfig returns sensible auto-scaling defaults.
func DefaultScaleConfig() *ScaleConfig {
	return &ScaleConfig{
		MinWorkers:     0,
		MaxWorkers:     10,
		ScaleUpAt:      3,
		ScaleDownDelay: 5 * time.Minute,
		CooldownPeriod: 1 * time.Minute,
	}
}

// =========================================================================
// FlyClient interface (for testing)
// =========================================================================

// FlyClient abstracts the Fly.io Machines API.
type FlyClient interface {
	// CreateMachine creates a new machine.
	CreateMachine(cfg *MachineConfig) (*Machine, error)

	// DestroyMachine stops and destroys a machine.
	DestroyMachine(machineID string) error

	// ListMachines returns all managed machines.
	ListMachines() ([]*Machine, error)

	// GetMachine returns a single machine by ID.
	GetMachine(machineID string) (*Machine, error)
}

// =========================================================================
// HTTP-based FlyClient
// =========================================================================

// HTTPFlyClient implements FlyClient using the Fly.io Machines REST API.
type HTTPFlyClient struct {
	AppName  string
	APIToken string
	BaseURL  string
	client   *http.Client
}

// NewHTTPFlyClient creates a Fly.io client.
func NewHTTPFlyClient(appName, apiToken string) *HTTPFlyClient {
	return &HTTPFlyClient{
		AppName:  appName,
		APIToken: apiToken,
		BaseURL:  "https://api.machines.dev",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateMachine creates a Fly.io machine via the Machines API.
func (c *HTTPFlyClient) CreateMachine(cfg *MachineConfig) (*Machine, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines", c.BaseURL, c.AppName)

	body := map[string]any{
		"name":   cfg.Name,
		"region": cfg.Region,
		"config": map[string]any{
			"image": cfg.Image,
			"guest": map[string]any{
				"cpus":    cfg.CPUs,
				"memory_mb": cfg.MemoryMB,
			},
			"env": cfg.Env,
		},
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create machine: HTTP %d", resp.StatusCode)
	}

	var machine Machine
	if err := json.NewDecoder(resp.Body).Decode(&machine); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	machine.Labels = cfg.Labels
	return &machine, nil
}

// DestroyMachine stops and destroys a Fly.io machine.
func (c *HTTPFlyClient) DestroyMachine(machineID string) error {
	// First stop the machine
	stopURL := fmt.Sprintf("%s/v1/apps/%s/machines/%s/stop", c.BaseURL, c.AppName, machineID)
	stopReq, _ := http.NewRequest("POST", stopURL, nil)
	c.setHeaders(stopReq)
	c.client.Do(stopReq)

	// Then destroy
	destroyURL := fmt.Sprintf("%s/v1/apps/%s/machines/%s", c.BaseURL, c.AppName, machineID)
	req, err := http.NewRequest("DELETE", destroyURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("destroy machine: HTTP %d", resp.StatusCode)
	}

	return nil
}

// ListMachines lists all machines for the app.
func (c *HTTPFlyClient) ListMachines() ([]*Machine, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines", c.BaseURL, c.AppName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list machines: HTTP %d", resp.StatusCode)
	}

	var machines []*Machine
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return machines, nil
}

// GetMachine gets a single machine.
func (c *HTTPFlyClient) GetMachine(machineID string) (*Machine, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/machines/%s", c.BaseURL, c.AppName, machineID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get machine: HTTP %d", resp.StatusCode)
	}

	var machine Machine
	if err := json.NewDecoder(resp.Body).Decode(&machine); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &machine, nil
}

func (c *HTTPFlyClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken))
	req.Header.Set("Content-Type", "application/json")
}

// =========================================================================
// CloudWorkerManager
// =========================================================================

// CloudWorkerManager manages Cloud Workers on Fly.io with auto-scaling.
type CloudWorkerManager struct {
	flyClient   FlyClient
	scaleConfig *ScaleConfig
	mu          sync.RWMutex
	workers     map[string]*Machine // machineID -> Machine
	lastScale   time.Time
}

// NewCloudWorkerManager creates a manager with the given Fly.io client.
func NewCloudWorkerManager(flyClient FlyClient, scaleCfg *ScaleConfig) *CloudWorkerManager {
	if scaleCfg == nil {
		scaleCfg = DefaultScaleConfig()
	}

	return &CloudWorkerManager{
		flyClient:   flyClient,
		scaleConfig: scaleCfg,
		workers:     make(map[string]*Machine),
	}
}

// CreateWorker creates a new Cloud Worker for a task.
func (m *CloudWorkerManager) CreateWorker(taskID string, cfg *MachineConfig) (string, error) {
	if cfg == nil {
		cfg = DefaultMachineConfig()
	}

	// Add task-specific config
	if cfg.Env == nil {
		cfg.Env = make(map[string]string)
	}
	cfg.Env["C4_TASK_ID"] = taskID

	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}
	cfg.Labels["c4-task"] = taskID

	machine, err := m.flyClient.CreateMachine(cfg)
	if err != nil {
		return "", fmt.Errorf("create machine: %w", err)
	}

	machine.TaskID = taskID

	m.mu.Lock()
	m.workers[machine.ID] = machine
	m.mu.Unlock()

	return machine.ID, nil
}

// DestroyWorker destroys a Cloud Worker.
func (m *CloudWorkerManager) DestroyWorker(machineID string) error {
	if err := m.flyClient.DestroyMachine(machineID); err != nil {
		return fmt.Errorf("destroy machine: %w", err)
	}

	m.mu.Lock()
	delete(m.workers, machineID)
	m.mu.Unlock()

	return nil
}

// ListWorkers returns all tracked Cloud Workers.
func (m *CloudWorkerManager) ListWorkers() []*Machine {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Machine, 0, len(m.workers))
	for _, w := range m.workers {
		result = append(result, w)
	}
	return result
}

// WorkerCount returns the current number of tracked workers.
func (m *CloudWorkerManager) WorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// ScaleTo sets the number of workers to the target count.
// Creates or destroys workers to match.
func (m *CloudWorkerManager) ScaleTo(target int) error {
	if target < m.scaleConfig.MinWorkers {
		target = m.scaleConfig.MinWorkers
	}
	if target > m.scaleConfig.MaxWorkers {
		target = m.scaleConfig.MaxWorkers
	}

	current := m.WorkerCount()

	if current == target {
		return nil
	}

	if current < target {
		// Scale up
		for i := current; i < target; i++ {
			cfg := DefaultMachineConfig()
			cfg.Name = fmt.Sprintf("c4-worker-%d", i+1)
			_, err := m.CreateWorker("", cfg)
			if err != nil {
				return fmt.Errorf("scale up: %w", err)
			}
		}
	} else {
		// Scale down: remove workers without assigned tasks first
		m.mu.RLock()
		toRemove := make([]string, 0)
		for id, w := range m.workers {
			if w.TaskID == "" && len(toRemove) < current-target {
				toRemove = append(toRemove, id)
			}
		}
		// If still need more, remove any
		for id := range m.workers {
			if len(toRemove) >= current-target {
				break
			}
			found := false
			for _, r := range toRemove {
				if r == id {
					found = true
					break
				}
			}
			if !found {
				toRemove = append(toRemove, id)
			}
		}
		m.mu.RUnlock()

		for _, id := range toRemove {
			if err := m.DestroyWorker(id); err != nil {
				return fmt.Errorf("scale down: %w", err)
			}
		}
	}

	m.mu.Lock()
	m.lastScale = time.Now()
	m.mu.Unlock()

	return nil
}

// EvaluateScale checks the task queue depth and adjusts worker count.
// Returns the number of workers created or destroyed (positive=up, negative=down).
func (m *CloudWorkerManager) EvaluateScale(queueDepth int) (int, error) {
	m.mu.RLock()
	lastScale := m.lastScale
	current := len(m.workers)
	m.mu.RUnlock()

	// Respect cooldown
	if !lastScale.IsZero() && time.Since(lastScale) < m.scaleConfig.CooldownPeriod {
		return 0, nil
	}

	var target int

	if queueDepth >= m.scaleConfig.ScaleUpAt {
		// Scale up: 1 worker per ScaleUpAt tasks
		target = current + (queueDepth / m.scaleConfig.ScaleUpAt)
	} else if queueDepth == 0 && current > m.scaleConfig.MinWorkers {
		// Scale down to min
		target = m.scaleConfig.MinWorkers
	} else {
		return 0, nil // no change needed
	}

	// Clamp
	if target < m.scaleConfig.MinWorkers {
		target = m.scaleConfig.MinWorkers
	}
	if target > m.scaleConfig.MaxWorkers {
		target = m.scaleConfig.MaxWorkers
	}

	if target == current {
		return 0, nil
	}

	diff := target - current
	if err := m.ScaleTo(target); err != nil {
		return 0, err
	}

	return diff, nil
}

// SyncFromFly refreshes the local worker list from Fly.io API.
func (m *CloudWorkerManager) SyncFromFly() error {
	machines, err := m.flyClient.ListMachines()
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.workers = make(map[string]*Machine)
	for _, machine := range machines {
		m.workers[machine.ID] = machine
	}

	return nil
}
