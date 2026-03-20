package affinity

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTestDB opens an in-memory SQLite database for testing.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestStore creates a Store backed by an in-memory SQLite database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := openTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// TestInitSchema verifies the worker_affinity table is created.
func TestInitSchema(t *testing.T) {
	db := openTestDB(t)
	s := &Store{db: db}
	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}
	// Calling again must be idempotent (IF NOT EXISTS).
	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema() second call error = %v", err)
	}

	// Verify the table exists by inserting a row.
	_, err := db.Exec(`INSERT INTO worker_affinity (worker_id, project_id) VALUES ('w1', 'p1')`)
	if err != nil {
		t.Fatalf("table not usable after InitSchema: %v", err)
	}
}

// TestRecordSuccess_FirstCall inserts a new record on first success.
func TestRecordSuccess_FirstCall(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordSuccess("w1", "p1", []string{"go", "linux"}); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	records, err := s.GetWorkerAffinity("w1")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.WorkerID != "w1" {
		t.Errorf("WorkerID = %q, want w1", r.WorkerID)
	}
	if r.ProjectID != "p1" {
		t.Errorf("ProjectID = %q, want p1", r.ProjectID)
	}
	if r.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", r.SuccessCount)
	}
	if r.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0", r.FailCount)
	}
	if r.LastSuccess.IsZero() {
		t.Error("LastSuccess should not be zero after first success")
	}
	if len(r.Tags) != 2 {
		t.Errorf("Tags = %v, want [go linux]", r.Tags)
	}
}

// TestRecordSuccess_Accumulates verifies success_count increments on repeated calls.
func TestRecordSuccess_Accumulates(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		if err := s.RecordSuccess("w1", "p1", []string{"go"}); err != nil {
			t.Fatalf("RecordSuccess #%d: %v", i, err)
		}
	}

	records, err := s.GetWorkerAffinity("w1")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if records[0].SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", records[0].SuccessCount)
	}
}

// TestRecordFailure_FirstCall inserts a new record on first failure.
func TestRecordFailure_FirstCall(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordFailure("w1", "p1"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	records, err := s.GetWorkerAffinity("w1")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", r.FailCount)
	}
	if r.SuccessCount != 0 {
		t.Errorf("SuccessCount = %d, want 0", r.SuccessCount)
	}
}

// TestRecordFailure_Accumulates verifies fail_count increments on repeated calls.
func TestRecordFailure_Accumulates(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		if err := s.RecordFailure("w1", "p1"); err != nil {
			t.Fatalf("RecordFailure #%d: %v", i, err)
		}
	}

	records, err := s.GetWorkerAffinity("w1")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if records[0].FailCount != 5 {
		t.Errorf("FailCount = %d, want 5", records[0].FailCount)
	}
}

// TestRecordSuccess_ThenFailure verifies mixed UPSERT behaviour.
func TestRecordSuccess_ThenFailure(t *testing.T) {
	s := newTestStore(t)
	_ = s.RecordSuccess("w1", "p1", []string{"go"})
	_ = s.RecordSuccess("w1", "p1", []string{"go"})
	_ = s.RecordFailure("w1", "p1")

	records, _ := s.GetWorkerAffinity("w1")
	r := records[0]
	if r.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", r.SuccessCount)
	}
	if r.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", r.FailCount)
	}
}

// TestScore_NoRecord returns 0 for an unknown worker/project pair.
func TestScore_NoRecord(t *testing.T) {
	s := newTestStore(t)
	score, err := s.Score("unknown", "p1", []string{"go"})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if score != 0 {
		t.Errorf("Score = %f, want 0 for unknown worker", score)
	}
}

// TestScore_PerfectRate verifies the formula for a worker with 100% success rate
// and recent activity and matching tags.
func TestScore_PerfectRate(t *testing.T) {
	s := newTestStore(t)
	// Insert directly so we can control last_success precisely.
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO worker_affinity (worker_id, project_id, success_count, fail_count, last_success, tags)
		VALUES ('w1', 'p1', 4, 0, ?, '["go","linux"]')`, now)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// project_match = 4×10 = 40
	// tag_overlap   = 2×3  = 6  (both "go" and "linux" match)
	// recency_bonus = 1×2  = 2  (within 7 days)
	// success_rate  = (4/4)×5 = 5
	// total = 53
	score, err := s.Score("w1", "p1", []string{"go", "linux"})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	const want float64 = 40 + 6 + 2 + 5 // 53
	if score != want {
		t.Errorf("Score = %f, want %v", score, want)
	}
}

// TestScore_OldLastSuccess verifies no recency bonus for stale records.
func TestScore_OldLastSuccess(t *testing.T) {
	s := newTestStore(t)
	old := time.Now().UTC().Add(-8 * 24 * time.Hour) // 8 days ago
	_, err := s.db.Exec(`
		INSERT INTO worker_affinity (worker_id, project_id, success_count, fail_count, last_success, tags)
		VALUES ('w1', 'p1', 2, 0, ?, '[]')`, old)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// project_match = 2×10 = 20
	// tag_overlap   = 0
	// recency_bonus = 0   (older than 7 days)
	// success_rate  = (2/2)×5 = 5
	// total = 25
	score, err := s.Score("w1", "p1", []string{"go"})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	const want = 25.0
	if score != want {
		t.Errorf("Score = %f, want %v", score, want)
	}
}

// TestScore_MixedSuccessRate verifies fractional success rate.
func TestScore_MixedSuccessRate(t *testing.T) {
	s := newTestStore(t)
	_ = s.RecordSuccess("w1", "p1", nil)  // success_count=1
	_ = s.RecordSuccess("w1", "p1", nil)  // success_count=2
	_ = s.RecordFailure("w1", "p1")       // fail_count=1
	_ = s.RecordFailure("w1", "p1")       // fail_count=2

	// success_rate = 2/4 = 0.5 → ×5 = 2.5
	// project_match = 2×10 = 20
	// recency within 7 days → +2
	// tag_overlap = 0 (no required tags)
	// total = 24.5
	score, err := s.Score("w1", "p1", nil)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	const want = 24.5
	if score != want {
		t.Errorf("Score = %f, want %f", score, want)
	}
}

// TestRankWorkers_EmptyCandidates returns nil for an empty candidate list.
func TestRankWorkers_EmptyCandidates(t *testing.T) {
	s := newTestStore(t)
	result, err := s.RankWorkers("p1", nil, nil)
	if err != nil {
		t.Fatalf("RankWorkers: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty candidates, got %v", result)
	}
}

// TestRankWorkers_Ordering verifies higher-scoring workers appear first.
func TestRankWorkers_Ordering(t *testing.T) {
	s := newTestStore(t)

	// w1: 3 successes, 0 failures, recent
	for i := 0; i < 3; i++ {
		_ = s.RecordSuccess("w1", "p1", []string{"go"})
	}
	// w2: 1 success, 0 failures, recent
	_ = s.RecordSuccess("w2", "p1", []string{"go"})
	// w3: no record → score 0

	ranked, err := s.RankWorkers("p1", []string{"go"}, []string{"w1", "w2", "w3"})
	if err != nil {
		t.Fatalf("RankWorkers: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 results, got %d", len(ranked))
	}
	if ranked[0].WorkerID != "w1" {
		t.Errorf("ranked[0] = %q, want w1", ranked[0].WorkerID)
	}
	if ranked[1].WorkerID != "w2" {
		t.Errorf("ranked[1] = %q, want w2", ranked[1].WorkerID)
	}
	if ranked[2].WorkerID != "w3" {
		t.Errorf("ranked[2] = %q, want w3", ranked[2].WorkerID)
	}
	if ranked[2].Score != 0 {
		t.Errorf("w3 score = %f, want 0", ranked[2].Score)
	}
}

// TestRankWorkers_TiebreakByID verifies deterministic ordering for equal scores.
func TestRankWorkers_TiebreakByID(t *testing.T) {
	s := newTestStore(t)
	// Both workers have identical history.
	_ = s.RecordSuccess("wb", "p1", []string{"go"})
	_ = s.RecordSuccess("wa", "p1", []string{"go"})

	ranked, err := s.RankWorkers("p1", []string{"go"}, []string{"wb", "wa"})
	if err != nil {
		t.Fatalf("RankWorkers: %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ranked))
	}
	// Equal scores → alphabetical by workerID.
	if ranked[0].WorkerID != "wa" {
		t.Errorf("ranked[0] = %q, want wa (alphabetical tiebreak)", ranked[0].WorkerID)
	}
}

// TestGetWorkerAffinity_MultipleProjects returns one record per project.
func TestGetWorkerAffinity_MultipleProjects(t *testing.T) {
	s := newTestStore(t)
	_ = s.RecordSuccess("w1", "proj-a", []string{"go"})
	_ = s.RecordSuccess("w1", "proj-b", []string{"python"})
	_ = s.RecordFailure("w1", "proj-c")

	records, err := s.GetWorkerAffinity("w1")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	// Records are ordered by project_id.
	if records[0].ProjectID != "proj-a" {
		t.Errorf("records[0].ProjectID = %q, want proj-a", records[0].ProjectID)
	}
	if records[1].ProjectID != "proj-b" {
		t.Errorf("records[1].ProjectID = %q, want proj-b", records[1].ProjectID)
	}
	if records[2].ProjectID != "proj-c" {
		t.Errorf("records[2].ProjectID = %q, want proj-c", records[2].ProjectID)
	}
}

// TestGetWorkerAffinity_UnknownWorker returns nil for a worker with no records.
func TestGetWorkerAffinity_UnknownWorker(t *testing.T) {
	s := newTestStore(t)
	records, err := s.GetWorkerAffinity("nobody")
	if err != nil {
		t.Fatalf("GetWorkerAffinity: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records for unknown worker, got %d", len(records))
	}
}

// TestTagIntersection exercises the helper directly.
func TestTagIntersection(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{nil, nil, 0},
		{[]string{"go"}, nil, 0},
		{nil, []string{"go"}, 0},
		{[]string{"go", "linux"}, []string{"go"}, 1},
		{[]string{"go", "linux"}, []string{"go", "linux"}, 2},
		{[]string{"go"}, []string{"python"}, 0},
		{[]string{"a", "b", "c"}, []string{"b", "c", "d"}, 2},
	}
	for _, tc := range cases {
		got := tagIntersection(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("tagIntersection(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
