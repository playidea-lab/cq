package handlers

import "encoding/json"

// --- Local interfaces for sqlite_store decoupling ---
// These allow sqlite_store.go to avoid direct imports of eventbus and knowledge packages.

// EventPublisher is a local interface for async event publishing.
// Satisfied by eventbus.Client, eventbus.NoopPublisher, and any
// type implementing PublishAsync.
type EventPublisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}

// EventDispatcher is a local interface for local rule-based event dispatch.
// Satisfied by *eventbus.Dispatcher.
type EventDispatcher interface {
	Dispatch(eventID, eventType string, eventData json.RawMessage)
}

// KnowledgeWriter creates knowledge documents (used by auto-record).
type KnowledgeWriter interface {
	CreateExperiment(metadata map[string]any, body string) (string, error)
}

// KnowledgeReader reads knowledge document bodies (used by context injection).
type KnowledgeReader interface {
	GetBody(docID string) (string, error)
}

// KnowledgeContextSearcher searches knowledge for context injection.
type KnowledgeContextSearcher interface {
	Search(query string, topK int, filters map[string]string) ([]KnowledgeSearchResult, error)
}

// KnowledgeSearchResult is a minimal representation of a knowledge search result.
type KnowledgeSearchResult struct {
	ID     string
	Title  string
	Type   string
	Domain string
}
