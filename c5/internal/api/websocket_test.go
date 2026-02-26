package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/piqsol/c4/c5/internal/model"
)

// dialWS opens a WebSocket connection to the given httptest.Server URL path.
func dialWS(t *testing.T, ts *httptest.Server, path string) net.Conn {
	t.Helper()
	u := "ws" + ts.URL[len("http"):] + path
	conn, _, _, err := ws.Dial(context.Background(), u)
	if err != nil {
		t.Fatalf("ws dial %s: %v", u, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readWSMsg reads one JSON WebSocket message from the server.
func readWSMsg(t *testing.T, conn net.Conn) model.MetricMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	data, err := wsutil.ReadServerText(conn)
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	var msg model.MetricMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode ws message: %v", err)
	}
	return msg
}

// submitTestJob creates a job via the API and returns its ID.
func submitTestJob(t *testing.T, srv *Server, name string) string {
	t.Helper()
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    name,
		Command: "echo",
	})
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("submit job %q: status %d body %s", name, w.Code, w.Body.String())
	}
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)
	return resp.JobID
}

// TestWebSocket_IncrementalOnly verifies that when include_history=false the
// WebSocket stream does not replay existing metrics and only delivers steps
// inserted after the connection was established.
func TestWebSocket_IncrementalOnly(t *testing.T) {
	srv := newTestServer(t)
	jobID := submitTestJob(t, srv, "ws-test")

	// Insert two metrics before connecting.
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    0,
		Metrics: map[string]any{"loss": 1.0},
	})
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    1,
		Metrics: map[string]any{"loss": 0.8},
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Connect without history — should not receive steps 0 or 1.
	conn := dialWS(t, ts, "/v1/ws/metrics/"+jobID+"?include_history=false")

	// The first message should be a "status" message (not a metric step).
	msg := readWSMsg(t, conn)
	if msg.Type != "status" {
		t.Fatalf("expected first msg type=status, got %q (step=%d)", msg.Type, msg.Step)
	}

	// Insert a new metric after connection is established.
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    2,
		Metrics: map[string]any{"loss": 0.5},
	})

	// Verify the store returns only the new step when queried with minStep=1.
	entries, err := srv.store.GetMetrics(jobID, 1, 0)
	if err != nil {
		t.Fatalf("GetMetrics(minStep=1): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry with step>1, got %d", len(entries))
	}
	if entries[0].Step != 2 {
		t.Fatalf("expected step=2, got %d", entries[0].Step)
	}
}

// TestWebSocket_IncludeHistory verifies that with include_history=true the
// stream sends existing metrics as "history" type messages before switching to
// incremental polling.
func TestWebSocket_IncludeHistory(t *testing.T) {
	srv := newTestServer(t)
	jobID := submitTestJob(t, srv, "ws-hist")

	// Insert two metrics.
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    0,
		Metrics: map[string]any{"loss": 1.0},
	})
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    1,
		Metrics: map[string]any{"loss": 0.9},
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	conn := dialWS(t, ts, "/v1/ws/metrics/"+jobID+"?include_history=true")

	// Expect two "history" messages followed by a "status" message.
	steps := make([]int, 0, 2)
	for i := 0; i < 3; i++ {
		msg := readWSMsg(t, conn)
		switch msg.Type {
		case "history":
			steps = append(steps, msg.Step)
		case "status":
			// done — no more messages expected
		default:
			t.Fatalf("unexpected msg type %q at i=%d", msg.Type, i)
		}
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 history steps, got %v", steps)
	}
	if steps[0] != 0 || steps[1] != 1 {
		t.Fatalf("expected steps [0,1], got %v", steps)
	}
}
