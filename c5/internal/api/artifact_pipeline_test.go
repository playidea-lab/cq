package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/storage"
	"github.com/piqsol/c4/c5/internal/store"
)

// TestArtifactPipelineE2E exercises the full artifact pipeline:
// submit job (with input+output artifacts) → worker register → lease acquire
// (presigned GET URLs) → verify download URL → get presigned upload URL →
// write output → confirm artifact → complete job → verify final state.
func TestArtifactPipelineE2E(t *testing.T) {
	// 1. Setup: temp dirs, SQLite store, LocalBackend storage, test server.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	storageDir := filepath.Join(tmpDir, "artifacts")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// LocalBackend needs a baseURL for download links. We use a placeholder
	// here and replace it after starting the httptest server. Since we use
	// the internal doRequest pattern (httptest.NewRequest → srv.Handler.ServeHTTP),
	// the actual URL doesn't matter for routing — we just verify the shape.
	local := storage.NewLocal(storageDir, "http://localhost:9999")

	srv := NewServer(Config{
		Store:   st,
		Storage: local,
		Version: "test",
	})
	t.Cleanup(func() { srv.Close() })

	// 2. Create a test input file in the local storage directory.
	inputDir := filepath.Join(storageDir, "inputs")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	inputContent := []byte("hello input")
	if err := os.WriteFile(filepath.Join(inputDir, "data.txt"), inputContent, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	// 3. Submit a job with input and output artifacts.
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "artifact-pipeline-test",
		Command: "cp $C5_INPUT_DIR/data.txt output.txt && echo done",
		Workdir: "/tmp/c5-test-workdir",
		InputArtifacts: []model.ArtifactRef{
			{Path: "inputs/data.txt", LocalPath: "data.txt", Required: true},
		},
		OutputArtifacts: []model.ArtifactRef{
			{Path: "outputs/result.txt", LocalPath: "output.txt", Required: true},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var submitResp model.JobSubmitResponse
	decodeJSON(t, w, &submitResp)
	jobID := submitResp.JobID
	if jobID == "" {
		t.Fatal("job_id should not be empty")
	}
	if submitResp.Status != "QUEUED" {
		t.Fatalf("expected QUEUED, got %s", submitResp.Status)
	}

	// Verify job has artifacts persisted.
	wg := doRequest(t, srv, "GET", "/v1/jobs/"+jobID, nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("get job: expected 200, got %d", wg.Code)
	}
	var job model.Job
	decodeJSON(t, wg, &job)
	if len(job.InputArtifacts) != 1 {
		t.Fatalf("expected 1 input artifact, got %d", len(job.InputArtifacts))
	}
	if job.InputArtifacts[0].Path != "inputs/data.txt" {
		t.Fatalf("input artifact path mismatch: %s", job.InputArtifacts[0].Path)
	}
	if len(job.OutputArtifacts) != 1 {
		t.Fatalf("expected 1 output artifact, got %d", len(job.OutputArtifacts))
	}
	if job.OutputArtifacts[0].Path != "outputs/result.txt" {
		t.Fatalf("output artifact path mismatch: %s", job.OutputArtifacts[0].Path)
	}

	// 4. Register a worker.
	ww := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "pipeline-worker",
	})
	if ww.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", ww.Code)
	}
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, ww, &regResp)
	workerID := regResp.WorkerID

	// 5. Acquire lease — verify presigned input URLs are returned.
	wl := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: workerID,
	})
	if wl.Code != http.StatusOK {
		t.Fatalf("acquire: expected 200, got %d: %s", wl.Code, wl.Body.String())
	}
	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, wl, &acqResp)

	if acqResp.JobID != jobID {
		t.Fatalf("acquired wrong job: expected %s, got %s", jobID, acqResp.JobID)
	}
	if acqResp.LeaseID == "" {
		t.Fatal("lease_id should not be empty")
	}
	if len(acqResp.InputPresignedURLs) != 1 {
		t.Fatalf("expected 1 input presigned URL, got %d", len(acqResp.InputPresignedURLs))
	}

	presigned := acqResp.InputPresignedURLs[0]
	if presigned.Path != "inputs/data.txt" {
		t.Fatalf("presigned path mismatch: %s", presigned.Path)
	}
	if presigned.LocalPath != "data.txt" {
		t.Fatalf("presigned local_path mismatch: %s", presigned.LocalPath)
	}
	if presigned.URL == "" {
		t.Fatal("presigned URL should not be empty")
	}
	// LocalBackend GET URLs have the form: http://localhost:9999/v1/storage/download/inputs/data.txt
	if !strings.Contains(presigned.URL, "/v1/storage/download/inputs/data.txt") {
		t.Fatalf("presigned URL unexpected format: %s", presigned.URL)
	}

	// 6. Download input artifact via HTTP GET through the storage download handler.
	wdl := httptest.NewRecorder()
	dlReq := httptest.NewRequest("GET", "/v1/storage/download/inputs/data.txt", nil)
	srv.Handler().ServeHTTP(wdl, dlReq)
	if wdl.Code != http.StatusOK {
		t.Fatalf("GET download: expected 200, got %d: %s", wdl.Code, wdl.Body.String())
	}
	if wdl.Body.String() != "hello input" {
		t.Fatalf("download content mismatch: got %q", wdl.Body.String())
	}

	// 7. Get presigned upload URL for output artifact.
	wpu := doRequest(t, srv, "POST", "/v1/storage/presigned-url", model.PresignedURLRequest{
		Path:   "outputs/result.txt",
		Method: "PUT",
	})
	if wpu.Code != http.StatusOK {
		t.Fatalf("presigned PUT URL: expected 200, got %d: %s", wpu.Code, wpu.Body.String())
	}
	var putResp model.PresignedURLResponse
	decodeJSON(t, wpu, &putResp)
	if putResp.URL == "" {
		t.Fatal("PUT presigned URL should not be empty")
	}
	// LocalBackend PUT URLs have the form: http://{baseURL}/v1/storage/upload/{path}
	if !strings.Contains(putResp.URL, "/v1/storage/upload/") {
		t.Fatalf("expected /v1/storage/upload/ URL for LocalBackend PUT, got: %s", putResp.URL)
	}
	if putResp.ExpiresAt == "" {
		t.Fatal("expires_at should not be empty")
	}

	// 8. Upload output artifact via HTTP PUT to the storage upload endpoint.
	outputContent := []byte("hello output")
	uploadPath := "/v1/storage/upload/outputs/result.txt"
	wup := httptest.NewRecorder()
	uploadReq := httptest.NewRequest("PUT", uploadPath, bytes.NewReader(outputContent))
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	srv.Handler().ServeHTTP(wup, uploadReq)
	if wup.Code != http.StatusOK {
		t.Fatalf("PUT upload: expected 200, got %d: %s", wup.Code, wup.Body.String())
	}

	// 9. Confirm artifact.
	wca := doRequest(t, srv, "POST", "/v1/artifacts/"+jobID+"/confirm", model.ArtifactConfirmRequest{
		Path:        "outputs/result.txt",
		ContentHash: "sha256:abc123def456",
		SizeBytes:   int64(len(outputContent)),
	})
	if wca.Code != http.StatusOK {
		t.Fatalf("confirm artifact: expected 200, got %d: %s", wca.Code, wca.Body.String())
	}
	var confirmResp model.ArtifactConfirmResponse
	decodeJSON(t, wca, &confirmResp)
	if !confirmResp.Confirmed {
		t.Fatal("expected confirmed=true")
	}
	if confirmResp.ArtifactID == "" {
		t.Fatal("artifact_id should not be empty")
	}

	// 10. Complete the job.
	exitZero := 0
	wc := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete", model.JobCompleteRequest{
		Status:   "SUCCEEDED",
		ExitCode: &exitZero,
	})
	if wc.Code != http.StatusOK {
		t.Fatalf("complete: expected 200, got %d: %s", wc.Code, wc.Body.String())
	}

	// 11. Verify final state.

	// 11a. Artifact list includes the confirmed output.
	wa := doRequest(t, srv, "GET", "/v1/artifacts/"+jobID, nil)
	if wa.Code != http.StatusOK {
		t.Fatalf("list artifacts: expected 200, got %d", wa.Code)
	}
	var artifacts []model.Artifact
	decodeJSON(t, wa, &artifacts)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Path != "outputs/result.txt" {
		t.Fatalf("artifact path mismatch: %s", artifacts[0].Path)
	}
	if !artifacts[0].Confirmed {
		t.Fatal("artifact should be confirmed")
	}
	if artifacts[0].ContentHash != "sha256:abc123def456" {
		t.Fatalf("content_hash mismatch: %s", artifacts[0].ContentHash)
	}
	if artifacts[0].SizeBytes != int64(len(outputContent)) {
		t.Fatalf("size_bytes mismatch: %d", artifacts[0].SizeBytes)
	}

	// 11b. Job status is SUCCEEDED.
	wf := doRequest(t, srv, "GET", "/v1/jobs/"+jobID, nil)
	if wf.Code != http.StatusOK {
		t.Fatalf("get final job: expected 200, got %d", wf.Code)
	}
	var finalJob model.Job
	decodeJSON(t, wf, &finalJob)
	if finalJob.Status != model.StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", finalJob.Status)
	}
	if finalJob.ExitCode == nil || *finalJob.ExitCode != 0 {
		t.Fatalf("expected exit_code=0, got %v", finalJob.ExitCode)
	}

	// 11c. Summary shows correct info.
	ws := doRequest(t, srv, "GET", "/v1/jobs/"+jobID+"/summary", nil)
	if ws.Code != http.StatusOK {
		t.Fatalf("summary: expected 200, got %d", ws.Code)
	}
	var summary model.JobSummaryResponse
	decodeJSON(t, ws, &summary)
	if summary.Status != "SUCCEEDED" {
		t.Fatalf("summary status: %s", summary.Status)
	}
	if summary.Name != "artifact-pipeline-test" {
		t.Fatalf("summary name: %s", summary.Name)
	}
}
