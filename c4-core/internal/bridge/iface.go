package bridge

import (
	"context"
	"encoding/json"
)

// Bridge defines the interface for communicating with the Python C4 system.
// Both PythonBridge (subprocess) and GRPCBridge (gRPC) implement this interface.
type Bridge interface {
	// Call sends a JSON-RPC request and returns the raw JSON response.
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)

	// IsAvailable returns true if the backend is reachable.
	IsAvailable() bool
}

// --- Domain-specific result types (pure Go, no proto dependency) ---

// SymbolResult represents a symbol found by FindSymbol or GetSymbolsOverview.
type SymbolResult struct {
	FilePath  string `json:"file_path"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	StartLine int    `json:"start_line"`
	StartCol  int    `json:"start_col"`
	EndLine   int    `json:"end_line"`
	EndCol    int    `json:"end_col"`
	Container string `json:"container,omitempty"`
}

// KnowledgeDoc represents a knowledge document.
type KnowledgeDoc struct {
	Slug       string  `json:"slug"`
	DocType    string  `json:"doc_type"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Metadata   string  `json:"metadata_json,omitempty"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
	Score      float32 `json:"score,omitempty"`
}

// GPUDeviceInfo describes a single GPU device.
type GPUDeviceInfo struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	MemoryTotalMB  int64   `json:"memory_total_mb"`
	MemoryUsedMB   int64   `json:"memory_used_mb"`
	MemoryFreeMB   int64   `json:"memory_free_mb"`
	UtilizationPct float32 `json:"utilization_pct"`
	Backend        string  `json:"backend"`
}

// ExtendedBridge extends Bridge with typed methods for LSP, Knowledge,
// and GPU operations. These avoid raw JSON marshaling at the call site.
type ExtendedBridge interface {
	Bridge

	// --- LSP operations ---

	// FindSymbol searches for a symbol by name, optionally filtered by file and kind.
	FindSymbol(ctx context.Context, name, filePath, kind string) ([]SymbolResult, error)

	// GetSymbolsOverview returns all symbols defined in a file.
	GetSymbolsOverview(ctx context.Context, filePath string) ([]SymbolResult, error)

	// ReplaceSymbolBody replaces the body of a symbol. Returns lines changed.
	ReplaceSymbolBody(ctx context.Context, filePath, symbolName, newBody string) (int, error)

	// InsertBeforeSymbol inserts content before a symbol definition.
	InsertBeforeSymbol(ctx context.Context, filePath, symbolName, content string) error

	// InsertAfterSymbol inserts content after a symbol definition.
	InsertAfterSymbol(ctx context.Context, filePath, symbolName, content string) error

	// RenameSymbol renames a symbol across all references.
	// Returns modified files and count of references updated.
	RenameSymbol(ctx context.Context, filePath, oldName, newName string) ([]string, int, error)

	// --- Knowledge operations ---

	// KnowledgeSearch performs hybrid search on knowledge documents.
	KnowledgeSearch(ctx context.Context, query, docType string, limit int) ([]KnowledgeDoc, error)

	// KnowledgeRecord creates or updates a knowledge document.
	// Returns the slug of the created/updated document.
	KnowledgeRecord(ctx context.Context, docType, title, content, metadataJSON string, tags []string) (string, error)

	// KnowledgeGet retrieves a knowledge document by slug.
	KnowledgeGet(ctx context.Context, slug string) (*KnowledgeDoc, error)

	// --- GPU operations ---

	// GPUStatus returns GPU availability and device info.
	GPUStatus(ctx context.Context) (bool, string, []GPUDeviceInfo, error)

	// JobSubmit submits a compute job. Returns job ID.
	JobSubmit(ctx context.Context, name, command, workDir string, env map[string]string, resourcesJSON string, priority int) (string, error)
}
