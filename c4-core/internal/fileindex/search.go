package fileindex

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FileResult represents a search result.
type FileResult struct {
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	ModifiedAt int64   `json:"modified_at"`
	DeviceID   string  `json:"device_id"`
	Score      float64 `json:"score"`
}

// CreateSearchIndex creates the FTS5 virtual table for file search.
// Call after CreateTables. Safe to call multiple times (idempotent).
func CreateSearchIndex(db *sql.DB) error {
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS file_index_fts USING fts5(
		path, name, device_id
	)`)
	if err != nil {
		return fmt.Errorf("fileindex: create fts5 index: %w", err)
	}
	return nil
}

// RebuildSearchIndex repopulates the FTS5 index from file_index.
// Call after Index() to keep FTS in sync with the base table.
func RebuildSearchIndex(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("fileindex: rebuild fts begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM file_index_fts`); err != nil {
		return fmt.Errorf("fileindex: clear fts: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO file_index_fts(path, name, device_id)
		SELECT path, name, device_id FROM file_index`); err != nil {
		return fmt.Errorf("fileindex: populate fts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("fileindex: rebuild fts commit: %w", err)
	}
	return nil
}

// Search queries the file index with a natural language query.
// Returns results ranked by FTS relevance combined with a recency boost.
// Each token in query is treated as a prefix search.
func Search(db *sql.DB, query string, limit int) ([]FileResult, error) {
	if limit <= 0 {
		limit = 10
	}

	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	// FTS5 bm25() returns negative values; more negative = better match.
	// We join with file_index to get size and modified_at.
	rows, err := db.Query(`
		SELECT fi.path, fi.name, fi.size, fi.modified_at, fi.device_id,
		       bm25(file_index_fts) AS rank
		FROM file_index_fts
		JOIN file_index fi ON fi.path = file_index_fts.path
		                   AND fi.device_id = file_index_fts.device_id
		WHERE file_index_fts MATCH ?
		ORDER BY bm25(file_index_fts)
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("fileindex: search query: %w", err)
	}
	defer rows.Close()

	now := float64(time.Now().Unix())
	const secsPerDay = 86400.0

	var results []FileResult
	for rows.Next() {
		var r FileResult
		var rank float64
		if err := rows.Scan(&r.Path, &r.Name, &r.Size, &r.ModifiedAt, &r.DeviceID, &rank); err != nil {
			return nil, fmt.Errorf("fileindex: scan result: %w", err)
		}

		// bm25 is negative; negate so higher = better relevance.
		relevance := -rank

		// Recency boost: 1.0 for files modified today, decays toward 0 with age.
		daysSince := (now - float64(r.ModifiedAt)) / secsPerDay
		recencyFactor := 1.0 / (1.0 + daysSince/30.0)

		r.Score = relevance + recencyFactor
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fileindex: iterate results: %w", err)
	}

	// Re-sort by computed score (relevance + recency) since SQL ORDER BY used raw rank.
	sortByScore(results)

	return results, nil
}

// buildFTSQuery converts a plain query string into an FTS5 MATCH expression.
// The query is split on whitespace and separator characters (., /, -, _).
// Each resulting sub-token becomes a prefix search (appends *).
// Example: "best.pt"  → `best* pt*`
// Example: "best model" → `best* model*`
// Example: "exp042"    → `exp042*`
func buildFTSQuery(query string) string {
	// Tokenize: split on any non-alphanumeric character.
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t != "" {
			parts = append(parts, t+"*")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

// tokenizeQuery splits the query into lowercase alphanumeric tokens.
// Non-alphanumeric characters (., /, -, space, etc.) are treated as separators.
func tokenizeQuery(query string) []string {
	var tokens []string
	var cur strings.Builder
	for _, ch := range strings.ToLower(query) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			cur.WriteRune(ch)
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// sortByScore sorts results descending by Score using a simple insertion sort.
// Result sets are expected to be small (bounded by limit).
func sortByScore(results []FileResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}
