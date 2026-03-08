// Package knowledge provides a Go-native knowledge document store with FTS5 search.
//
// Obsidian-style Markdown files are the SSOT (Single Source of Truth).
// index.db maintains metadata + FTS5 for fast search. Schema is compatible
// with the Python DocumentStore (c4/knowledge/documents.py).
package knowledge

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// DocumentType represents knowledge document types.
type DocumentType string

const (
	TypeExperiment DocumentType = "experiment"
	TypePattern    DocumentType = "pattern"
	TypeInsight    DocumentType = "insight"
	TypeHypothesis DocumentType = "hypothesis"
)

// docTypePrefixes maps ID prefixes to document types.
var docTypePrefixes = map[string]DocumentType{
	"exp": TypeExperiment,
	"pat": TypePattern,
	"ins": TypeInsight,
	"hyp": TypeHypothesis,
}

// prefixForType returns the ID prefix for a document type.
func prefixForType(dt DocumentType) string {
	for prefix, t := range docTypePrefixes {
		if t == dt {
			return prefix
		}
	}
	return "doc"
}

// Document represents a knowledge document with frontmatter + body.
type Document struct {
	ID               string       `json:"id"`
	Type             DocumentType `json:"type"`
	Title            string       `json:"title"`
	Domain           string       `json:"domain,omitempty"`
	Tags             []string     `json:"tags,omitempty"`
	TaskID           string       `json:"task_id,omitempty"`
	Visibility       string       `json:"visibility,omitempty"`
	Hypothesis       string       `json:"hypothesis,omitempty"`
	HypothesisStatus string       `json:"hypothesis_status,omitempty"`
	Confidence       float64      `json:"confidence,omitempty"`
	EvidenceCount    int          `json:"evidence_count,omitempty"`
	EvidenceIDs      []string     `json:"evidence_ids,omitempty"`
	InsightType      string       `json:"insight_type,omitempty"`
	SourceCount      int          `json:"source_count,omitempty"`
	Status           string       `json:"status,omitempty"`
	EvidenceFor      []string     `json:"evidence_for,omitempty"`
	EvidenceAgainst  []string     `json:"evidence_against,omitempty"`
	ExpiresAt        string       `json:"expires_at,omitempty"`
	YAMLDraft        string       `json:"yaml_draft,omitempty"`
	CreatedAt        string       `json:"created_at"`
	UpdatedAt        string       `json:"updated_at"`
	Version          int          `json:"version"`
	Body             string       `json:"body"`
}

// Store provides knowledge document persistence with Markdown SSOT + SQLite FTS5 index.
type Store struct {
	db      *sql.DB
	docsDir string
	dbPath  string
}

const indexSchema = `
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    domain TEXT DEFAULT '',
    tags_json TEXT DEFAULT '[]',
    hypothesis_status TEXT DEFAULT '',
    confidence REAL DEFAULT 0.0,
    task_id TEXT DEFAULT '',
    metadata_json TEXT DEFAULT '{}',
    file_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_doc_type ON documents(type);
CREATE INDEX IF NOT EXISTS idx_doc_domain ON documents(domain);
CREATE INDEX IF NOT EXISTS idx_doc_task ON documents(task_id);
CREATE INDEX IF NOT EXISTS idx_doc_hypothesis ON documents(hypothesis_status);
`

const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    id, title, domain, tags_text, body_text
);
`

// NewStore opens (or creates) the knowledge store at basePath.
// basePath is typically ".c4/knowledge".
func NewStore(basePath string) (*Store, error) {
	docsDir := filepath.Join(basePath, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return nil, fmt.Errorf("create docs dir: %w", err)
	}

	dbPath := filepath.Join(basePath, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open knowledge db: %w", err)
	}
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			fmt.Fprintf(os.Stderr, "c4: knowledge: %s failed: %v\n", pragma, err)
		}
	}

	s := &Store{db: db, docsDir: docsDir, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("knowledge migrate: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(indexSchema); err != nil {
		return fmt.Errorf("index schema: %w", err)
	}

	// Check if FTS table exists before creating
	var name sql.NullString
	err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='documents_fts'").Scan(&name)
	if err == sql.ErrNoRows || !name.Valid {
		if _, err := s.db.Exec(ftsSchema); err != nil {
			return fmt.Errorf("fts schema: %w", err)
		}
	}
	return nil
}

// generateID creates a document ID with type prefix.
func generateID(docType DocumentType) string {
	prefix := prefixForType(docType)
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

// contentHash returns a truncated SHA256 hash for change detection.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)[:16]
}

// Create creates a new knowledge document.
func (s *Store) Create(docType DocumentType, metadata map[string]any, body string) (string, error) {
	docID, _ := metadata["id"].(string)
	if docID == "" {
		docID = generateID(docType)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	doc := &Document{
		ID:        docID,
		Type:      docType,
		Title:     stringVal(metadata, "title"),
		Domain:    stringVal(metadata, "domain"),
		Tags:      stringSliceVal(metadata, "tags"),
		TaskID:    stringVal(metadata, "task_id"),
		CreatedAt: stringOrDefault(metadata, "created_at", now),
		UpdatedAt: now,
		Version:   1,
		Body:      body,
	}

	// Visibility
	doc.Visibility = stringVal(metadata, "visibility")

	// Type-specific fields
	doc.Hypothesis = stringVal(metadata, "hypothesis")
	doc.HypothesisStatus = stringVal(metadata, "hypothesis_status")
	doc.Confidence = floatVal(metadata, "confidence")
	doc.EvidenceCount = intVal(metadata, "evidence_count")
	doc.EvidenceIDs = stringSliceVal(metadata, "evidence_ids")
	doc.InsightType = stringVal(metadata, "insight_type")
	doc.SourceCount = intVal(metadata, "source_count")
	doc.Status = stringVal(metadata, "status")
	doc.EvidenceFor = stringSliceVal(metadata, "evidence_for")
	doc.EvidenceAgainst = stringSliceVal(metadata, "evidence_against")
	doc.ExpiresAt = stringVal(metadata, "expires_at")
	doc.YAMLDraft = stringVal(metadata, "yaml_draft")

	// Write Markdown file first (SSOT)
	mdContent := docToMarkdown(doc)
	filePath := filepath.Join(s.docsDir, docID+".md")
	if err := os.WriteFile(filePath, []byte(mdContent), 0644); err != nil {
		return "", fmt.Errorf("write markdown: %w", err)
	}

	// Index in SQLite
	if err := s.indexDocument(doc, filePath, mdContent); err != nil {
		os.Remove(filePath) // rollback
		return "", fmt.Errorf("index document: %w", err)
	}

	return docID, nil
}

// Get retrieves a document by ID, reading from the Markdown file (SSOT).
func (s *Store) Get(docID string) (*Document, error) {
	filePath := filepath.Join(s.docsDir, docID+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read document: %w", err)
	}

	fm, body := parseFrontmatter(string(data))
	doc := frontmatterToDocument(fm, body, docID)
	return doc, nil
}

// Update updates an existing document (merge metadata, optional body).
func (s *Store) Update(docID string, metadata map[string]any, body *string) (bool, error) {
	doc, err := s.Get(docID)
	if err != nil {
		return false, err
	}
	if doc == nil {
		return false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Merge metadata
	if metadata != nil {
		if v, ok := metadata["title"]; ok {
			doc.Title, _ = v.(string)
		}
		if v, ok := metadata["domain"]; ok {
			doc.Domain, _ = v.(string)
		}
		if v, ok := metadata["tags"]; ok {
			doc.Tags = toStringSlice(v)
		}
		if v, ok := metadata["task_id"]; ok {
			doc.TaskID, _ = v.(string)
		}
		if v, ok := metadata["visibility"]; ok {
			doc.Visibility, _ = v.(string)
		}
		if v, ok := metadata["hypothesis"]; ok {
			doc.Hypothesis, _ = v.(string)
		}
		if v, ok := metadata["hypothesis_status"]; ok {
			doc.HypothesisStatus, _ = v.(string)
		}
		if v, ok := metadata["confidence"]; ok {
			doc.Confidence = toFloat(v)
		}
		if v, ok := metadata["status"]; ok {
			doc.Status, _ = v.(string)
		}
	}

	if body != nil {
		doc.Body = *body
	}

	doc.UpdatedAt = now
	doc.Version++

	// Write updated Markdown
	mdContent := docToMarkdown(doc)
	filePath := filepath.Join(s.docsDir, docID+".md")
	oldContent, _ := os.ReadFile(filePath)
	if err := os.WriteFile(filePath, []byte(mdContent), 0644); err != nil {
		return false, fmt.Errorf("write markdown: %w", err)
	}

	if err := s.indexDocument(doc, filePath, mdContent); err != nil {
		// Rollback
		if oldContent != nil {
			os.WriteFile(filePath, oldContent, 0644)
		}
		return false, fmt.Errorf("index document: %w", err)
	}

	return true, nil
}

// Delete removes a document.
func (s *Store) Delete(docID string) (bool, error) {
	filePath := filepath.Join(s.docsDir, docID+".md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, nil
	}

	if err := os.Remove(filePath); err != nil {
		return false, fmt.Errorf("remove file: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM documents_fts WHERE id = ?", docID)
	tx.Exec("DELETE FROM documents WHERE id = ?", docID)
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// List returns document summaries with optional filters.
func (s *Store) List(docType string, domain string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}

	query := "SELECT id, type, title, domain, tags_json, hypothesis_status, confidence, task_id, created_at, updated_at, version FROM documents"
	var conditions []string
	var args []any

	if docType != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, docType)
	}
	if domain != "" {
		conditions = append(conditions, "domain = ?")
		args = append(args, domain)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var (
			id, typ, title, dom, tagsJSON string
			hypStatus, taskID              string
			conf                           float64
			createdAt, updatedAt           string
			version                        int
		)
		if err := rows.Scan(&id, &typ, &title, &dom, &tagsJSON, &hypStatus, &conf, &taskID, &createdAt, &updatedAt, &version); err != nil {
			return nil, err
		}

		var tags []string
		json.Unmarshal([]byte(tagsJSON), &tags)

		results = append(results, map[string]any{
			"id":                id,
			"type":              typ,
			"title":             title,
			"domain":            dom,
			"tags":              tags,
			"hypothesis_status": hypStatus,
			"confidence":        conf,
			"task_id":           taskID,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
			"version":           version,
		})
	}
	return results, rows.Err()
}

// ListPending returns knowledge documents with status="pending", filtered by confidence level.
// confidence: "HIGH" (>=0.8), "MEDIUM" (>=0.5), "ALL" (no filter). Default limit: 5.
func (s *Store) ListPending(confidence string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 5
	}

	query := `SELECT id, type, title, confidence, created_at, metadata_json, file_path
		FROM documents
		WHERE json_extract(metadata_json, '$.status') = 'pending'`
	var args []any

	switch strings.ToUpper(confidence) {
	case "HIGH":
		query += " AND confidence >= ?"
		args = append(args, 0.8)
	case "MEDIUM":
		query += " AND confidence >= ?"
		args = append(args, 0.5)
	}
	query += " ORDER BY confidence DESC, created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, typ, title, createdAt, metaJSON, filePath string
		var conf float64
		if err := rows.Scan(&id, &typ, &title, &conf, &createdAt, &metaJSON, &filePath); err != nil {
			return nil, err
		}

		// Read body from Markdown SSOT
		var body string
		if data, err := os.ReadFile(filePath); err == nil {
			_, body = parseFrontmatter(string(data))
		}

		// Map confidence float to label
		confLabel := "LOW"
		switch {
		case conf >= 0.8:
			confLabel = "HIGH"
		case conf >= 0.5:
			confLabel = "MEDIUM"
		}

		// Extract source_date (date part of created_at)
		sourceDate := createdAt
		if len(createdAt) >= 10 {
			sourceDate = createdAt[:10]
		}

		results = append(results, map[string]any{
			"id":          id,
			"title":       title,
			"content":     body,
			"item_type":   typ,
			"confidence":  confLabel,
			"source_date": sourceDate,
			"proposed_at": nil,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	return results, rows.Err()
}

// SearchFTS performs full-text search using FTS5.
func (s *Store) SearchFTS(query string, topK int) ([]map[string]any, error) {
	if topK <= 0 {
		topK = 10
	}

	rows, err := s.db.Query(`
		SELECT d.id, d.title, d.type, d.domain, rank AS score
		FROM documents_fts f
		JOIN documents d ON d.id = f.id
		WHERE documents_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, query, topK)
	if err != nil {
		// FTS query syntax error — fallback to LIKE
		likeQ := "%" + query + "%"
		rows, err = s.db.Query(`
			SELECT id, title, type, domain, 0.0 AS score
			FROM documents
			WHERE title LIKE ? OR domain LIKE ?
			ORDER BY updated_at DESC
			LIMIT ?`, likeQ, likeQ, topK)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, title, typ, domain string
		var score float64
		if err := rows.Scan(&id, &title, &typ, &domain, &score); err != nil {
			return nil, err
		}
		if score < 0 {
			score = -score // FTS5 rank is negative
		}
		results = append(results, map[string]any{
			"id":     id,
			"title":  title,
			"type":   typ,
			"domain": domain,
			"score":  score,
		})
	}
	return results, rows.Err()
}

// RebuildIndex rebuilds the SQLite index from Markdown files.
func (s *Store) RebuildIndex() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM documents_fts")
	tx.Exec("DELETE FROM documents")
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	entries, err := os.ReadDir(s.docsDir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(s.docsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		fm, body := parseFrontmatter(string(data))
		docID := strings.TrimSuffix(entry.Name(), ".md")
		if fmID, ok := fm["id"].(string); ok && fmID != "" {
			docID = fmID
		}

		doc := frontmatterToDocument(fm, body, docID)
		if err := s.indexDocument(doc, filePath, string(data)); err != nil {
			continue
		}
		count++
	}

	return count, nil
}

// DocsDir returns the path to the documents directory.
func (s *Store) DocsDir() string {
	return s.docsDir
}

// DB returns the underlying database (for vector store to share).
func (s *Store) DB() *sql.DB {
	return s.db
}

// indexDocument inserts or replaces a document in the index + FTS.
func (s *Store) indexDocument(doc *Document, filePath, mdContent string) error {
	tagsJSON, _ := json.Marshal(doc.Tags)
	tagsText := strings.Join(doc.Tags, " ")
	ch := contentHash(mdContent)

	// Build extra metadata
	extraMeta := map[string]any{}
	if doc.Hypothesis != "" {
		extraMeta["hypothesis"] = doc.Hypothesis
	}
	if doc.InsightType != "" {
		extraMeta["insight_type"] = doc.InsightType
	}
	if doc.Status != "" {
		extraMeta["status"] = doc.Status
	}
	metaJSON, _ := json.Marshal(extraMeta)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT OR REPLACE INTO documents
		(id, type, title, domain, tags_json, hypothesis_status,
		 confidence, task_id, metadata_json, file_path,
		 content_hash, created_at, updated_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, string(doc.Type), doc.Title, doc.Domain, string(tagsJSON),
		doc.HypothesisStatus, doc.Confidence, doc.TaskID,
		string(metaJSON), filePath, ch, doc.CreatedAt, doc.UpdatedAt, doc.Version)
	if err != nil {
		return err
	}

	// Update FTS (delete + insert)
	tx.Exec("DELETE FROM documents_fts WHERE id = ?", doc.ID)
	_, err = tx.Exec(`INSERT INTO documents_fts (id, title, domain, tags_text, body_text)
		VALUES (?, ?, ?, ?, ?)`,
		doc.ID, doc.Title, doc.Domain, tagsText, doc.Body)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// =========================================================================
// Markdown frontmatter helpers
// =========================================================================

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)`)

// parseFrontmatter extracts YAML-like frontmatter and body from Markdown.
// Uses simple key: value parsing instead of a YAML library to avoid dependencies.
func parseFrontmatter(text string) (map[string]any, string) {
	match := frontmatterRe.FindStringSubmatch(text)
	if match == nil {
		return map[string]any{}, text
	}

	fm := parseSimpleYAML(match[1])
	body := strings.TrimSpace(match[2])
	return fm, body
}

// parseSimpleYAML handles basic YAML frontmatter (key: value, lists).
func parseSimpleYAML(text string) map[string]any {
	result := make(map[string]any)
	lines := strings.Split(text, "\n")
	var currentKey string
	var currentList []string
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// List item
		if strings.HasPrefix(trimmed, "- ") && inList {
			val := strings.TrimPrefix(trimmed, "- ")
			val = strings.Trim(val, "'\"")
			currentList = append(currentList, val)
			continue
		}

		// Save previous list
		if inList && currentKey != "" {
			result[currentKey] = currentList
			inList = false
			currentList = nil
		}

		// Key: value
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])

		if val == "" {
			// Start of a list
			currentKey = key
			inList = true
			currentList = nil
			continue
		}

		// Inline list [a, b, c]
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			inner := val[1 : len(val)-1]
			var items []string
			for _, item := range strings.Split(inner, ",") {
				item = strings.TrimSpace(item)
				item = strings.Trim(item, "'\"")
				if item != "" {
					items = append(items, item)
				}
			}
			result[key] = items
			continue
		}

		// Remove quotes
		val = strings.Trim(val, "'\"")

		// Type detection
		if val == "true" {
			result[key] = true
		} else if val == "false" {
			result[key] = false
		} else if f, err := parseFloat(val); err == nil {
			// Check if it's an integer
			if !strings.Contains(val, ".") {
				if i, err := parseInt(val); err == nil {
					result[key] = i
					continue
				}
			}
			result[key] = f
		} else {
			result[key] = val
		}
	}

	// Save trailing list
	if inList && currentKey != "" {
		result[currentKey] = currentList
	}

	return result
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

// docToMarkdown converts a Document to Markdown with YAML frontmatter.
func docToMarkdown(doc *Document) string {
	var b strings.Builder
	b.WriteString("---\n")

	writeField := func(key, val string) {
		if val != "" {
			b.WriteString(fmt.Sprintf("%s: %s\n", key, val))
		}
	}
	writeFieldAlways := func(key, val string) {
		b.WriteString(fmt.Sprintf("%s: %s\n", key, val))
	}

	writeFieldAlways("id", doc.ID)
	writeFieldAlways("type", string(doc.Type))
	writeFieldAlways("title", doc.Title)
	writeField("domain", doc.Domain)
	writeField("task_id", doc.TaskID)
	writeField("visibility", doc.Visibility)

	if len(doc.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range doc.Tags {
			b.WriteString(fmt.Sprintf("- %s\n", tag))
		}
	}

	// Type-specific fields
	switch doc.Type {
	case TypeExperiment:
		writeField("hypothesis", doc.Hypothesis)
		writeField("hypothesis_status", doc.HypothesisStatus)
	case TypePattern:
		b.WriteString(fmt.Sprintf("confidence: %g\n", doc.Confidence))
		b.WriteString(fmt.Sprintf("evidence_count: %d\n", doc.EvidenceCount))
		if len(doc.EvidenceIDs) > 0 {
			b.WriteString("evidence_ids:\n")
			for _, id := range doc.EvidenceIDs {
				b.WriteString(fmt.Sprintf("- %s\n", id))
			}
		}
	case TypeInsight:
		writeField("insight_type", doc.InsightType)
		if doc.SourceCount > 0 {
			b.WriteString(fmt.Sprintf("source_count: %d\n", doc.SourceCount))
		}
	case TypeHypothesis:
		writeField("status", doc.Status)
		writeField("hypothesis_status", doc.HypothesisStatus)
		writeField("expires_at", doc.ExpiresAt)
		writeField("yaml_draft", doc.YAMLDraft)
		b.WriteString(fmt.Sprintf("confidence: %g\n", doc.Confidence))
		if len(doc.EvidenceFor) > 0 {
			b.WriteString("evidence_for:\n")
			for _, id := range doc.EvidenceFor {
				b.WriteString(fmt.Sprintf("- %s\n", id))
			}
		}
		if len(doc.EvidenceAgainst) > 0 {
			b.WriteString("evidence_against:\n")
			for _, id := range doc.EvidenceAgainst {
				b.WriteString(fmt.Sprintf("- %s\n", id))
			}
		}
	}

	writeFieldAlways("created_at", doc.CreatedAt)
	writeFieldAlways("updated_at", doc.UpdatedAt)
	b.WriteString(fmt.Sprintf("version: %d\n", doc.Version))

	b.WriteString("---\n\n")
	b.WriteString(doc.Body)

	return b.String()
}

// frontmatterToDocument converts parsed frontmatter + body to a Document.
func frontmatterToDocument(fm map[string]any, body, fallbackID string) *Document {
	docID, _ := fm["id"].(string)
	if docID == "" {
		docID = fallbackID
	}
	docType, _ := fm["type"].(string)
	if docType == "" {
		docType = "experiment"
	}

	doc := &Document{
		ID:               docID,
		Type:             DocumentType(docType),
		Title:            fmString(fm, "title"),
		Domain:           fmString(fm, "domain"),
		Tags:             fmStringSlice(fm, "tags"),
		TaskID:           fmString(fm, "task_id"),
		Visibility:       fmString(fm, "visibility"),
		Hypothesis:       fmString(fm, "hypothesis"),
		HypothesisStatus: fmString(fm, "hypothesis_status"),
		Confidence:       fmFloat(fm, "confidence"),
		EvidenceCount:    fmInt(fm, "evidence_count"),
		EvidenceIDs:      fmStringSlice(fm, "evidence_ids"),
		InsightType:      fmString(fm, "insight_type"),
		SourceCount:      fmInt(fm, "source_count"),
		Status:           fmString(fm, "status"),
		EvidenceFor:      fmStringSlice(fm, "evidence_for"),
		EvidenceAgainst:  fmStringSlice(fm, "evidence_against"),
		ExpiresAt:        fmString(fm, "expires_at"),
		YAMLDraft:        fmString(fm, "yaml_draft"),
		CreatedAt:        fmString(fm, "created_at"),
		UpdatedAt:        fmString(fm, "updated_at"),
		Version:          fmInt(fm, "version"),
		Body:             body,
	}
	if doc.Version == 0 {
		doc.Version = 1
	}
	return doc
}

// GetBacklinks finds all documents that reference docID via [[backlink]].
func (s *Store) GetBacklinks(docID string) ([]string, error) {
	re := regexp.MustCompile(`\[\[` + regexp.QuoteMeta(docID) + `(?:\|[^\]]+)?\]\]`)

	entries, err := os.ReadDir(s.docsDir)
	if err != nil {
		return nil, err
	}

	var backlinks []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		stem := strings.TrimSuffix(entry.Name(), ".md")
		if stem == docID {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.docsDir, entry.Name()))
		if err != nil {
			continue
		}
		if re.Match(data) {
			backlinks = append(backlinks, stem)
		}
	}
	sort.Strings(backlinks)
	return backlinks, nil
}

// =========================================================================
// Value extraction helpers
// =========================================================================

func stringVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func stringOrDefault(m map[string]any, key, def string) string {
	v, _ := m[key].(string)
	if v == "" {
		return def
	}
	return v
}

func stringSliceVal(m map[string]any, key string) []string {
	return toStringSlice(m[key])
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var result []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func floatVal(m map[string]any, key string) float64 {
	return toFloat(m[key])
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return 0
}

func intVal(m map[string]any, key string) int {
	switch t := m[key].(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func fmString(fm map[string]any, key string) string {
	v, _ := fm[key].(string)
	return v
}

func fmFloat(fm map[string]any, key string) float64 {
	return toFloat(fm[key])
}

func fmInt(fm map[string]any, key string) int {
	switch t := fm[key].(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func fmStringSlice(fm map[string]any, key string) []string {
	return toStringSlice(fm[key])
}
