package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
)

// Embedder generates embeddings from text. Implemented by llm.EmbeddingProvider.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedDimension() int
}

// VectorStore provides embedding storage and cosine similarity search
// using SQLite BLOB columns (no sqlite-vec / no CGO required).
//
// Embeddings are stored as little-endian float32 arrays in a BLOB column.
// Search is brute-force cosine similarity — sufficient for <10K documents.
type VectorStore struct {
	db        *sql.DB
	dimension int
	embedder  Embedder // nil → MockEmbedding fallback
}

const vectorSchema = `
CREATE TABLE IF NOT EXISTS knowledge_vectors (
    doc_id TEXT PRIMARY KEY,
    embedding BLOB NOT NULL,
    model TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
`

// NewVectorStore creates a VectorStore sharing the same *sql.DB as the knowledge Store.
// dimension is the expected embedding size (e.g., 1536 for OpenAI, 384 for MiniLM).
// embedder may be nil — falls back to MockEmbedding.
func NewVectorStore(db *sql.DB, dimension int, embedder Embedder) (*VectorStore, error) {
	if _, err := db.Exec(vectorSchema); err != nil {
		return nil, fmt.Errorf("vector schema: %w", err)
	}
	return &VectorStore{db: db, dimension: dimension, embedder: embedder}, nil
}

// HasRealEmbedder returns true if a real (non-mock) embedder is configured.
func (v *VectorStore) HasRealEmbedder() bool {
	return v.embedder != nil
}

// EmbedText generates an embedding for the given text.
// Returns error if a real embedder is configured but fails (no mock fallback).
// Falls back to MockEmbedding only when no real embedder is configured.
func (v *VectorStore) EmbedText(ctx context.Context, text string) ([]float32, string, error) {
	if v.embedder != nil {
		embeddings, err := v.embedder.Embed(ctx, []string{text})
		if err != nil {
			return nil, "", fmt.Errorf("embedding failed: %w", err)
		}
		if len(embeddings) > 0 {
			return embeddings[0], "real", nil
		}
		return nil, "", fmt.Errorf("embedding returned empty result")
	}
	return MockEmbedding(text, v.dimension), "mock", nil
}

// EmbedTexts generates embeddings for multiple texts in a batch.
// Returns error if a real embedder is configured but fails (no mock fallback).
func (v *VectorStore) EmbedTexts(ctx context.Context, texts []string) ([][]float32, string, error) {
	if v.embedder != nil {
		embeddings, err := v.embedder.Embed(ctx, texts)
		if err != nil {
			return nil, "", fmt.Errorf("batch embedding failed: %w", err)
		}
		return embeddings, "real", nil
	}
	results := make([][]float32, len(texts))
	for i, t := range texts {
		results[i] = MockEmbedding(t, v.dimension)
	}
	return results, "mock", nil
}

// Dimension returns the expected embedding dimension.
func (v *VectorStore) Dimension() int {
	return v.dimension
}

// Add stores an embedding for a document. Replaces if exists.
func (v *VectorStore) Add(docID string, embedding []float32, model string) error {
	if len(embedding) != v.dimension {
		return fmt.Errorf("embedding dimension %d != expected %d", len(embedding), v.dimension)
	}
	blob := encodeEmbedding(embedding)
	_, err := v.db.Exec(
		`INSERT OR REPLACE INTO knowledge_vectors (doc_id, embedding, model, created_at)
		 VALUES (?, ?, ?, datetime('now'))`,
		docID, blob, model)
	return err
}

// Delete removes an embedding by document ID.
func (v *VectorStore) Delete(docID string) (bool, error) {
	res, err := v.db.Exec("DELETE FROM knowledge_vectors WHERE doc_id = ?", docID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Exists checks if an embedding exists for the given document ID.
func (v *VectorStore) Exists(docID string) bool {
	var n int
	v.db.QueryRow("SELECT 1 FROM knowledge_vectors WHERE doc_id = ?", docID).Scan(&n)
	return n == 1
}

// Count returns the number of stored embeddings.
func (v *VectorStore) Count() int {
	var n int
	v.db.QueryRow("SELECT COUNT(*) FROM knowledge_vectors").Scan(&n)
	return n
}

// DeleteByPrefix removes all embeddings whose doc_id starts with the given prefix.
// Used for removing chunk embeddings when reindexing a document.
func (v *VectorStore) DeleteByPrefix(prefix string) (int, error) {
	res, err := v.db.Exec("DELETE FROM knowledge_vectors WHERE doc_id LIKE ?", prefix+"%")
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// GetModel returns the embedding model name for a document ID, or "" if not found.
func (v *VectorStore) GetModel(docID string) string {
	var model string
	v.db.QueryRow("SELECT model FROM knowledge_vectors WHERE doc_id = ?", docID).Scan(&model)
	return model
}

// CountByModel returns the count of embeddings grouped by model name.
func (v *VectorStore) CountByModel() (map[string]int, error) {
	rows, err := v.db.Query("SELECT COALESCE(model, ''), COUNT(*) FROM knowledge_vectors GROUP BY model")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var model string
		var count int
		if err := rows.Scan(&model, &count); err != nil {
			continue
		}
		if model == "" {
			model = "unknown"
		}
		counts[model] = count
	}
	return counts, rows.Err()
}

// AllEmbeddings returns all stored embeddings as a map of docID → embedding.
// Used for batch analysis like clustering and pairwise similarity stats.
func (v *VectorStore) AllEmbeddings() (map[string][]float32, error) {
	rows, err := v.db.Query("SELECT doc_id, embedding FROM knowledge_vectors")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float32)
	for rows.Next() {
		var docID string
		var blob []byte
		if err := rows.Scan(&docID, &blob); err != nil {
			continue
		}
		emb := decodeEmbedding(blob)
		if len(emb) == v.dimension {
			result[docID] = emb
		}
	}
	return result, rows.Err()
}

// VectorResult holds a single search result with similarity score.
type VectorResult struct {
	DocID    string  `json:"id"`
	Score    float64 `json:"score"`
	Distance float64 `json:"distance"`
}

// Search performs brute-force cosine similarity search.
// Returns top-K results sorted by similarity (highest first).
func (v *VectorStore) Search(queryEmbedding []float32, topK int) ([]VectorResult, error) {
	if len(queryEmbedding) != v.dimension {
		return nil, fmt.Errorf("query dimension %d != expected %d", len(queryEmbedding), v.dimension)
	}
	if topK <= 0 {
		topK = 10
	}

	rows, err := v.db.Query("SELECT doc_id, embedding FROM knowledge_vectors")
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}
	defer rows.Close()

	var candidates []scoredDoc

	for rows.Next() {
		var docID string
		var blob []byte
		if err := rows.Scan(&docID, &blob); err != nil {
			continue
		}
		emb := decodeEmbedding(blob)
		if len(emb) != v.dimension {
			continue
		}
		sim := cosineSimilarity(queryEmbedding, emb)
		candidates = append(candidates, scoredDoc{docID: docID, sim: sim})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by similarity descending
	sortByScore(candidates)

	// Take top-K
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	results := make([]VectorResult, len(candidates))
	for i, c := range candidates {
		distance := 1.0 - c.sim
		if distance < 0 {
			distance = 0
		}
		results[i] = VectorResult{
			DocID:    c.docID,
			Score:    c.sim,
			Distance: distance,
		}
	}
	return results, nil
}

type scoredDoc struct {
	docID string
	sim   float64
}

// sortByScore sorts scored items by similarity descending (simple insertion sort — small N).
func sortByScore(items []scoredDoc) {
	for i := 1; i < len(items); i++ {
		key := items[i]
		j := i - 1
		for j >= 0 && items[j].sim < key.sim {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = key
	}
}

// =========================================================================
// Cosine similarity — pure Go math
// =========================================================================

// cosineSimilarity computes cos(a, b) = dot(a,b) / (||a|| * ||b||).
// Returns 0 if either vector has zero norm.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// =========================================================================
// BLOB serialization — binary.LittleEndian float32
// =========================================================================

// encodeEmbedding serializes []float32 to a little-endian byte slice.
func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeEmbedding deserializes a little-endian byte slice to []float32.
func decodeEmbedding(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// =========================================================================
// Mock embedding generator — deterministic hash-based (for testing / no API)
// =========================================================================

// MockEmbedding generates a deterministic pseudo-embedding from text using SHA256.
// Compatible with Python MockEmbeddings for testing round-trip.
func MockEmbedding(text string, dimension int) []float32 {
	hash := sha256.Sum256([]byte(text))
	result := make([]float32, dimension)
	for i := 0; i < dimension; i++ {
		byteIdx := i % len(hash)
		result[i] = (float32(hash[byteIdx])/255.0)*2 - 1
	}
	return result
}
