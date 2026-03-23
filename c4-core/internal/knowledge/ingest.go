package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IngestOpts configures document ingestion.
type IngestOpts struct {
	Title      string      // optional override; defaults to filename
	Tags       []string    // optional tags
	Visibility string      // private, team, public (default: team)
	MaxTokens  int         // chunk size in tokens (default: 512)
	Cloud      CloudSyncer // optional: sync chunks to cloud after indexing
}

// IngestResult holds the outcome of a document ingestion.
type IngestResult struct {
	DocID      string `json:"doc_id"`
	Title      string `json:"title"`
	ChunkCount int    `json:"chunk_count"`
	BodyLen    int    `json:"body_length"`
}

// Ingest reads a text file, chunks it, and stores each chunk as an indexed document.
// The parent document is stored with the full body, and chunks are stored in the
// local vector store for semantic search.
func Ingest(store *Store, searcher *Searcher, filePath string, opts IngestOpts) (*IngestResult, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	body := string(data)
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("file is empty: %s", filePath)
	}

	// Determine title
	title := opts.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	visibility := opts.Visibility
	if visibility == "" {
		visibility = "team"
	}

	// Create parent document
	metadata := map[string]any{
		"title":      title,
		"tags":       opts.Tags,
		"visibility": visibility,
		"domain":     "ingested",
	}

	docID, err := store.Create(TypeInsight, metadata, body)
	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	// Chunk the body
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	chunks := ChunkText(body, maxTokens)

	// Batch-index all chunks + parent document
	if searcher != nil && searcher.vectorStore != nil {
		var batchIDs []string
		var batchDocs []*Document

		for _, chunk := range chunks {
			chunkID := fmt.Sprintf("%s-chunk-%d", docID, chunk.Index)
			batchIDs = append(batchIDs, chunkID)
			batchDocs = append(batchDocs, &Document{
				ID:        chunkID,
				Type:      TypeInsight,
				Title:     fmt.Sprintf("%s (chunk %d)", title, chunk.Index),
				Body:      chunk.Body,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}

		// Add parent document
		parentDoc, _ := store.Get(docID)
		if parentDoc != nil {
			batchIDs = append(batchIDs, docID)
			batchDocs = append(batchDocs, parentDoc)
		}

		if err := searcher.BatchIndexDocuments(batchIDs, batchDocs); err != nil {
			fmt.Fprintf(os.Stderr, "c4: batch index: %v\n", err)
		}

		collectAndSyncChunks(opts.Cloud, searcher, docID, title, visibility, chunks)
	} else if searcher != nil {
		// FTS-only mode: index parent for metadata enrichment
		doc, _ := store.Get(docID)
		if doc != nil {
			searcher.IndexDocument(docID, doc)
		}
	}

	return &IngestResult{
		DocID:      docID,
		Title:      title,
		ChunkCount: len(chunks),
		BodyLen:    len(body),
	}, nil
}

// IngestText ingests text content directly (without reading from file).
func IngestText(store *Store, searcher *Searcher, content string, opts IngestOpts) (*IngestResult, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content is empty")
	}

	title := opts.Title
	if title == "" {
		title = "Untitled Document"
	}

	visibility := opts.Visibility
	if visibility == "" {
		visibility = "team"
	}

	metadata := map[string]any{
		"title":      title,
		"tags":       opts.Tags,
		"visibility": visibility,
		"domain":     "ingested",
	}

	docID, err := store.Create(TypeInsight, metadata, content)
	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	chunks := ChunkText(content, maxTokens)

	// Batch-index chunks + parent
	if searcher != nil && searcher.vectorStore != nil {
		var batchIDs []string
		var batchDocs []*Document

		for _, chunk := range chunks {
			chunkID := fmt.Sprintf("%s-chunk-%d", docID, chunk.Index)
			batchIDs = append(batchIDs, chunkID)
			batchDocs = append(batchDocs, &Document{
				ID:    chunkID,
				Title: fmt.Sprintf("%s (chunk %d)", title, chunk.Index),
				Body:  chunk.Body,
			})
		}

		parentDoc, _ := store.Get(docID)
		if parentDoc != nil {
			batchIDs = append(batchIDs, docID)
			batchDocs = append(batchDocs, parentDoc)
		}

		if err := searcher.BatchIndexDocuments(batchIDs, batchDocs); err != nil {
			fmt.Fprintf(os.Stderr, "c4: batch index: %v\n", err)
		}

		collectAndSyncChunks(opts.Cloud, searcher, docID, title, visibility, chunks)
	} else if searcher != nil {
		doc, _ := store.Get(docID)
		if doc != nil {
			searcher.IndexDocument(docID, doc)
		}
	}

	return &IngestResult{
		DocID:      docID,
		Title:      title,
		ChunkCount: len(chunks),
		BodyLen:    len(content),
	}, nil
}

// collectAndSyncChunks builds IngestChunk slice and syncs to cloud.
func collectAndSyncChunks(cloud CloudSyncer, searcher *Searcher, docID, title, visibility string, chunks []Chunk) {
	if cloud == nil {
		return
	}
	var ingestChunks []IngestChunk
	for _, chunk := range chunks {
		chunkID := fmt.Sprintf("%s-chunk-%d", docID, chunk.Index)
		ic := IngestChunk{Index: chunk.Index, Body: chunk.Body}
		if searcher != nil && searcher.vectorStore != nil {
			ic.Embedding = searcher.vectorStore.Get(chunkID)
		}
		ingestChunks = append(ingestChunks, ic)
	}
	SyncIngestChunks(cloud, docID, title, visibility, ingestChunks)
}
