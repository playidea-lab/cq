// Package apps implements the MCP Apps infrastructure for serving ui:// resources.
//
// MCP Apps spec: tool responses include _meta.ui.resourceUri pointing to a ui:// URI.
// Clients request that URI via resources/read, and this package returns the HTML content.
package apps

import (
	"fmt"
	"strings"
	"sync"
)

// UIResource represents a registered UI resource with its content.
type UIResource struct {
	Uri         string // e.g. "ui://c4/task-card"
	Content     string // HTML content
	ContentType string // MIME type, defaults to "text/html"
}

// ResourceStore holds registered ui:// resources.
type ResourceStore struct {
	mu        sync.RWMutex
	resources map[string]UIResource
}

// NewResourceStore creates an empty ResourceStore.
func NewResourceStore() *ResourceStore {
	return &ResourceStore{
		resources: make(map[string]UIResource),
	}
}

// Register stores a UI resource under the given uri.
// If ContentType is empty, "text/html" is used.
func (s *ResourceStore) Register(uri, html string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[uri] = UIResource{
		Uri:         uri,
		Content:     html,
		ContentType: "text/html",
	}
}

// Get retrieves a registered UI resource by URI.
// Returns the resource and true on success, or a zero value and false if not found.
func (s *ResourceStore) Get(uri string) (UIResource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.resources[uri]
	return r, ok
}

// HandleResourcesRead handles a resources/read request for ui:// URIs.
// Returns (content, mimeType, error). The caller is responsible for encoding the response
// in the MCP resources/read wire format.
func (s *ResourceStore) HandleResourcesRead(uri string) (string, string, error) {
	if !strings.HasPrefix(uri, "ui://") {
		return "", "", fmt.Errorf("unsupported scheme in URI %q: only ui:// is handled", uri)
	}
	r, ok := s.Get(uri)
	if !ok {
		return "", "", fmt.Errorf("ui resource not found: %s", uri)
	}
	return r.Content, r.ContentType, nil
}
