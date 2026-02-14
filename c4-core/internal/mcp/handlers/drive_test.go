package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/mcp"
)

// newDriveTestServer creates an httptest server simulating Supabase for drive tests.
func newDriveTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	storedFiles := make(map[string][]byte)
	metadataRows := make([]map[string]any, 0)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		body, _ := io.ReadAll(r.Body)
		storedFiles[storagePath] = body
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"Key": storagePath})
	})

	mux.HandleFunc("GET /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		data, ok := storedFiles[storagePath]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Write(data)
	})

	mux.HandleFunc("DELETE /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		delete(storedFiles, storagePath)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		row["id"] = "uuid-" + row["path"].(string)
		row["created_at"] = "2026-02-14T00:00:00Z"
		row["updated_at"] = "2026-02-14T00:00:00Z"
		found := false
		for i, existing := range metadataRows {
			if existing["path"] == row["path"] && existing["project_id"] == row["project_id"] {
				metadataRows[i] = row
				found = true
				break
			}
		}
		if !found {
			metadataRows = append(metadataRows, row)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode([]map[string]any{row})
	})

	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		var result []map[string]any
		for _, row := range metadataRows {
			match := true
			if strings.Contains(query, "path=eq.") {
				idx := strings.Index(query, "path=eq.")
				val := query[idx+8:]
				if amp := strings.Index(val, "&"); amp >= 0 {
					val = val[:amp]
				}
				if row["path"] != val {
					match = false
				}
			}
			if match {
				result = append(result, row)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("DELETE /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		newRows := make([]map[string]any, 0)
		for _, row := range metadataRows {
			idx := strings.Index(query, "path=eq.")
			if idx >= 0 {
				val := query[idx+8:]
				if amp := strings.Index(val, "&"); amp >= 0 {
					val = val[:amp]
				}
				if row["path"] != val {
					newRows = append(newRows, row)
				}
			}
		}
		metadataRows = newRows
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

func TestRegisterDriveHandlers(t *testing.T) {
	srv := newDriveTestServer(t)
	defer srv.Close()

	reg := mcp.NewRegistry()
	client := drive.NewClient(srv.URL, "test-key", "test-token", "test-proj")
	RegisterDriveHandlers(reg, client)

	// Verify all 5 tools are registered
	tools := reg.ListTools()
	driveTools := make(map[string]bool)
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, "c4_drive_") {
			driveTools[tool.Name] = true
		}
	}

	expected := []string{
		"c4_drive_upload",
		"c4_drive_download",
		"c4_drive_list",
		"c4_drive_delete",
		"c4_drive_info",
	}
	for _, name := range expected {
		if !driveTools[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestDriveUploadHandler(t *testing.T) {
	srv := newDriveTestServer(t)
	defer srv.Close()

	reg := mcp.NewRegistry()
	client := drive.NewClient(srv.URL, "test-key", "test-token", "test-proj")
	RegisterDriveHandlers(reg, client)

	// Create test file
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(srcPath, []byte("handler test"), 0o644)

	args, _ := json.Marshal(map[string]string{
		"local_path": srcPath,
		"drive_path": "/handler-test.txt",
	})
	result, err := reg.Call("c4_drive_upload", args)
	if err != nil {
		t.Fatalf("c4_drive_upload failed: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["status"] != "uploaded" {
		t.Errorf("status = %v, want uploaded", m["status"])
	}
	if m["name"] != "handler-test.txt" {
		t.Errorf("name = %v, want handler-test.txt", m["name"])
	}
}

func TestDriveInfoHandler(t *testing.T) {
	srv := newDriveTestServer(t)
	defer srv.Close()

	reg := mcp.NewRegistry()
	client := drive.NewClient(srv.URL, "test-key", "test-token", "test-proj")
	RegisterDriveHandlers(reg, client)

	// Upload first
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "info.txt")
	os.WriteFile(srcPath, []byte("info data"), 0o644)

	uploadArgs, _ := json.Marshal(map[string]string{
		"local_path": srcPath,
		"drive_path": "/info.txt",
	})
	if _, err := reg.Call("c4_drive_upload", uploadArgs); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// Get info
	infoArgs, _ := json.Marshal(map[string]string{"path": "/info.txt"})
	result, err := reg.Call("c4_drive_info", infoArgs)
	if err != nil {
		t.Fatalf("c4_drive_info failed: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["name"] != "info.txt" {
		t.Errorf("name = %v, want info.txt", m["name"])
	}
	if m["is_folder"] != false {
		t.Errorf("is_folder = %v, want false", m["is_folder"])
	}
}

func TestDriveListHandler(t *testing.T) {
	srv := newDriveTestServer(t)
	defer srv.Close()

	reg := mcp.NewRegistry()
	client := drive.NewClient(srv.URL, "test-key", "test-token", "test-proj")
	RegisterDriveHandlers(reg, client)

	// List empty
	listArgs, _ := json.Marshal(map[string]string{"path": "/"})
	result, err := reg.Call("c4_drive_list", listArgs)
	if err != nil {
		t.Fatalf("c4_drive_list failed: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["folder"] != "/" {
		t.Errorf("folder = %v, want /", m["folder"])
	}
}

func TestDriveDeleteHandler(t *testing.T) {
	srv := newDriveTestServer(t)
	defer srv.Close()

	reg := mcp.NewRegistry()
	client := drive.NewClient(srv.URL, "test-key", "test-token", "test-proj")
	RegisterDriveHandlers(reg, client)

	// Upload then delete
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "del.txt")
	os.WriteFile(srcPath, []byte("delete me"), 0o644)

	uploadArgs, _ := json.Marshal(map[string]string{
		"local_path": srcPath,
		"drive_path": "/del.txt",
	})
	if _, err := reg.Call("c4_drive_upload", uploadArgs); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	delArgs, _ := json.Marshal(map[string]string{"path": "/del.txt"})
	result, err := reg.Call("c4_drive_delete", delArgs)
	if err != nil {
		t.Fatalf("c4_drive_delete failed: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["status"] != "deleted" {
		t.Errorf("status = %v, want deleted", m["status"])
	}
}
