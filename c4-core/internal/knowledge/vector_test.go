package knowledge

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

func setupTestVectorStore(t *testing.T) *VectorStore {
	t.Helper()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := NewStore(basePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	vs, err := NewVectorStore(store.DB(), 4)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	return vs
}

func TestVectorAddAndSearch(t *testing.T) {
	vs := setupTestVectorStore(t)

	// Add two embeddings: one close to query, one far
	vs.Add("doc1", []float32{1, 0, 0, 0}, "test")
	vs.Add("doc2", []float32{0, 1, 0, 0}, "test")

	results, err := vs.Search([]float32{0.9, 0.1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results: got %d, want 2", len(results))
	}
	// doc1 should be more similar
	if results[0].DocID != "doc1" {
		t.Errorf("most similar: got %s, want doc1", results[0].DocID)
	}
	if results[0].Score < results[1].Score {
		t.Error("doc1 score should be higher than doc2")
	}
}

func TestVectorReplace(t *testing.T) {
	vs := setupTestVectorStore(t)

	vs.Add("doc1", []float32{1, 0, 0, 0}, "v1")
	vs.Add("doc1", []float32{0, 1, 0, 0}, "v2") // replace

	if vs.Count() != 1 {
		t.Errorf("count: got %d, want 1", vs.Count())
	}

	results, _ := vs.Search([]float32{0, 1, 0, 0}, 1)
	if len(results) != 1 || results[0].DocID != "doc1" {
		t.Error("replaced embedding should match new vector")
	}
	if results[0].Score < 0.99 {
		t.Errorf("score should be ~1.0 for identical vectors, got %f", results[0].Score)
	}
}

func TestVectorDelete(t *testing.T) {
	vs := setupTestVectorStore(t)

	vs.Add("doc1", []float32{1, 0, 0, 0}, "test")

	deleted, err := vs.Delete("doc1")
	if err != nil || !deleted {
		t.Fatalf("Delete: err=%v, deleted=%v", err, deleted)
	}
	if vs.Count() != 0 {
		t.Errorf("count after delete: got %d", vs.Count())
	}

	// Delete non-existent
	deleted, _ = vs.Delete("nonexistent")
	if deleted {
		t.Error("Delete returned true for non-existent")
	}
}

func TestVectorExists(t *testing.T) {
	vs := setupTestVectorStore(t)

	vs.Add("doc1", []float32{1, 0, 0, 0}, "test")

	if !vs.Exists("doc1") {
		t.Error("Exists should be true for doc1")
	}
	if vs.Exists("doc2") {
		t.Error("Exists should be false for doc2")
	}
}

func TestVectorDimensionMismatch(t *testing.T) {
	vs := setupTestVectorStore(t)

	err := vs.Add("doc1", []float32{1, 0}, "test") // wrong dimension
	if err == nil {
		t.Error("expected error for dimension mismatch")
	}

	_, err = vs.Search([]float32{1, 0}, 10) // wrong dimension
	if err == nil {
		t.Error("expected error for search dimension mismatch")
	}
}

func TestVectorEmptySearch(t *testing.T) {
	vs := setupTestVectorStore(t)

	results, err := vs.Search([]float32{1, 0, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty search: got %d results", len(results))
	}
}

func TestVectorTopK(t *testing.T) {
	vs := setupTestVectorStore(t)

	for i := 0; i < 10; i++ {
		emb := []float32{float32(i), 0, 0, 0}
		vs.Add(fmt.Sprintf("doc%d", i), emb, "test")
	}

	results, _ := vs.Search([]float32{9, 0, 0, 0}, 3)
	if len(results) != 3 {
		t.Errorf("topK=3: got %d results", len(results))
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors → 1.0
	sim := cosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0})
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical: got %f, want 1.0", sim)
	}

	// Orthogonal vectors → 0.0
	sim = cosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0})
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal: got %f, want 0.0", sim)
	}

	// Opposite vectors → -1.0
	sim = cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0})
	if math.Abs(sim+1.0) > 1e-6 {
		t.Errorf("opposite: got %f, want -1.0", sim)
	}

	// Zero vector → 0.0
	sim = cosineSimilarity([]float32{0, 0, 0}, []float32{1, 0, 0})
	if sim != 0 {
		t.Errorf("zero: got %f, want 0.0", sim)
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	original := []float32{1.5, -2.3, 0.0, 42.0}
	blob := encodeEmbedding(original)
	decoded := decodeEmbedding(blob)

	if len(decoded) != len(original) {
		t.Fatalf("len: got %d, want %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("[%d]: got %f, want %f", i, decoded[i], original[i])
		}
	}
}

func TestMockEmbedding(t *testing.T) {
	emb := MockEmbedding("hello", 8)
	if len(emb) != 8 {
		t.Fatalf("dimension: got %d, want 8", len(emb))
	}

	// Deterministic
	emb2 := MockEmbedding("hello", 8)
	for i := range emb {
		if emb[i] != emb2[i] {
			t.Errorf("[%d]: not deterministic", i)
		}
	}

	// Different text → different embedding
	emb3 := MockEmbedding("world", 8)
	same := true
	for i := range emb {
		if emb[i] != emb3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different text should produce different embeddings")
	}

	// Values in [-1, 1]
	for i, v := range emb {
		if v < -1 || v > 1 {
			t.Errorf("[%d]: value %f out of [-1,1]", i, v)
		}
	}
}

// suppress unused import
var _ = fmt.Sprintf
