//go:build grpc

// Package bridge provides the gRPC-based bridge to the Python C4 system.
//
// This file is only compiled when the "grpc" build tag is set,
// because it depends on the generated protobuf package (pb/) which
// is produced by running `make proto-gen`.
//
// Without the build tag, the codebase uses PythonBridge (subprocess)
// as the fallback.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pb "github.com/changmin/c4-core/internal/bridge/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCBridge communicates with the Python C4 system via gRPC.
// It implements both Bridge and ExtendedBridge interfaces.
type GRPCBridge struct {
	// Address of the Python gRPC server (e.g., "localhost:50051").
	Address string
	// Timeout for individual RPC calls.
	Timeout time.Duration
	// Fallback is used when gRPC is unavailable.
	Fallback *PythonBridge

	mu     sync.Mutex
	conn   *grpc.ClientConn
	client pb.BridgeServiceClient
}

// NewGRPCBridge creates a GRPCBridge with the given address and a
// PythonBridge subprocess fallback.
func NewGRPCBridge(addr string) *GRPCBridge {
	if addr == "" {
		addr = "localhost:50051"
	}
	return &GRPCBridge{
		Address:  addr,
		Timeout:  30 * time.Second,
		Fallback: NewPythonBridge(),
	}
}

// connect establishes a gRPC connection lazily.
func (g *GRPCBridge) connect() (pb.BridgeServiceClient, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.client != nil {
		return g.client, nil
	}

	conn, err := grpc.NewClient(
		g.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", g.Address, err)
	}

	g.conn = conn
	g.client = pb.NewBridgeServiceClient(conn)
	return g.client, nil
}

// Close closes the gRPC connection.
func (g *GRPCBridge) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.conn != nil {
		err := g.conn.Close()
		g.conn = nil
		g.client = nil
		return err
	}
	return nil
}

// withTimeout wraps ctx with the bridge's timeout if one is configured.
func (g *GRPCBridge) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if g.Timeout > 0 {
		return context.WithTimeout(ctx, g.Timeout)
	}
	return ctx, func() {}
}

// IsAvailable checks if the gRPC server is reachable by attempting a
// GetState call. Falls back to checking subprocess availability.
func (g *GRPCBridge) IsAvailable() bool {
	client, err := g.connect()
	if err != nil {
		return g.Fallback != nil && g.Fallback.IsAvailable()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = client.GetState(ctx, &pb.GetStateRequest{})
	return err == nil
}

// Call sends a JSON-RPC style request via gRPC. For backward compatibility
// with PythonBridge, it maps common MCP method names to the appropriate
// gRPC calls. Unrecognized methods fall back to subprocess.
func (g *GRPCBridge) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	client, err := g.connect()
	if err != nil {
		if g.Fallback != nil {
			return g.Fallback.Call(ctx, method, params)
		}
		return nil, fmt.Errorf("grpc unavailable and no fallback: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	// Extract params as map for convenience.
	paramsMap, _ := toMap(params)

	switch method {
	case "tools/call":
		return g.dispatchToolCall(ctx, client, paramsMap)
	default:
		// Unknown method — use subprocess fallback.
		if g.Fallback != nil {
			return g.Fallback.Call(ctx, method, params)
		}
		return nil, fmt.Errorf("unknown method %q and no fallback", method)
	}
}

// dispatchToolCall routes MCP tool calls to typed gRPC RPCs.
func (g *GRPCBridge) dispatchToolCall(
	ctx context.Context,
	client pb.BridgeServiceClient,
	params map[string]any,
) (json.RawMessage, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "c4_status":
		resp, err := client.GetState(ctx, &pb.GetStateRequest{})
		if err != nil {
			return nil, fmt.Errorf("GetState: %w", err)
		}
		return json.Marshal(resp)

	case "c4_find_symbol":
		resp, err := client.FindSymbol(ctx, &pb.FindSymbolRequest{
			Name:     getStr(args, "name"),
			FilePath: getStr(args, "file_path"),
			Kind:     getStr(args, "kind"),
		})
		if err != nil {
			return nil, fmt.Errorf("FindSymbol: %w", err)
		}
		return json.Marshal(resp)

	case "c4_get_symbols_overview":
		resp, err := client.GetSymbolsOverview(ctx, &pb.GetSymbolsOverviewRequest{
			FilePath: getStr(args, "file_path"),
		})
		if err != nil {
			return nil, fmt.Errorf("GetSymbolsOverview: %w", err)
		}
		return json.Marshal(resp)

	case "c4_knowledge_search":
		resp, err := client.KnowledgeSearch(ctx, &pb.KnowledgeSearchRequest{
			Query:   getStr(args, "query"),
			DocType: getStr(args, "doc_type"),
			Limit:   getInt32(args, "limit"),
		})
		if err != nil {
			return nil, fmt.Errorf("KnowledgeSearch: %w", err)
		}
		return json.Marshal(resp)

	case "c4_gpu_status":
		resp, err := client.GPUStatus(ctx, &pb.GPUStatusRequest{})
		if err != nil {
			return nil, fmt.Errorf("GPUStatus: %w", err)
		}
		return json.Marshal(resp)

	default:
		// Unknown tool — fall back to subprocess.
		if g.Fallback != nil {
			return g.Fallback.Call(ctx, "tools/call", params)
		}
		return nil, fmt.Errorf("unknown tool %q and no fallback", toolName)
	}
}

// --- ExtendedBridge typed methods ---

// FindSymbol searches for a symbol by name.
func (g *GRPCBridge) FindSymbol(ctx context.Context, name, filePath, kind string) ([]SymbolResult, error) {
	client, err := g.connect()
	if err != nil {
		return nil, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.FindSymbol(ctx, &pb.FindSymbolRequest{
		Name:     name,
		FilePath: filePath,
		Kind:     kind,
	})
	if err != nil {
		return nil, fmt.Errorf("FindSymbol: %w", err)
	}

	return convertSymbols(resp.Symbols), nil
}

// GetSymbolsOverview returns all symbols in a file.
func (g *GRPCBridge) GetSymbolsOverview(ctx context.Context, filePath string) ([]SymbolResult, error) {
	client, err := g.connect()
	if err != nil {
		return nil, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.GetSymbolsOverview(ctx, &pb.GetSymbolsOverviewRequest{
		FilePath: filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("GetSymbolsOverview: %w", err)
	}

	return convertSymbols(resp.Symbols), nil
}

// ReplaceSymbolBody replaces the body of a symbol.
func (g *GRPCBridge) ReplaceSymbolBody(ctx context.Context, filePath, symbolName, newBody string) (int, error) {
	client, err := g.connect()
	if err != nil {
		return 0, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.ReplaceSymbolBody(ctx, &pb.ReplaceSymbolBodyRequest{
		FilePath:   filePath,
		SymbolName: symbolName,
		NewBody:    newBody,
	})
	if err != nil {
		return 0, fmt.Errorf("ReplaceSymbolBody: %w", err)
	}
	if resp.Error != "" {
		return 0, fmt.Errorf("ReplaceSymbolBody: %s", resp.Error)
	}

	return int(resp.LinesChanged), nil
}

// InsertBeforeSymbol inserts content before a symbol.
func (g *GRPCBridge) InsertBeforeSymbol(ctx context.Context, filePath, symbolName, content string) error {
	client, err := g.connect()
	if err != nil {
		return fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.InsertBeforeSymbol(ctx, &pb.InsertSymbolRequest{
		FilePath:   filePath,
		SymbolName: symbolName,
		Content:    content,
	})
	if err != nil {
		return fmt.Errorf("InsertBeforeSymbol: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("InsertBeforeSymbol: %s", resp.Error)
	}

	return nil
}

// InsertAfterSymbol inserts content after a symbol.
func (g *GRPCBridge) InsertAfterSymbol(ctx context.Context, filePath, symbolName, content string) error {
	client, err := g.connect()
	if err != nil {
		return fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.InsertAfterSymbol(ctx, &pb.InsertSymbolRequest{
		FilePath:   filePath,
		SymbolName: symbolName,
		Content:    content,
	})
	if err != nil {
		return fmt.Errorf("InsertAfterSymbol: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("InsertAfterSymbol: %s", resp.Error)
	}

	return nil
}

// RenameSymbol renames a symbol across all references.
func (g *GRPCBridge) RenameSymbol(ctx context.Context, filePath, oldName, newName string) ([]string, int, error) {
	client, err := g.connect()
	if err != nil {
		return nil, 0, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.RenameSymbol(ctx, &pb.RenameSymbolRequest{
		FilePath: filePath,
		OldName:  oldName,
		NewName:  newName,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("RenameSymbol: %w", err)
	}
	if resp.Error != "" {
		return nil, 0, fmt.Errorf("RenameSymbol: %s", resp.Error)
	}

	return resp.ModifiedFiles, int(resp.ReferencesUpdated), nil
}

// KnowledgeSearch searches knowledge documents.
func (g *GRPCBridge) KnowledgeSearch(ctx context.Context, query, docType string, limit int) ([]KnowledgeDoc, error) {
	client, err := g.connect()
	if err != nil {
		return nil, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.KnowledgeSearch(ctx, &pb.KnowledgeSearchRequest{
		Query:   query,
		DocType: docType,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("KnowledgeSearch: %w", err)
	}

	docs := make([]KnowledgeDoc, 0, len(resp.Documents))
	for _, d := range resp.Documents {
		docs = append(docs, convertKnowledgeDoc(d))
	}
	return docs, nil
}

// KnowledgeRecord creates or updates a knowledge document.
func (g *GRPCBridge) KnowledgeRecord(ctx context.Context, docType, title, content, metadataJSON string, tags []string) (string, error) {
	client, err := g.connect()
	if err != nil {
		return "", fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.KnowledgeRecord(ctx, &pb.KnowledgeRecordRequest{
		DocType:      docType,
		Title:        title,
		Content:      content,
		MetadataJson: metadataJSON,
		Tags:         tags,
	})
	if err != nil {
		return "", fmt.Errorf("KnowledgeRecord: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("KnowledgeRecord: %s", resp.Error)
	}

	return resp.Slug, nil
}

// KnowledgeGet retrieves a knowledge document by slug.
func (g *GRPCBridge) KnowledgeGet(ctx context.Context, slug string) (*KnowledgeDoc, error) {
	client, err := g.connect()
	if err != nil {
		return nil, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.KnowledgeGet(ctx, &pb.KnowledgeGetRequest{
		Slug: slug,
	})
	if err != nil {
		return nil, fmt.Errorf("KnowledgeGet: %w", err)
	}
	if !resp.Found {
		return nil, nil
	}

	doc := convertKnowledgeDoc(resp.Document)
	return &doc, nil
}

// GPUStatus returns GPU availability and device info.
func (g *GRPCBridge) GPUStatus(ctx context.Context) (bool, string, []GPUDeviceInfo, error) {
	client, err := g.connect()
	if err != nil {
		return false, "", nil, fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.GPUStatus(ctx, &pb.GPUStatusRequest{})
	if err != nil {
		return false, "", nil, fmt.Errorf("GPUStatus: %w", err)
	}

	devices := make([]GPUDeviceInfo, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		devices = append(devices, GPUDeviceInfo{
			Index:          int(d.Index),
			Name:           d.Name,
			MemoryTotalMB:  d.MemoryTotalMb,
			MemoryUsedMB:   d.MemoryUsedMb,
			MemoryFreeMB:   d.MemoryFreeMb,
			UtilizationPct: d.UtilizationPct,
			Backend:        d.Backend,
		})
	}

	return resp.Available, resp.Backend, devices, nil
}

// JobSubmit submits a compute job.
func (g *GRPCBridge) JobSubmit(ctx context.Context, name, command, workDir string, env map[string]string, resourcesJSON string, priority int) (string, error) {
	client, err := g.connect()
	if err != nil {
		return "", fmt.Errorf("grpc unavailable: %w", err)
	}

	ctx, cancel := g.withTimeout(ctx)
	defer cancel()

	resp, err := client.JobSubmit(ctx, &pb.JobSubmitRequest{
		Name:          name,
		Command:       command,
		WorkingDir:    workDir,
		Env:           env,
		ResourcesJson: resourcesJSON,
		Priority:      int32(priority),
	})
	if err != nil {
		return "", fmt.Errorf("JobSubmit: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("JobSubmit: %s", resp.Error)
	}

	return resp.JobId, nil
}

// --- Helpers ---

// convertSymbols converts proto SymbolLocation slice to Go SymbolResult slice.
func convertSymbols(symbols []*pb.SymbolLocation) []SymbolResult {
	result := make([]SymbolResult, 0, len(symbols))
	for _, s := range symbols {
		result = append(result, SymbolResult{
			FilePath:  s.FilePath,
			Name:      s.Name,
			Kind:      s.Kind,
			StartLine: int(s.StartLine),
			StartCol:  int(s.StartCol),
			EndLine:   int(s.EndLine),
			EndCol:    int(s.EndCol),
			Container: s.Container,
		})
	}
	return result
}

// convertKnowledgeDoc converts a proto KnowledgeDocument to Go KnowledgeDoc.
func convertKnowledgeDoc(d *pb.KnowledgeDocument) KnowledgeDoc {
	if d == nil {
		return KnowledgeDoc{}
	}
	return KnowledgeDoc{
		Slug:      d.Slug,
		DocType:   d.DocType,
		Title:     d.Title,
		Content:   d.Content,
		Metadata:  d.MetadataJson,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		Score:     d.Score,
	}
}

// toMap converts an arbitrary value to map[string]any via JSON roundtrip.
func toMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

// getStr extracts a string from a map, returning "" if absent or wrong type.
func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

// getInt32 extracts an int32 from a map, handling float64 (JSON default).
func getInt32(m map[string]any, key string) int32 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int32(v)
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	default:
		return 0
	}
}
