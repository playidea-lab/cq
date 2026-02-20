package eventbus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
	"gopkg.in/yaml.v3"
)

// eventIDCounter ensures unique event IDs even within the same millisecond.
var eventIDCounter atomic.Int64

// Store provides event and rule persistence backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the eventbus database at dbPath.
// Uses MaxOpenConns(1) + WAL + busy_timeout to prevent deadlocks.
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: eventbus: PRAGMA journal_mode=WAL failed: %v\n", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: eventbus: PRAGMA busy_timeout=5000 failed: %v\n", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS c4_events (
			id           TEXT PRIMARY KEY,
			type         TEXT NOT NULL,
			source       TEXT NOT NULL DEFAULT '',
			data         TEXT NOT NULL DEFAULT '{}',
			project_id   TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			processed    INTEGER NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_events_type ON c4_events(type);
		CREATE INDEX IF NOT EXISTS idx_events_created ON c4_events(created_at DESC);

		CREATE TABLE IF NOT EXISTS c4_event_rules (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL UNIQUE,
			event_pattern TEXT NOT NULL,
			filter_json   TEXT NOT NULL DEFAULT '{}',
			action_type   TEXT NOT NULL,
			action_config TEXT NOT NULL DEFAULT '{}',
			enabled       INTEGER NOT NULL DEFAULT 1,
			priority      INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_rules_pattern ON c4_event_rules(event_pattern);

		CREATE TABLE IF NOT EXISTS c4_event_log (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id    TEXT NOT NULL,
			rule_id     TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'pending',
			error       TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_log_event ON c4_event_log(event_id);
	`)
	if err != nil {
		return err
	}

	// v4: add correlation_id to events (idempotent ALTER TABLE)
	s.db.Exec(`ALTER TABLE c4_events ADD COLUMN correlation_id TEXT NOT NULL DEFAULT ''`)

	// v4: Dead Letter Queue
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS c4_event_dlq (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id        TEXT NOT NULL,
			rule_id         TEXT NOT NULL,
			rule_name       TEXT NOT NULL DEFAULT '',
			event_type      TEXT NOT NULL DEFAULT '',
			error           TEXT NOT NULL DEFAULT '',
			retry_count     INTEGER NOT NULL DEFAULT 0,
			max_retries     INTEGER NOT NULL DEFAULT 3,
			created_at      TEXT NOT NULL,
			last_retried_at TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_dlq_event ON c4_event_dlq(event_id);
	`)
	return err
}

// StoredEvent represents a persisted event.
type StoredEvent struct {
	ID            string
	Type          string
	Source        string
	Data          json.RawMessage
	ProjectID     string
	CorrelationID string
	CreatedAt     time.Time
	Processed     bool
}

// StoredRule represents a persisted rule.
type StoredRule struct {
	ID           string
	Name         string
	EventPattern string
	FilterJSON   string
	ActionType   string
	ActionConfig string
	Enabled      bool
	Priority     int
	CreatedAt    time.Time
}

// StoreEvent persists an event and returns the generated ID.
func (s *Store) StoreEvent(evType, source string, data json.RawMessage, projectID string, correlationID ...string) (string, error) {
	id := generateEventID()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	dataStr := "{}"
	if len(data) > 0 {
		dataStr = string(data)
	}

	corrID := ""
	if len(correlationID) > 0 {
		corrID = correlationID[0]
	}

	_, err := s.db.Exec(`
		INSERT INTO c4_events (id, type, source, data, project_id, correlation_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, evType, source, dataStr, projectID, corrID, now)
	if err != nil {
		return "", fmt.Errorf("insert event: %w", err)
	}
	return id, nil
}

// ListEvents returns events optionally filtered by type, with limit and since.
func (s *Store) ListEvents(evType string, limit int, sinceMs int64) ([]StoredEvent, error) {
	query := `SELECT id, type, source, data, project_id, correlation_id, created_at, processed FROM c4_events`
	args := []any{}
	clauses := []string{}

	if evType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, evType)
	}
	if sinceMs > 0 {
		since := time.UnixMilli(sinceMs).UTC().Format(time.RFC3339Nano)
		clauses = append(clauses, "created_at >= ?")
		args = append(args, since)
	}
	if len(clauses) > 0 {
		query += " WHERE " + clauses[0]
		for _, c := range clauses[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY created_at DESC"
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []StoredEvent
	for rows.Next() {
		var e StoredEvent
		var createdAt string
		var dataStr string
		var processed int
		if err := rows.Scan(&e.ID, &e.Type, &e.Source, &dataStr, &e.ProjectID, &e.CorrelationID, &createdAt, &processed); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(dataStr)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.Processed = processed != 0
		events = append(events, e)
	}
	return events, rows.Err()
}

// AddRule inserts a new rule. Returns the generated ID.
func (s *Store) AddRule(name, pattern, filterJSON, actionType, actionConfig string, enabled bool, priority int) (string, error) {
	id := generateEventID()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if filterJSON == "" {
		filterJSON = "{}"
	}
	if actionConfig == "" {
		actionConfig = "{}"
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO c4_event_rules (id, name, event_pattern, filter_json, action_type, action_config, enabled, priority, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, pattern, filterJSON, actionType, actionConfig, enabledInt, priority, now)
	if err != nil {
		return "", fmt.Errorf("insert rule: %w", err)
	}
	return id, nil
}

// RemoveRule deletes a rule by ID or name.
func (s *Store) RemoveRule(id, name string) error {
	var result sql.Result
	var err error
	if id != "" {
		result, err = s.db.Exec(`DELETE FROM c4_event_rules WHERE id = ?`, id)
	} else if name != "" {
		result, err = s.db.Exec(`DELETE FROM c4_event_rules WHERE name = ?`, name)
	} else {
		return fmt.Errorf("id or name required")
	}
	if err != nil {
		return fmt.Errorf("remove rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rule not found")
	}
	return nil
}

// ToggleRule enables or disables a rule by name.
func (s *Store) ToggleRule(name string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	result, err := s.db.Exec(`UPDATE c4_event_rules SET enabled = ? WHERE name = ?`, enabledInt, name)
	if err != nil {
		return fmt.Errorf("toggle rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rule %q not found", name)
	}
	return nil
}

// ListRules returns all rules ordered by priority DESC.
func (s *Store) ListRules() ([]StoredRule, error) {
	rows, err := s.db.Query(`
		SELECT id, name, event_pattern, filter_json, action_type, action_config, enabled, priority, created_at
		FROM c4_event_rules ORDER BY priority DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var rules []StoredRule
	for rows.Next() {
		var r StoredRule
		var createdAt string
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.EventPattern, &r.FilterJSON, &r.ActionType, &r.ActionConfig, &enabled, &r.Priority, &createdAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// MatchRules returns enabled rules whose event_pattern matches the given event type.
func (s *Store) MatchRules(eventType string) ([]StoredRule, error) {
	rules, err := s.ListRules()
	if err != nil {
		return nil, err
	}

	var matched []StoredRule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if matchPattern(r.EventPattern, eventType) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

// LogDispatch records a dispatch attempt for an event+rule.
func (s *Store) LogDispatch(eventID, ruleID, status, errMsg string, durationMs int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO c4_event_log (event_id, rule_id, status, error, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		eventID, ruleID, status, errMsg, durationMs, now)
	return err
}

// PurgeOldEvents removes events older than maxAge and returns the count deleted.
func (s *Store) PurgeOldEvents(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`DELETE FROM c4_events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge old events: %w", err)
	}
	return result.RowsAffected()
}

// PurgeByCount keeps only the newest maxCount events, deleting the rest.
func (s *Store) PurgeByCount(maxCount int) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM c4_events WHERE id NOT IN (
		SELECT id FROM c4_events ORDER BY created_at DESC LIMIT ?
	)`, maxCount)
	if err != nil {
		return 0, fmt.Errorf("purge by count: %w", err)
	}
	return result.RowsAffected()
}

// PurgeOldLogs removes dispatch log entries older than maxAge.
func (s *Store) PurgeOldLogs(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`DELETE FROM c4_event_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge old logs: %w", err)
	}
	return result.RowsAffected()
}

// EventStats returns aggregate statistics about events, rules, and logs.
func (s *Store) EventStats() (map[string]any, error) {
	stats := map[string]any{}

	var eventCount, ruleCount, logCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM c4_events`).Scan(&eventCount); err != nil {
		return nil, fmt.Errorf("count events: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_rules`).Scan(&ruleCount); err != nil {
		return nil, fmt.Errorf("count rules: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM c4_event_log`).Scan(&logCount); err != nil {
		return nil, fmt.Errorf("count logs: %w", err)
	}

	stats["event_count"] = eventCount
	stats["rule_count"] = ruleCount
	stats["log_count"] = logCount

	var oldest, newest sql.NullString
	s.db.QueryRow(`SELECT MIN(created_at) FROM c4_events`).Scan(&oldest)
	s.db.QueryRow(`SELECT MAX(created_at) FROM c4_events`).Scan(&newest)

	if oldest.Valid {
		stats["oldest_event"] = oldest.String
	} else {
		stats["oldest_event"] = ""
	}
	if newest.Valid {
		stats["newest_event"] = newest.String
	} else {
		stats["newest_event"] = ""
	}

	return stats, nil
}

// ListLogs returns dispatch log entries with optional filters.
// eventID filters by specific event, eventType filters by event type pattern.
func (s *Store) ListLogs(eventID string, limit int, sinceMs int64, eventType ...string) ([]StoredLog, error) {
	query := `SELECT l.id, l.event_id, COALESCE(r.name,''), COALESCE(e.type,''),
		l.status, l.error, l.duration_ms, l.created_at
		FROM c4_event_log l
		LEFT JOIN c4_event_rules r ON l.rule_id = r.id
		LEFT JOIN c4_events e ON l.event_id = e.id`
	args := []any{}
	clauses := []string{}

	if eventID != "" {
		clauses = append(clauses, "l.event_id = ?")
		args = append(args, eventID)
	}
	if len(eventType) > 0 && eventType[0] != "" {
		clauses = append(clauses, "e.type = ?")
		args = append(args, eventType[0])
	}
	if sinceMs > 0 {
		since := time.UnixMilli(sinceMs).UTC().Format(time.RFC3339Nano)
		clauses = append(clauses, "l.created_at >= ?")
		args = append(args, since)
	}
	if len(clauses) > 0 {
		query += " WHERE " + clauses[0]
		for _, c := range clauses[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY l.id DESC"
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var logs []StoredLog
	for rows.Next() {
		var l StoredLog
		var createdAt string
		if err := rows.Scan(&l.ID, &l.EventID, &l.RuleName, &l.EventType,
			&l.Status, &l.Error, &l.DurationMs, &createdAt); err != nil {
			return nil, err
		}
		l.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ListEventsASC returns events in ascending order (for replay).
func (s *Store) ListEventsASC(evType string, sinceMs int64, limit int) ([]StoredEvent, error) {
	query := `SELECT id, type, source, data, project_id, correlation_id, created_at, processed FROM c4_events`
	args := []any{}
	clauses := []string{}

	if evType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, evType)
	}
	if sinceMs > 0 {
		since := time.UnixMilli(sinceMs).UTC().Format(time.RFC3339Nano)
		clauses = append(clauses, "created_at >= ?")
		args = append(args, since)
	}
	if len(clauses) > 0 {
		query += " WHERE " + clauses[0]
		for _, c := range clauses[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY created_at ASC"
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events asc: %w", err)
	}
	defer rows.Close()

	var events []StoredEvent
	for rows.Next() {
		var e StoredEvent
		var createdAt string
		var dataStr string
		var processed int
		if err := rows.Scan(&e.ID, &e.Type, &e.Source, &dataStr, &e.ProjectID, &e.CorrelationID, &createdAt, &processed); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(dataStr)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.Processed = processed != 0
		events = append(events, e)
	}
	return events, rows.Err()
}

// EnsureDefaultRules loads rules from a YAML file, adding any that don't already exist (by name).
func (s *Store) EnsureDefaultRules(yamlData []byte) error {
	type ruleYAML struct {
		Name         string `yaml:"name"          json:"name"`
		EventPattern string `yaml:"event_pattern" json:"event_pattern"`
		FilterJSON   string `yaml:"filter_json"   json:"filter_json"`
		ActionType   string `yaml:"action_type"   json:"action_type"`
		ActionConfig string `yaml:"action_config" json:"action_config"`
		Enabled      bool   `yaml:"enabled"       json:"enabled"`
		Priority     int    `yaml:"priority"      json:"priority"`
	}
	type rulesFile struct {
		Rules []ruleYAML `yaml:"rules" json:"rules"`
	}

	var rf rulesFile
	if err := yaml.Unmarshal(yamlData, &rf); err != nil {
		return fmt.Errorf("parse default rules: %w", err)
	}

	// Get existing rule names
	existing, err := s.ListRules()
	if err != nil {
		return err
	}
	nameSet := make(map[string]bool, len(existing))
	for _, r := range existing {
		nameSet[r.Name] = true
	}

	for _, r := range rf.Rules {
		if nameSet[r.Name] {
			continue
		}
		if _, err := s.AddRule(r.Name, r.EventPattern, r.FilterJSON, r.ActionType, r.ActionConfig, r.Enabled, r.Priority); err != nil {
			fmt.Fprintf(os.Stderr, "c4: eventbus: default rule %q: %v\n", r.Name, err)
		}
	}
	return nil
}

// StoredLog represents a persisted dispatch log entry.
type StoredLog struct {
	ID         int64
	EventID    string
	RuleName   string
	EventType  string
	Status     string
	Error      string
	DurationMs int64
	CreatedAt  time.Time
}

// matchPattern checks if eventType matches a glob pattern.
// Supports "*" (match all), "prefix.*" (wildcard suffix), and exact match.
func matchPattern(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1] // includes the trailing dot
		return len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix
	}
	return pattern == eventType
}

// DLQEntry represents a dead letter queue entry.
type DLQEntry struct {
	ID            int64
	EventID       string
	RuleID        string
	RuleName      string
	EventType     string
	Error         string
	RetryCount    int
	MaxRetries    int
	CreatedAt     time.Time
	LastRetriedAt time.Time
}

// GetEventByID retrieves a single event by its ID.
func (s *Store) GetEventByID(eventID string) (*StoredEvent, error) {
	row := s.db.QueryRow(
		`SELECT id, type, source, data, project_id, correlation_id, created_at, processed FROM c4_events WHERE id = ?`,
		eventID,
	)
	var ev StoredEvent
	var createdAt string
	if err := row.Scan(&ev.ID, &ev.Type, &ev.Source, &ev.Data, &ev.ProjectID, &ev.CorrelationID, &createdAt, &ev.Processed); err != nil {
		return nil, err
	}
	ev.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &ev, nil
}

// InsertDLQ adds a failed dispatch to the dead letter queue.
func (s *Store) InsertDLQ(eventID, ruleID, ruleName, eventType, errMsg string, maxRetries int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO c4_event_dlq (event_id, rule_id, rule_name, event_type, error, max_retries, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		eventID, ruleID, ruleName, eventType, errMsg, maxRetries, now)
	return err
}

// ListDLQ returns dead letter queue entries.
func (s *Store) ListDLQ(limit int) ([]DLQEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, event_id, rule_id, rule_name, event_type, error, retry_count, max_retries, created_at, last_retried_at
		FROM c4_event_dlq ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list dlq: %w", err)
	}
	defer rows.Close()

	var entries []DLQEntry
	for rows.Next() {
		var e DLQEntry
		var createdAt, lastRetried string
		if err := rows.Scan(&e.ID, &e.EventID, &e.RuleID, &e.RuleName, &e.EventType, &e.Error, &e.RetryCount, &e.MaxRetries, &createdAt, &lastRetried); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if lastRetried != "" {
			e.LastRetriedAt, _ = time.Parse(time.RFC3339Nano, lastRetried)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// IncrementDLQRetry increments the retry count for a DLQ entry and returns the entry.
// Returns an error if the entry has already reached max_retries.
func (s *Store) IncrementDLQRetry(id int64) (*DLQEntry, error) {
	// Check current state before incrementing
	var currentRetry, maxRetries int
	err := s.db.QueryRow(`SELECT retry_count, max_retries FROM c4_event_dlq WHERE id = ?`, id).Scan(&currentRetry, &maxRetries)
	if err != nil {
		return nil, fmt.Errorf("dlq entry %d not found", id)
	}
	if currentRetry >= maxRetries {
		return nil, fmt.Errorf("dlq entry %d exceeded max retries (%d/%d)", id, currentRetry, maxRetries)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`UPDATE c4_event_dlq SET retry_count = retry_count + 1, last_retried_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return nil, fmt.Errorf("increment dlq retry: %w", err)
	}

	var e DLQEntry
	var createdAt, lastRetried string
	err = s.db.QueryRow(`
		SELECT id, event_id, rule_id, rule_name, event_type, error, retry_count, max_retries, created_at, last_retried_at
		FROM c4_event_dlq WHERE id = ?`, id).Scan(
		&e.ID, &e.EventID, &e.RuleID, &e.RuleName, &e.EventType, &e.Error, &e.RetryCount, &e.MaxRetries, &createdAt, &lastRetried)
	if err != nil {
		return nil, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if lastRetried != "" {
		e.LastRetriedAt, _ = time.Parse(time.RFC3339Nano, lastRetried)
	}
	return &e, nil
}

// RemoveDLQ removes a DLQ entry by ID.
func (s *Store) RemoveDLQ(id int64) error {
	result, err := s.db.Exec(`DELETE FROM c4_event_dlq WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove dlq: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("dlq entry %d not found", id)
	}
	return nil
}

// PurgeDLQ removes DLQ entries older than maxAge.
func (s *Store) PurgeDLQ(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`DELETE FROM c4_event_dlq WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge dlq: %w", err)
	}
	return result.RowsAffected()
}

func generateEventID() string {
	seq := eventIDCounter.Add(1)
	return fmt.Sprintf("ev-%d-%d", time.Now().UnixNano()/1e6, seq)
}
