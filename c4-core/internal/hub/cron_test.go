package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =========================================================================
// TestParseCron — basic cron expression parsing
// =========================================================================

func TestParseCron_EveryMinute(t *testing.T) {
	after := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	got, err := parseCron("* * * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_HourlyAtMinuteZero(t *testing.T) {
	after := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	got, err := parseCron("0 * * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_DailyAtMidnight(t *testing.T) {
	after := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	got, err := parseCron("0 0 * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_EveryFiveMinutes(t *testing.T) {
	after := time.Date(2024, 1, 15, 10, 7, 0, 0, time.UTC)
	got, err := parseCron("*/5 * * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_SpecificDayOfWeek(t *testing.T) {
	// Monday is weekday 1. Start from a Wednesday.
	after := time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC) // Wednesday
	got, err := parseCron("0 9 * * 1", after)             // 9am every Monday
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	// Next Monday is Jan 22 2024.
	want := time.Date(2024, 1, 22, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_SpecificMonthAndDay(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := parseCron("0 12 15 3 *", after) // noon on March 15
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_RangeAndList(t *testing.T) {
	// Minutes 10,20,30 — start at 10:05
	after := time.Date(2024, 6, 1, 10, 5, 0, 0, time.UTC)
	got, err := parseCron("10,20,30 * * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 6, 1, 10, 10, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCron_InvalidFieldCount(t *testing.T) {
	_, err := parseCron("* * * *", time.Now())
	if err == nil {
		t.Fatal("expected error for 4-field cron, got nil")
	}
}

func TestParseCron_InvalidStep(t *testing.T) {
	_, err := parseCron("*/0 * * * *", time.Now())
	if err == nil {
		t.Fatal("expected error for step=0, got nil")
	}
}

func TestParseCron_HourRange(t *testing.T) {
	// Business hours 9-17, every hour at minute 0, starting before 9am.
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	got, err := parseCron("0 9-17 * * *", after)
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	want := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// =========================================================================
// TestCronSchedulerTick — mock Supabase server
// =========================================================================

func TestCronSchedulerTick_SubmitsJob(t *testing.T) {
	now := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	// Track what was submitted.
	var submitted []map[string]any

	// Build mock HTTP server acting as Supabase PostgREST.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/v1/hub_cron_schedules":
			// Return one due schedule with a job template.
			nextRun := now.Add(-1 * time.Minute)
			schedules := []map[string]any{
				{
					"id":           "cron-1",
					"name":         "test-job",
					"cron_expr":    "*/5 * * * *",
					"job_template": `{"name":"test-job","command":"echo hello","workdir":"/tmp"}`,
					"dag_id":       "",
					"enabled":      true,
					"next_run":     nextRun.Format(time.RFC3339),
					"project_id":   "proj-1",
					"created_at":   now.Add(-time.Hour).Format(time.RFC3339),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(schedules)

		case r.Method == "POST" && r.URL.Path == "/rest/v1/hub_jobs":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			submitted = append(submitted, body)
			// Return a minimal job row.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "job-123", "status": "QUEUED"},
			})

		case r.Method == "PATCH" && r.URL.Path == "/rest/v1/hub_cron_schedules":
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestCronClient(srv.URL)
	scheduler := NewCronScheduler(client, nil)

	scheduler.tick(context.Background())

	if len(submitted) != 1 {
		t.Fatalf("expected 1 job submitted, got %d", len(submitted))
	}
	if submitted[0]["command"] != "echo hello" {
		t.Errorf("command = %v, want echo hello", submitted[0]["command"])
	}
}

func TestCronSchedulerTick_ExecutesDAG(t *testing.T) {
	now := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	var dagPatched []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/v1/hub_cron_schedules":
			nextRun := now.Add(-1 * time.Minute)
			schedules := []map[string]any{
				{
					"id":           "cron-2",
					"name":         "dag-trigger",
					"cron_expr":    "0 * * * *",
					"job_template": "{}",
					"dag_id":       "dag-abc",
					"enabled":      true,
					"next_run":     nextRun.Format(time.RFC3339),
					"project_id":   "proj-1",
					"created_at":   now.Add(-time.Hour).Format(time.RFC3339),
				},
			}
			json.NewEncoder(w).Encode(schedules)

		case r.Method == "GET" && r.URL.Path == "/rest/v1/hub_dag_nodes":
			// Return one root node so ExecuteDAG can proceed.
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "node-1", "name": "step1", "command": "echo dag", "working_dir": "/tmp"},
			})

		case r.Method == "GET" && r.URL.Path == "/rest/v1/hub_dag_dependencies":
			json.NewEncoder(w).Encode([]any{})

		case r.Method == "POST" && r.URL.Path == "/rest/v1/hub_jobs":
			// Acknowledge root node job submission.
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "job-dag-1", "status": "QUEUED"},
			})

		case r.Method == "PATCH" && r.URL.Path == "/rest/v1/hub_dag_nodes":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "PATCH" && r.URL.Path == "/rest/v1/hub_dags":
			dagPatched = append(dagPatched, r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "PATCH" && r.URL.Path == "/rest/v1/hub_cron_schedules":
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestCronClient(srv.URL)
	scheduler := NewCronScheduler(client, nil)

	scheduler.tick(context.Background())

	if len(dagPatched) != 1 {
		t.Fatalf("expected 1 DAG patch, got %d", len(dagPatched))
	}
}

func TestCronSchedulerTick_NoSchedules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]any{})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	client := newTestCronClient(srv.URL)
	scheduler := NewCronScheduler(client, nil)

	// Should not panic or make any write calls.
	scheduler.tick(context.Background())
}

// newTestCronClient creates a minimal Client pointing at a test server URL.
func newTestCronClient(baseURL string) *Client {
	return &Client{
		supabaseURL: baseURL,
		supabaseKey: "test-key",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}
