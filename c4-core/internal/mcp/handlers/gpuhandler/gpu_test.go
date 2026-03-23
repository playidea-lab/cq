//go:build gpu


package gpuhandler

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

func TestGpuStatusHandler_NoGPU(t *testing.T) {
	// GpuMonitor will fail on macOS/no-GPU — should return fallback
	mon := daemon.NewGpuMonitor()
	handler := gpuStatusHandler(mon)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}

	// Should have gpu_count (0 on macOS/no-GPU)
	if _, ok := m["gpu_count"]; !ok {
		t.Error("missing gpu_count field")
	}
	if _, ok := m["backend"]; !ok {
		t.Error("missing backend field")
	}
}

func TestJobSubmitHandler_NoCommand(t *testing.T) {
	handler := jobSubmitHandler(nil)

	args, _ := json.Marshal(map[string]any{})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] != "command is required" {
		t.Errorf("error = %v, want 'command is required'", m["error"])
	}
}

func TestJobSubmitHandler_NoStore(t *testing.T) {
	handler := jobSubmitHandler(nil)

	args, _ := json.Marshal(map[string]any{"command": "python train.py"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobSubmitHandler_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSubmitHandler(store)

	args, _ := json.Marshal(map[string]any{"command": "python train.py", "priority": 5})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["job_id"] == nil || m["job_id"] == "" {
		t.Error("expected non-empty job_id")
	}
}

func TestJobSubmitHandler_WithExtendedParams(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSubmitHandler(store)

	args, _ := json.Marshal(map[string]any{
		"command":     "python train.py",
		"exp_id":      "exp001",
		"tags":        []string{"gpu", "training"},
		"env":         map[string]string{"CUDA_VISIBLE_DEVICES": "0"},
		"timeout_sec": 3600,
		"memo":        "baseline run",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	jobID, _ := m["job_id"].(string)
	if jobID == "" {
		t.Error("expected non-empty job_id")
	}

	// Verify fields were persisted
	job, err := store.GetJob(jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.ExpID != "exp001" {
		t.Errorf("exp_id = %q, want 'exp001'", job.ExpID)
	}
	if job.Memo != "baseline run" {
		t.Errorf("memo = %q, want 'baseline run'", job.Memo)
	}
	if job.TimeoutSec != 3600 {
		t.Errorf("timeout_sec = %d, want 3600", job.TimeoutSec)
	}
	if len(job.Tags) != 2 {
		t.Errorf("tags len = %d, want 2", len(job.Tags))
	}
	if job.Env["CUDA_VISIBLE_DEVICES"] != "0" {
		t.Errorf("env CUDA_VISIBLE_DEVICES = %q, want '0'", job.Env["CUDA_VISIBLE_DEVICES"])
	}
}

func TestJobSubmitHandler_WithRoutingParams(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSubmitHandler(store)

	args, _ := json.Marshal(map[string]any{
		"command":       "python train.py",
		"capability":    "gpu-a100",
		"required_tags": []string{"prod", "high-mem"},
		"target_worker": "worker-abc123",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	jobID, _ := m["job_id"].(string)
	if jobID == "" {
		t.Error("expected non-empty job_id")
	}

	// Verify routing fields were persisted
	job, err := store.GetJob(jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Capability != "gpu-a100" {
		t.Errorf("capability = %q, want 'gpu-a100'", job.Capability)
	}
	if len(job.RequiredTags) != 2 || job.RequiredTags[0] != "prod" || job.RequiredTags[1] != "high-mem" {
		t.Errorf("required_tags = %v, want [prod high-mem]", job.RequiredTags)
	}
	if job.TargetWorker != "worker-abc123" {
		t.Errorf("target_worker = %q, want 'worker-abc123'", job.TargetWorker)
	}
}

// TestJobListHandler tests the c4_job_list handler.
func TestJobListHandler_NoStore(t *testing.T) {
	handler := jobListHandler(nil)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobListHandler_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobListHandler(store)

	result, err := handler([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty array, not an error
	switch v := result.(type) {
	case []map[string]any:
		if len(v) != 0 {
			t.Errorf("expected empty array, got %d items", len(v))
		}
	case []any:
		if len(v) != 0 {
			t.Errorf("expected empty array, got %d items", len(v))
		}
	default:
		t.Fatalf("expected array, got %T: %v", result, result)
	}
}

func TestJobListHandler_WithJobs(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Submit 2 jobs
	for i := 0; i < 2; i++ {
		_, err := store.CreateJob(&daemon.JobSubmitRequest{
			Name:    "test-job",
			Command: "echo hello",
			Workdir: ".",
		})
		if err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
	}

	handler := jobListHandler(store)

	result, err := handler([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	arr, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(arr))
	}
	// Each item should have job_id, name, status
	for _, item := range arr {
		if item["job_id"] == "" {
			t.Error("missing job_id in list item")
		}
		if item["status"] != "QUEUED" {
			t.Errorf("expected QUEUED status, got %v", item["status"])
		}
	}
}

func TestJobListHandler_WithStatusFilter(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	store.CreateJob(&daemon.JobSubmitRequest{Name: "j1", Command: "echo 1", Workdir: "."})
	store.CreateJob(&daemon.JobSubmitRequest{Name: "j2", Command: "echo 2", Workdir: "."})

	handler := jobListHandler(store)

	// Filter for RUNNING — should return 0
	args, _ := json.Marshal(map[string]any{"status": "RUNNING"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When the store returns nil/empty, handler returns []any{}
	switch v := result.(type) {
	case []map[string]any:
		if len(v) != 0 {
			t.Errorf("expected 0 RUNNING jobs, got %d", len(v))
		}
	case []any:
		if len(v) != 0 {
			t.Errorf("expected 0 RUNNING jobs, got %d", len(v))
		}
	default:
		t.Fatalf("expected array, got %T", result)
	}
}

// TestJobStatusHandler tests the c4_job_status handler.
func TestJobStatusHandler_NoStore(t *testing.T) {
	handler := jobStatusHandler(nil, nil)

	args, _ := json.Marshal(map[string]any{"job_id": "j-123"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobStatusHandler_MissingJobID(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobStatusHandler(store, nil)

	result, err := handler([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "job_id is required" {
		t.Errorf("error = %v, want 'job_id is required'", m["error"])
	}
}

func TestJobStatusHandler_WithJob(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	job, err := store.CreateJob(&daemon.JobSubmitRequest{
		Name:    "test-job",
		Command: "echo hello",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	handler := jobStatusHandler(store, nil)

	args, _ := json.Marshal(map[string]any{"job_id": job.ID})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["job_id"] != job.ID {
		t.Errorf("job_id = %v, want %s", m["job_id"], job.ID)
	}
	if m["status"] != "QUEUED" {
		t.Errorf("status = %v, want QUEUED", m["status"])
	}
	if m["name"] != "test-job" {
		t.Errorf("name = %v, want 'test-job'", m["name"])
	}
}

// TestJobCancelHandler tests the c4_job_cancel handler.
func TestJobCancelHandler_NoStore(t *testing.T) {
	handler := jobCancelHandler(nil, nil)

	args, _ := json.Marshal(map[string]any{"job_id": "j-123"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobCancelHandler_MissingJobID(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobCancelHandler(store, nil)

	result, err := handler([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "job_id is required" {
		t.Errorf("error = %v, want 'job_id is required'", m["error"])
	}
}

func TestJobCancelHandler_StoreOnly(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	job, err := store.CreateJob(&daemon.JobSubmitRequest{
		Name:    "cancel-test",
		Command: "sleep 100",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Cancel without scheduler — store-only path
	handler := jobCancelHandler(store, nil)

	args, _ := json.Marshal(map[string]any{"job_id": job.ID})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	// Message should warn about no process kill
	msg, _ := m["message"].(string)
	if msg == "" {
		t.Error("expected non-empty message")
	}

	// Verify job is cancelled in store
	updated, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if updated.Status != daemon.StatusCancelled {
		t.Errorf("status = %v, want CANCELLED", updated.Status)
	}
}

// TestJobSummaryHandler tests the c4_job_summary handler.
func TestJobSummaryHandler_NoStore(t *testing.T) {
	handler := jobSummaryHandler(nil)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobSummaryHandler_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSummaryHandler(store)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	// All fields should exist
	for _, field := range []string{"queued", "running", "succeeded", "failed", "cancelled", "total"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q", field)
		}
	}
	if m["total"] != 0 {
		t.Errorf("total = %v, want 0", m["total"])
	}
}

func TestJobSummaryHandler_WithJobs(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Submit 3 jobs
	for i := 0; i < 3; i++ {
		_, err := store.CreateJob(&daemon.JobSubmitRequest{
			Name:    "test-job",
			Command: "echo hello",
			Workdir: ".",
		})
		if err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
	}

	handler := jobSummaryHandler(store)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["queued"] != 3 {
		t.Errorf("queued = %v, want 3", m["queued"])
	}
	if m["total"] != 3 {
		t.Errorf("total = %v, want 3", m["total"])
	}
}

// TestJobStatusHandler_WidgetFormat tests that format=widget returns _meta.ui.
func TestJobStatusHandler_WidgetFormat(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Submit a job first
	job, err := store.CreateJob(&daemon.JobSubmitRequest{
		Name:    "test-widget",
		Command: "echo hello",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	handler := jobStatusHandler(store, nil)
	args, _ := json.Marshal(map[string]any{"job_id": job.ID, "format": "widget"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outer, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Must have data and _meta keys
	if _, hasData := outer["data"]; !hasData {
		t.Error("widget response must have 'data' key")
	}
	metaRaw, hasMeta := outer["_meta"]
	if !hasMeta {
		t.Fatal("widget response must have '_meta' key")
	}
	meta, ok := metaRaw.(map[string]any)
	if !ok {
		t.Fatalf("_meta must be map[string]any, got %T", metaRaw)
	}
	uiRaw, hasUI := meta["ui"]
	if !hasUI {
		t.Fatal("_meta must have 'ui' key")
	}
	ui, ok := uiRaw.(map[string]any)
	if !ok {
		t.Fatalf("_meta.ui must be map[string]any, got %T", uiRaw)
	}
	uri, _ := ui["resourceUri"].(string)
	if uri != jobProgressResourceURI {
		t.Errorf("resourceUri = %q, want %q", uri, jobProgressResourceURI)
	}

	// Verify data contains job fields
	data := outer["data"].(map[string]any)
	if data["job_id"] != job.ID {
		t.Errorf("data.job_id = %v, want %v", data["job_id"], job.ID)
	}
}

// TestJobStatusHandler_TextFormatNoMeta tests that format=text (default) has no _meta.
func TestJobStatusHandler_TextFormatNoMeta(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	job, err := store.CreateJob(&daemon.JobSubmitRequest{
		Name:    "test-text",
		Command: "echo hello",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	handler := jobStatusHandler(store, nil)
	args, _ := json.Marshal(map[string]any{"job_id": job.ID})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, hasMeta := m["_meta"]; hasMeta {
		t.Error("text format must not include _meta")
	}
}

// TestJobSummaryHandler_WidgetFormat tests that format=widget returns _meta.ui.
func TestJobSummaryHandler_WidgetFormat(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSummaryHandler(store)
	args, _ := json.Marshal(map[string]any{"format": "widget"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outer, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if _, hasData := outer["data"]; !hasData {
		t.Error("widget response must have 'data' key")
	}
	metaRaw, hasMeta := outer["_meta"]
	if !hasMeta {
		t.Fatal("widget response must have '_meta' key")
	}
	meta := metaRaw.(map[string]any)
	ui := meta["ui"].(map[string]any)
	uri, _ := ui["resourceUri"].(string)
	if uri != jobProgressResourceURI {
		t.Errorf("resourceUri = %q, want %q", uri, jobProgressResourceURI)
	}
}

// TestRegisterJobProgressWidget tests widget registration in ResourceStore.
func TestRegisterJobProgressWidget(t *testing.T) {
	rs := apps.NewResourceStore()
	html := "<html>job progress</html>"

	RegisterJobProgressWidget(rs, html)

	content, mime, err := rs.HandleResourcesRead(jobProgressResourceURI)
	if err != nil {
		t.Fatalf("HandleResourcesRead: %v", err)
	}
	if content != html {
		t.Errorf("content = %q, want %q", content, html)
	}
	if mime != "text/html" {
		t.Errorf("mime = %q, want text/html", mime)
	}
}

// TestRegisterJobProgressWidget_NilStore tests nil safety.
func TestRegisterJobProgressWidget_NilStore(t *testing.T) {
	// Should not panic with nil store
	RegisterJobProgressWidget(nil, "<html>test</html>")
	RegisterJobProgressWidget(apps.NewResourceStore(), "")
}

// TestRegisterGPUNativeHandlers tests that 6 tools are registered.
func TestRegisterGPUNativeHandlers_RegistersAllTools(t *testing.T) {
	reg := mcp.NewRegistry()

	RegisterGPUNativeHandlers(reg, nil, nil)

	tools := reg.ListTools()
	wantTools := []string{
		"c4_gpu_status",
		"c4_job_submit",
		"c4_job_list",
		"c4_job_status",
		"c4_job_cancel",
		"c4_job_summary",
	}

	toolMap := make(map[string]bool)
	for _, t := range tools {
		toolMap[t.Name] = true
	}

	for _, name := range wantTools {
		if !toolMap[name] {
			t.Errorf("tool %q not registered", name)
		}
	}
}
