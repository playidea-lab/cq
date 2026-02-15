package knowledge

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// UsageAction represents a document usage action type.
type UsageAction string

const (
	ActionView      UsageAction = "view"
	ActionSearchHit UsageAction = "search_hit"
	ActionCite      UsageAction = "cite"
)

// UsageRecord represents a single usage event.
type UsageRecord struct {
	DocID     string
	Action    UsageAction
	Timestamp time.Time
}

// UsageTracker provides async, batched usage recording.
// Records are buffered in a channel and flushed periodically to SQLite.
type UsageTracker struct {
	db      *sql.DB
	buffer  chan UsageRecord
	done    chan struct{}
	wg      sync.WaitGroup
}

const (
	usageBufSize    = 256
	usageFlushEvery = 5 * time.Second
)

const usageSchema = `
CREATE TABLE IF NOT EXISTS doc_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_doc ON doc_usage(doc_id);
`

// NewUsageTracker creates a usage tracker backed by SQLite.
func NewUsageTracker(db *sql.DB) (*UsageTracker, error) {
	if _, err := db.Exec(usageSchema); err != nil {
		return nil, fmt.Errorf("usage schema: %w", err)
	}

	ut := &UsageTracker{
		db:     db,
		buffer: make(chan UsageRecord, usageBufSize),
		done:   make(chan struct{}),
	}
	ut.wg.Add(1)
	go ut.flushLoop()
	return ut, nil
}

// Record adds a usage event (non-blocking).
func (ut *UsageTracker) Record(docID string, action UsageAction) {
	select {
	case ut.buffer <- UsageRecord{
		DocID:     docID,
		Action:    action,
		Timestamp: time.Now().UTC(),
	}:
	default:
		// Buffer full — drop silently (usage is best-effort)
	}
}

// Close stops the flush loop and flushes remaining records.
func (ut *UsageTracker) Close() {
	close(ut.done)
	ut.wg.Wait()
}

// GetPopularity returns popularity scores for the given doc IDs.
func (ut *UsageTracker) GetPopularity(docIDs []string) map[string]float64 {
	if len(docIDs) == 0 {
		return nil
	}

	result := make(map[string]float64, len(docIDs))

	// Build query with placeholders
	placeholders := ""
	args := make([]any, len(docIDs))
	for i, id := range docIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT doc_id,
			SUM(CASE action
				WHEN 'cite' THEN 5.0
				WHEN 'view' THEN 2.0
				WHEN 'search_hit' THEN 1.0
				ELSE 0.0
			END) AS score
		FROM doc_usage
		WHERE doc_id IN (%s)
		GROUP BY doc_id`, placeholders)

	rows, err := ut.db.Query(query, args...)
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var docID string
		var score float64
		if rows.Scan(&docID, &score) == nil {
			result[docID] = score
		}
	}

	return result
}

// flushLoop periodically writes buffered records to SQLite.
func (ut *UsageTracker) flushLoop() {
	defer ut.wg.Done()
	ticker := time.NewTicker(usageFlushEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ut.flush()
		case <-ut.done:
			ut.flush() // final flush
			return
		}
	}
}

func (ut *UsageTracker) flush() {
	var records []UsageRecord
	for {
		select {
		case r := <-ut.buffer:
			records = append(records, r)
		default:
			goto insert
		}
	}

insert:
	if len(records) == 0 {
		return
	}

	tx, err := ut.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)")
	if err != nil {
		return
	}
	defer stmt.Close()

	for _, r := range records {
		stmt.Exec(r.DocID, string(r.Action), r.Timestamp.Format(time.RFC3339))
	}
	tx.Commit()
}
