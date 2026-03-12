package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newExperimentTestServer creates an authenticated test server for experiment tests.
func newExperimentTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	const key = "exp-test-key"
	srv := newTestServerWithKey(t, key)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		srv.Close()
	})
	return ts, key
}

func doJSON(t *testing.T, ts *httptest.Server, method, path, apiKey, body string) *http.Response {
	t.Helper()
	var req *http.Request
	if body != "" {
		req, _ = http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, ts.URL+path, nil)
	}
	req.Header.Set("X-API-Key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func TestHubExperimentAPI_CreateRun(t *testing.T) {
	ts, key := newExperimentTestServer(t)

	resp := doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"pose-exp","capability":"pose-estimation"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var got map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if got["run_id"] == "" || got["run_id"] == nil {
		t.Fatal("run_id must not be empty")
	}
	if got["status"] != "running" {
		t.Fatalf("expected running, got %v", got["status"])
	}
}

func TestHubExperimentAPI_Checkpoint(t *testing.T) {
	ts, key := newExperimentTestServer(t)

	// Create run first.
	resp := doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"exp1","capability":"cap1"}`)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	runID := created["run_id"].(string)

	// Record checkpoint.
	body := `{"run_id":"` + runID + `","metric":45.2,"path":""}`
	resp2 := doJSON(t, ts, "POST", "/v1/experiment/checkpoint", key, body)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var got map[string]any
	json.NewDecoder(resp2.Body).Decode(&got)
	if v, ok := got["is_best"].(bool); !ok || !v {
		t.Fatalf("first checkpoint must be best, got %v", got)
	}
}

func TestHubExperimentAPI_Continue(t *testing.T) {
	ts, key := newExperimentTestServer(t)

	resp := doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"exp1","capability":"cap1"}`)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	runID := created["run_id"].(string)

	resp2 := doJSON(t, ts, "GET", "/v1/experiment/continue?run_id="+runID, key, "")
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var got map[string]any
	json.NewDecoder(resp2.Body).Decode(&got)
	if v, ok := got["should_continue"].(bool); !ok || !v {
		t.Fatalf("running run should continue, got %v", got)
	}
}

func TestHubExperimentAPI_Complete(t *testing.T) {
	ts, key := newExperimentTestServer(t)

	resp := doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"exp1","capability":"cap1"}`)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	runID := created["run_id"].(string)

	body := `{"run_id":"` + runID + `","status":"success","final_metric":42.1,"summary":"done"}`
	resp2 := doJSON(t, ts, "POST", "/v1/experiment/complete", key, body)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var got map[string]any
	json.NewDecoder(resp2.Body).Decode(&got)
	if got["success"] != true {
		t.Fatalf("expected success=true, got %v", got)
	}
}

func TestHubExperimentAPI_Search(t *testing.T) {
	ts, key := newExperimentTestServer(t)

	doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"pose-exp","capability":"pose-estimation"}`).Body.Close()
	doJSON(t, ts, "POST", "/v1/experiment/run", key,
		`{"name":"depth-exp","capability":"depth-estimation"}`).Body.Close()

	resp := doJSON(t, ts, "GET", "/v1/experiment/search?query=pose&limit=10", key, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var runs []map[string]any
	json.NewDecoder(resp.Body).Decode(&runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(runs))
	}
	if runs[0]["name"] != "pose-exp" {
		t.Fatalf("expected pose-exp, got %v", runs[0]["name"])
	}
}

func TestHubExperimentAPI_Unauthorized(t *testing.T) {
	ts, _ := newExperimentTestServer(t)

	req, _ := http.NewRequest("POST", ts.URL+"/v1/experiment/run",
		strings.NewReader(`{"name":"x","capability":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
