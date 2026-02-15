package knowledge

import (
	"context"
	"fmt"
	"sort"
)

// Searcher provides hybrid search combining vector similarity and FTS5 keyword search.
// Uses 2-way or 3-way RRF (Reciprocal Rank Fusion) to merge results.
// When UsageTracker is set, popularity scores boost ranking (3-way RRF).
type Searcher struct {
	store        *Store
	vectorStore  *VectorStore  // nil if embeddings not available
	usageTracker *UsageTracker // nil if usage tracking disabled
	dimension    int
}

// NewSearcher creates a hybrid searcher. vectorStore may be nil (FTS-only mode).
func NewSearcher(store *Store, vectorStore *VectorStore) *Searcher {
	dim := 0
	if vectorStore != nil {
		dim = vectorStore.dimension
	}
	return &Searcher{
		store:       store,
		vectorStore: vectorStore,
		dimension:   dim,
	}
}

// SetUsageTracker enables popularity-boosted ranking (3-way RRF).
func (s *Searcher) SetUsageTracker(ut *UsageTracker) {
	s.usageTracker = ut
}

// VectorStore returns the underlying vector store (may be nil).
func (s *Searcher) VectorStore() *VectorStore {
	return s.vectorStore
}

// SearchResult holds a single hybrid search result.
type SearchResult struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Type            string  `json:"type"`
	Domain          string  `json:"domain"`
	RRFScore        float64 `json:"rrf_score"`
	EmbeddingSource string  `json:"embedding_source,omitempty"` // "real", "mock", or ""
}

// Search performs hybrid search with optional filters.
func (s *Searcher) Search(query string, topK int, filters map[string]string) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	fetchK := topK * 2 // over-fetch for better RRF merge

	// 1. FTS5 keyword search
	ftsResults, err := s.store.SearchFTS(query, fetchK)
	if err != nil {
		return nil, err
	}

	// 2. Vector search (semantic) — skip if no vector store
	var vecResults []VectorResult
	if s.vectorStore != nil && s.vectorStore.Count() > 0 {
		queryEmb, _, _ := s.vectorStore.EmbedText(context.Background(), query)
		vecResults, _ = s.vectorStore.Search(queryEmb, fetchK)
	}

	// 3. RRF merge
	merged := rrfMerge(ftsResults, vecResults, 60)

	// 3.5. Popularity boost (3-way RRF)
	if s.usageTracker != nil && len(merged) > 0 {
		docIDs := make([]string, len(merged))
		for i, r := range merged {
			docIDs[i] = r.ID
		}
		popularity := s.usageTracker.GetPopularity(docIDs)
		if len(popularity) > 0 {
			boostRRFWithPopularity(merged, popularity)
		}
	}

	// 4. Enrich with metadata and apply filters
	merged = s.enrichAndFilter(merged, filters)

	// 5. Annotate embedding source
	if s.vectorStore != nil {
		for i, r := range merged {
			model := s.vectorStore.GetModel(r.ID)
			if model != "" {
				merged[i].EmbeddingSource = model
			}
		}
	}

	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}

// SearchByType is a convenience for type-filtered search.
func (s *Searcher) SearchByType(query, docType string, topK int) ([]SearchResult, error) {
	return s.Search(query, topK, map[string]string{"type": docType})
}

// rrfMerge performs Reciprocal Rank Fusion of FTS and vector results.
// RRF score = sum(1 / (k + rank_i + 1)) for each list where doc appears.
func rrfMerge(ftsResults []map[string]any, vecResults []VectorResult, k int) []SearchResult {
	scores := map[string]float64{}
	docs := map[string]SearchResult{}

	// FTS results
	for rank, r := range ftsResults {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		scores[id] += 1.0 / float64(k+rank+1)
		if _, exists := docs[id]; !exists {
			title, _ := r["title"].(string)
			typ, _ := r["type"].(string)
			domain, _ := r["domain"].(string)
			docs[id] = SearchResult{
				ID:     id,
				Title:  title,
				Type:   typ,
				Domain: domain,
			}
		}
	}

	// Vector results
	for rank, r := range vecResults {
		scores[r.DocID] += 1.0 / float64(k+rank+1)
		if _, exists := docs[r.DocID]; !exists {
			docs[r.DocID] = SearchResult{ID: r.DocID}
		}
	}

	// Sort by RRF score descending
	type idScore struct {
		id    string
		score float64
	}
	var sorted []idScore
	for id, score := range scores {
		sorted = append(sorted, idScore{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	results := make([]SearchResult, 0, len(sorted))
	for _, is := range sorted {
		sr := docs[is.id]
		sr.RRFScore = is.score
		results = append(results, sr)
	}
	return results
}

// boostRRFWithPopularity adds a popularity component to RRF scores.
// Popularity is normalized and added as a third RRF signal with k=60.
func boostRRFWithPopularity(results []SearchResult, popularity map[string]float64) {
	if len(popularity) == 0 {
		return
	}

	// Find max popularity for normalization
	maxPop := 0.0
	for _, p := range popularity {
		if p > maxPop {
			maxPop = p
		}
	}
	if maxPop == 0 {
		return
	}

	// Sort by popularity descending to assign ranks
	type popEntry struct {
		id    string
		score float64
	}
	var sorted []popEntry
	for id, score := range popularity {
		sorted = append(sorted, popEntry{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	popRank := make(map[string]int, len(sorted))
	for i, e := range sorted {
		popRank[e.id] = i
	}

	// Boost RRF scores with popularity rank
	k := 60
	for i, r := range results {
		if rank, ok := popRank[r.ID]; ok {
			results[i].RRFScore += 1.0 / float64(k+rank+1)
		}
	}

	// Re-sort by boosted RRF score
	sort.Slice(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})
}

// enrichAndFilter enriches results with metadata from the index and applies filters.
func (s *Searcher) enrichAndFilter(results []SearchResult, filters map[string]string) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// Load metadata for all docs (O(m+n))
	allDocs, err := s.store.List("", "", 10000)
	if err != nil {
		return results
	}
	docMap := make(map[string]map[string]any, len(allDocs))
	for _, d := range allDocs {
		id, _ := d["id"].(string)
		docMap[id] = d
	}

	var enriched []SearchResult
	for _, r := range results {
		meta, ok := docMap[r.ID]
		if !ok {
			continue
		}

		// Enrich
		if title, ok := meta["title"].(string); ok && title != "" {
			r.Title = title
		}
		if typ, ok := meta["type"].(string); ok && typ != "" {
			r.Type = typ
		}
		if domain, ok := meta["domain"].(string); ok {
			r.Domain = domain
		}

		// Apply filters
		if filters != nil {
			match := true
			for key, val := range filters {
				metaVal, _ := meta[key].(string)
				if metaVal != val {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		enriched = append(enriched, r)
	}
	return enriched
}

// IndexDocument generates and stores an embedding for a document.
// Uses real embeddings if an Embedder is configured, otherwise falls back to MockEmbedding.
func (s *Searcher) IndexDocument(docID string, doc *Document) error {
	if s.vectorStore == nil {
		return nil
	}

	text := documentToText(doc)
	if text == "" {
		return nil
	}

	emb, model, err := s.vectorStore.EmbedText(context.Background(), text)
	if err != nil {
		return err
	}
	return s.vectorStore.Add(docID, emb, model)
}

// BatchIndexDocuments generates embeddings for multiple documents in a single batch API call.
func (s *Searcher) BatchIndexDocuments(ids []string, docs []*Document) error {
	if s.vectorStore == nil || len(ids) == 0 {
		return nil
	}
	if len(ids) != len(docs) {
		return fmt.Errorf("ids and docs length mismatch: %d vs %d", len(ids), len(docs))
	}

	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = documentToText(doc)
	}

	embeddings, model, err := s.vectorStore.EmbedTexts(context.Background(), texts)
	if err != nil {
		return fmt.Errorf("batch embed: %w", err)
	}

	for i, emb := range embeddings {
		if err := s.vectorStore.Add(ids[i], emb, model); err != nil {
			return fmt.Errorf("add embedding %s: %w", ids[i], err)
		}
	}
	return nil
}

// ReindexDocument removes existing embeddings and re-embeds a document with its chunks.
// Used when document content changes to ensure fresh embeddings.
func (s *Searcher) ReindexDocument(docID string, doc *Document) error {
	if s.vectorStore == nil {
		return nil
	}

	// Delete existing embeddings (parent + chunks)
	s.vectorStore.DeleteByPrefix(docID)

	// Re-index the parent document
	return s.IndexDocument(docID, doc)
}

// documentToText converts a Document to searchable text for embedding.
// Compatible with Python _document_to_text().
func documentToText(doc *Document) string {
	var parts []string
	if doc.Title != "" {
		parts = append(parts, doc.Title)
	}
	if doc.Hypothesis != "" {
		parts = append(parts, doc.Hypothesis)
	}
	if doc.Domain != "" {
		parts = append(parts, "domain: "+doc.Domain)
	}
	for _, tag := range doc.Tags {
		parts = append(parts, tag)
	}
	if doc.Body != "" {
		body := doc.Body
		if len(body) > 500 {
			body = body[:500]
		}
		parts = append(parts, body)
	}
	if doc.InsightType != "" {
		parts = append(parts, "type: "+doc.InsightType)
	}
	if doc.Status != "" {
		parts = append(parts, "status: "+doc.Status)
	}

	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += " | " + p
	}
	return result
}
