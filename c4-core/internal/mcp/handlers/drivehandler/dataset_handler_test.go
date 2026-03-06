//go:build c0_drive

package drivehandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/mcp"
)

// mockToken satisfies the tokenProvider interface for tests.
type mockToken struct{}

func (m *mockToken) Token() string { return "test-token" }

// newTestDatasetClient creates a DatasetClient backed by a test HTTP server.
func newTestDatasetClient(t *testing.T, handler http.HandlerFunc) (*drive.DatasetClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := drive.NewClient(srv.URL, "test-key", &mockToken{}, "proj-test")
	return drive.NewDatasetClient(c), srv
}

// TestRegisterDatasetHandlers verifies all 3 dataset tools are registered.
func TestRegisterDatasetHandlers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]")) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	reg := mcp.NewRegistry()
	c := drive.NewClient(srv.URL, "key", &mockToken{}, "proj")
	RegisterDatasetHandlers(reg, drive.NewDatasetClient(c))

	tools := []string{"c4_drive_dataset_upload", "c4_drive_dataset_list", "c4_drive_dataset_pull"}
	for _, name := range tools {
		if !reg.HasTool(name) {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// TestDatasetHandlerList_Empty verifies that the list handler returns an empty
// array when the server reports no versions.
func TestDatasetHandlerList_Empty(t *testing.T) {
	client, _ := newTestDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]")) //nolint:errcheck
	})

	reg := mcp.NewRegistry()
	RegisterDatasetHandlers(reg, client)

	raw, _ := json.Marshal(map[string]any{"name": "mydata"})
	result, err := reg.Call("c4_drive_dataset_list", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	versions, ok := result.([]drive.DatasetVersion)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// TestDatasetHandlerUpload_MissingArgs verifies that missing required args returns an error.
func TestDatasetHandlerUpload_MissingArgs(t *testing.T) {
	client, _ := newTestDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	reg := mcp.NewRegistry()
	RegisterDatasetHandlers(reg, client)

	// Missing both path and name.
	raw, _ := json.Marshal(map[string]any{})
	_, err := reg.Call("c4_drive_dataset_upload", raw)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

// TestDatasetHandlerPull_MissingName verifies that missing name returns an error.
func TestDatasetHandlerPull_MissingName(t *testing.T) {
	client, _ := newTestDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	reg := mcp.NewRegistry()
	RegisterDatasetHandlers(reg, client)

	raw, _ := json.Marshal(map[string]any{"dest": "/tmp/out"})
	_, err := reg.Call("c4_drive_dataset_pull", raw)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// TestDatasetHandlerUpload_ChangedTrue verifies that uploading a new directory
// returns Changed=true when no previous version exists.
func TestDatasetHandlerUpload_ChangedTrue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// No existing version.
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("[]")) //nolint:errcheck
		case http.MethodHead:
			// CAS object doesn't exist → upload needed.
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPost:
			// Both storage upload and metadata insert succeed.
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("[]")) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	c := drive.NewClient(srv.URL, "key", &mockToken{}, "proj")
	client := drive.NewDatasetClient(c)

	reg := mcp.NewRegistry()
	RegisterDatasetHandlers(reg, client)

	raw, _ := json.Marshal(map[string]any{"path": dir, "name": "testds"})
	result, err := reg.Call("c4_drive_dataset_upload", raw)
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}
	ur, ok := result.(*drive.DatasetUploadResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if !ur.Changed {
		t.Errorf("expected Changed=true for new upload")
	}
}
