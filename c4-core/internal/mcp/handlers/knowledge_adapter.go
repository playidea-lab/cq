package handlers

import "github.com/changmin/c4-core/internal/knowledge"

// knowledgeStoreAdapter wraps *knowledge.Store to satisfy KnowledgeWriter and KnowledgeReader.
type knowledgeStoreAdapter struct {
	store *knowledge.Store
}

func (a *knowledgeStoreAdapter) CreateExperiment(metadata map[string]any, body string) (string, error) {
	return a.store.Create(knowledge.TypeExperiment, metadata, body)
}

func (a *knowledgeStoreAdapter) GetBody(docID string) (string, error) {
	doc, err := a.store.Get(docID)
	if err != nil {
		return "", err
	}
	return doc.Body, nil
}

// knowledgeSearcherAdapter wraps *knowledge.Searcher to satisfy KnowledgeContextSearcher.
type knowledgeSearcherAdapter struct {
	searcher *knowledge.Searcher
}

func (a *knowledgeSearcherAdapter) Search(query string, topK int, filters map[string]string) ([]KnowledgeSearchResult, error) {
	results, err := a.searcher.Search(query, topK, filters)
	if err != nil {
		return nil, err
	}
	out := make([]KnowledgeSearchResult, len(results))
	for i, r := range results {
		out[i] = KnowledgeSearchResult{
			ID:     r.ID,
			Title:  r.Title,
			Type:   r.Type,
			Domain: r.Domain,
		}
	}
	return out, nil
}

// AdaptKnowledge wraps concrete knowledge types into local interfaces for sqlite_store.
// Returns (writer, reader, searcher). Any nil input produces nil output.
func AdaptKnowledge(store *knowledge.Store, searcher *knowledge.Searcher) (KnowledgeWriter, KnowledgeReader, KnowledgeContextSearcher) {
	var w KnowledgeWriter
	var r KnowledgeReader
	var s KnowledgeContextSearcher
	if store != nil {
		a := &knowledgeStoreAdapter{store: store}
		w = a
		r = a
	}
	if searcher != nil {
		s = &knowledgeSearcherAdapter{searcher: searcher}
	}
	return w, r, s
}
