package knowledge

import (
	"sort"
)

// Searcher provides hybrid search combining vector similarity and FTS5 keyword search.
// Uses 2-way RRF (Reciprocal Rank Fusion) to merge results.
type Searcher struct {
	store       *Store
	vectorStore *VectorStore // nil if embeddings not available
	dimension   int
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

// SearchResult holds a single hybrid search result.
type SearchResult struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Domain   string  `json:"domain"`
	RRFScore float64 `json:"rrf_score"`
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
		queryEmb := MockEmbedding(query, s.dimension)
		vecResults, _ = s.vectorStore.Search(queryEmb, fetchK)
	}

	// 3. RRF merge
	merged := rrfMerge(ftsResults, vecResults, 60)

	// 4. Enrich with metadata and apply filters
	merged = s.enrichAndFilter(merged, filters)

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
// Uses MockEmbedding if no external embedding provider is configured.
func (s *Searcher) IndexDocument(docID string, doc *Document) error {
	if s.vectorStore == nil {
		return nil
	}

	text := documentToText(doc)
	if text == "" {
		return nil
	}

	emb := MockEmbedding(text, s.dimension)
	return s.vectorStore.Add(docID, emb, "mock")
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
