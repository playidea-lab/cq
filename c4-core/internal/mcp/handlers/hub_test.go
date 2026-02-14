package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/changmin/c4-core/internal/hub"
)

// newTestHubClient creates a hub.Client pointed at an httptest server.
func newTestHubClient(handler http.HandlerFunc) (*hub.Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	client := hub.NewClient(hub.HubConfig{
		Enabled:   true,
		URL:       srv.URL,
		APIPrefix: "",
	})
	return client, srv
}

// --- handleHubSubmit ---

func TestHandleHubSubmit_Success(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/submit" {
			http.Error(w, "not found", 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"job_id":         "j-001",
			"status":         "QUEUED",
			"queue_position": 1,
		})
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{
		"name":    "test-job",
		"workdir": "/tmp",
		"command": "echo hello",
	})

	result, err := handleHubSubmit(client, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["job_id"] != "j-001" {
		t.Errorf("job_id = %v", m["job_id"])
	}
	if m["status"] != "QUEUED" {
		t.Errorf("status = %v", m["status"])
	}
}

func TestHandleHubSubmit_MissingFields(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()

	// Missing required fields
	args, _ := json.Marshal(map[string]any{"name": "test"})
	_, err := handleHubSubmit(client, args)
	if err == nil {
		t.Error("expected error for missing fields")
	}
}

func TestHandleHubSubmit_InvalidJSON(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()

	_, err := handleHubSubmit(client, json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- handleHubStatus ---

func TestHandleHubStatus_Success(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs/j-001" {
			json.NewEncoder(w).Encode(hub.Job{
				ID:     "j-001",
				Name:   "test-job",
				Status: "RUNNING",
			})
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]string{"job_id": "j-001"})
	result, err := handleHubStatus(client, args)
	if err != nil {
		t.Fatal(err)
	}
	job := result.(*hub.Job)
	if job.Status != "RUNNING" {
		t.Errorf("status = %v", job.Status)
	}
}

func TestHandleHubStatus_MissingJobID(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()

	args, _ := json.Marshal(map[string]string{})
	_, err := handleHubStatus(client, args)
	if err == nil {
		t.Error("expected error for missing job_id")
	}
}

func TestHandleHubStatus_NotFound(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]string{"job_id": "nope"})
	_, err := handleHubStatus(client, args)
	if err == nil {
		t.Error("expected error for 404")
	}
}

// --- handleHubList ---

func TestHandleHubList_Success(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs" {
			json.NewEncoder(w).Encode([]hub.Job{
				{ID: "j-001", Status: "QUEUED"},
				{ID: "j-002", Status: "RUNNING"},
			})
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"limit": 10})
	result, err := handleHubList(client, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 2 {
		t.Errorf("count = %v", m["count"])
	}
}

func TestHandleHubList_WithStatusFilter(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "QUEUED" {
			json.NewEncoder(w).Encode([]hub.Job{{ID: "j-001", Status: "QUEUED"}})
		} else {
			json.NewEncoder(w).Encode([]hub.Job{})
		}
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"status": "QUEUED"})
	result, err := handleHubList(client, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 1 {
		t.Errorf("count = %v", m["count"])
	}
}

// --- handleHubCancel ---

func TestHandleHubCancel_Success(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/jobs/j-001/cancel" {
			json.NewEncoder(w).Encode(map[string]any{"cancelled": true})
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	args, _ := json.Marshal(map[string]string{"job_id": "j-001"})
	result, err := handleHubCancel(client, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if !m["cancelled"].(bool) {
		t.Error("expected cancelled=true")
	}
}

func TestHandleHubCancel_MissingJobID(t *testing.T) {
	client, srv := newTestHubClient(func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()

	args, _ := json.Marshal(map[string]string{})
	_, err := handleHubCancel(client, args)
	if err == nil {
		t.Error("expected error for missing job_id")
	}
}
