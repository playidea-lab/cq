//go:build grpc

package bridge

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/changmin/c4-core/internal/bridge/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockBridgeServer implements pb.BridgeServiceServer for testing.
type mockBridgeServer struct {
	pb.UnimplementedBridgeServiceServer
}

func (m *mockBridgeServer) GetState(_ context.Context, _ *pb.GetStateRequest) (*pb.GetStateResponse, error) {
	return &pb.GetStateResponse{
		State:        "EXECUTE",
		ProjectName:  "test-project",
		TotalTasks:   10,
		PendingTasks: 3,
		DoneTasks:    5,
		BlockedTasks: 2,
	}, nil
}

func (m *mockBridgeServer) FindSymbol(_ context.Context, req *pb.FindSymbolRequest) (*pb.FindSymbolResponse, error) {
	return &pb.FindSymbolResponse{
		Symbols: []*pb.SymbolLocation{
			{
				FilePath:  "main.go",
				Name:      req.Name,
				Kind:      "function",
				StartLine: 10,
				StartCol:  0,
				EndLine:   20,
				EndCol:    1,
			},
		},
	}, nil
}

func (m *mockBridgeServer) GetSymbolsOverview(_ context.Context, req *pb.GetSymbolsOverviewRequest) (*pb.GetSymbolsOverviewResponse, error) {
	return &pb.GetSymbolsOverviewResponse{
		Symbols: []*pb.SymbolLocation{
			{
				FilePath:  req.FilePath,
				Name:      "main",
				Kind:      "function",
				StartLine: 1,
				EndLine:   10,
			},
			{
				FilePath:  req.FilePath,
				Name:      "Config",
				Kind:      "struct",
				StartLine: 12,
				EndLine:   18,
			},
		},
	}, nil
}

func (m *mockBridgeServer) ReplaceSymbolBody(_ context.Context, _ *pb.ReplaceSymbolBodyRequest) (*pb.ReplaceSymbolBodyResponse, error) {
	return &pb.ReplaceSymbolBodyResponse{
		Success:      true,
		LinesChanged: 5,
	}, nil
}

func (m *mockBridgeServer) InsertBeforeSymbol(_ context.Context, _ *pb.InsertSymbolRequest) (*pb.InsertSymbolResponse, error) {
	return &pb.InsertSymbolResponse{Success: true}, nil
}

func (m *mockBridgeServer) InsertAfterSymbol(_ context.Context, _ *pb.InsertSymbolRequest) (*pb.InsertSymbolResponse, error) {
	return &pb.InsertSymbolResponse{Success: true}, nil
}

func (m *mockBridgeServer) RenameSymbol(_ context.Context, _ *pb.RenameSymbolRequest) (*pb.RenameSymbolResponse, error) {
	return &pb.RenameSymbolResponse{
		Success:           true,
		ModifiedFiles:     []string{"main.go", "config.go"},
		ReferencesUpdated: 7,
	}, nil
}

func (m *mockBridgeServer) KnowledgeSearch(_ context.Context, req *pb.KnowledgeSearchRequest) (*pb.KnowledgeSearchResponse, error) {
	return &pb.KnowledgeSearchResponse{
		Documents: []*pb.KnowledgeDocument{
			{
				Slug:    "ins-test",
				DocType: "insight",
				Title:   "Test Insight",
				Content: "Matches: " + req.Query,
				Score:   0.95,
			},
		},
		TotalCount: 1,
	}, nil
}

func (m *mockBridgeServer) KnowledgeRecord(_ context.Context, _ *pb.KnowledgeRecordRequest) (*pb.KnowledgeRecordResponse, error) {
	return &pb.KnowledgeRecordResponse{
		Success: true,
		Slug:    "ins-new-doc",
	}, nil
}

func (m *mockBridgeServer) KnowledgeGet(_ context.Context, req *pb.KnowledgeGetRequest) (*pb.KnowledgeGetResponse, error) {
	if req.Slug == "missing" {
		return &pb.KnowledgeGetResponse{Found: false}, nil
	}
	return &pb.KnowledgeGetResponse{
		Found: true,
		Document: &pb.KnowledgeDocument{
			Slug:    req.Slug,
			DocType: "pattern",
			Title:   "Test Pattern",
			Content: "Pattern content",
		},
	}, nil
}

func (m *mockBridgeServer) GPUStatus(_ context.Context, _ *pb.GPUStatusRequest) (*pb.GPUStatusResponse, error) {
	return &pb.GPUStatusResponse{
		Available: true,
		Backend:   "mps",
		Devices: []*pb.GPUDevice{
			{
				Index:          0,
				Name:           "Apple M1 Pro",
				MemoryTotalMb:  16384,
				MemoryUsedMb:   4096,
				MemoryFreeMb:   12288,
				UtilizationPct: 25.0,
				Backend:        "mps",
			},
		},
	}, nil
}

func (m *mockBridgeServer) JobSubmit(_ context.Context, _ *pb.JobSubmitRequest) (*pb.JobSubmitResponse, error) {
	return &pb.JobSubmitResponse{
		Success: true,
		JobId:   "job-001",
		Status:  "queued",
	}, nil
}

// startMockServer starts a gRPC server on a random port and returns
// the address and a cleanup function.
func startMockServer(t *testing.T) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &mockBridgeServer{})

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Server stopped.
		}
	}()

	return lis.Addr().String(), func() {
		srv.GracefulStop()
	}
}

func TestGRPCBridgeIsAvailable(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	if !b.IsAvailable() {
		t.Error("expected bridge to be available")
	}
}

func TestGRPCBridgeIsAvailableFallback(t *testing.T) {
	// Connect to a port with no server.
	b := NewGRPCBridge("localhost:0")
	b.Fallback = &PythonBridge{Command: "sh"} // sh is always available
	defer b.Close()

	// gRPC will fail, but IsAvailable checks fallback too.
	// The gRPC health check will fail since no server is listening.
	// This just validates no panic.
	_ = b.IsAvailable()
}

func TestGRPCBridgeFindSymbol(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	ctx := context.Background()
	symbols, err := b.FindSymbol(ctx, "main", "", "")
	if err != nil {
		t.Fatalf("FindSymbol error: %v", err)
	}

	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].Name != "main" {
		t.Errorf("symbol name = %q, want %q", symbols[0].Name, "main")
	}
	if symbols[0].Kind != "function" {
		t.Errorf("symbol kind = %q, want %q", symbols[0].Kind, "function")
	}
}

func TestGRPCBridgeGetSymbolsOverview(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	symbols, err := b.GetSymbolsOverview(context.Background(), "main.go")
	if err != nil {
		t.Fatalf("GetSymbolsOverview error: %v", err)
	}

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestGRPCBridgeReplaceSymbolBody(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	lines, err := b.ReplaceSymbolBody(context.Background(), "main.go", "main", "new body")
	if err != nil {
		t.Fatalf("ReplaceSymbolBody error: %v", err)
	}
	if lines != 5 {
		t.Errorf("lines changed = %d, want 5", lines)
	}
}

func TestGRPCBridgeInsertSymbol(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	ctx := context.Background()

	if err := b.InsertBeforeSymbol(ctx, "main.go", "main", "// comment"); err != nil {
		t.Fatalf("InsertBeforeSymbol error: %v", err)
	}

	if err := b.InsertAfterSymbol(ctx, "main.go", "main", "// comment"); err != nil {
		t.Fatalf("InsertAfterSymbol error: %v", err)
	}
}

func TestGRPCBridgeRenameSymbol(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	files, refs, err := b.RenameSymbol(context.Background(), "main.go", "old", "new")
	if err != nil {
		t.Fatalf("RenameSymbol error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("modified files = %d, want 2", len(files))
	}
	if refs != 7 {
		t.Errorf("references updated = %d, want 7", refs)
	}
}

func TestGRPCBridgeKnowledgeSearch(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	docs, err := b.KnowledgeSearch(context.Background(), "test query", "", 10)
	if err != nil {
		t.Fatalf("KnowledgeSearch error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Slug != "ins-test" {
		t.Errorf("slug = %q, want %q", docs[0].Slug, "ins-test")
	}
	if docs[0].Score < 0.9 {
		t.Errorf("score = %f, want >= 0.9", docs[0].Score)
	}
}

func TestGRPCBridgeKnowledgeRecord(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	slug, err := b.KnowledgeRecord(context.Background(), "insight", "Title", "Content", "{}", []string{"tag1"})
	if err != nil {
		t.Fatalf("KnowledgeRecord error: %v", err)
	}
	if slug != "ins-new-doc" {
		t.Errorf("slug = %q, want %q", slug, "ins-new-doc")
	}
}

func TestGRPCBridgeKnowledgeGet(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	ctx := context.Background()

	// Found case.
	doc, err := b.KnowledgeGet(ctx, "pat-test")
	if err != nil {
		t.Fatalf("KnowledgeGet error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if doc.Slug != "pat-test" {
		t.Errorf("slug = %q, want %q", doc.Slug, "pat-test")
	}

	// Not found case.
	doc, err = b.KnowledgeGet(ctx, "missing")
	if err != nil {
		t.Fatalf("KnowledgeGet error for missing: %v", err)
	}
	if doc != nil {
		t.Errorf("expected nil for missing doc, got %+v", doc)
	}
}

func TestGRPCBridgeGPUStatus(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	available, backend, devices, err := b.GPUStatus(context.Background())
	if err != nil {
		t.Fatalf("GPUStatus error: %v", err)
	}
	if !available {
		t.Error("expected GPU available")
	}
	if backend != "mps" {
		t.Errorf("backend = %q, want %q", backend, "mps")
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Name != "Apple M1 Pro" {
		t.Errorf("device name = %q, want %q", devices[0].Name, "Apple M1 Pro")
	}
}

func TestGRPCBridgeJobSubmit(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	jobID, err := b.JobSubmit(context.Background(), "train", "python train.py", "/work", nil, "{}", 1)
	if err != nil {
		t.Fatalf("JobSubmit error: %v", err)
	}
	if jobID != "job-001" {
		t.Errorf("job ID = %q, want %q", jobID, "job-001")
	}
}

func TestGRPCBridgeCallDispatch(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)
	defer b.Close()

	ctx := context.Background()

	// Dispatch c4_status via Call.
	result, err := b.Call(ctx, "tools/call", map[string]any{
		"name":      "c4_status",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("Call c4_status error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGRPCBridgeClose(t *testing.T) {
	addr, cleanup := startMockServer(t)
	defer cleanup()

	b := NewGRPCBridge(addr)

	// Force connection.
	_, _ = b.connect()

	if err := b.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Double close should be safe.
	if err := b.Close(); err != nil {
		t.Fatalf("Double close error: %v", err)
	}
}

func TestGRPCBridgeNewDefault(t *testing.T) {
	b := NewGRPCBridge("")
	if b.Address != "localhost:50051" {
		t.Errorf("Address = %q, want %q", b.Address, "localhost:50051")
	}
	if b.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", b.Timeout)
	}
	if b.Fallback == nil {
		t.Error("expected non-nil Fallback")
	}
}

func TestHelperGetStr(t *testing.T) {
	m := map[string]any{
		"name": "hello",
		"num":  42.0,
	}

	if got := getStr(m, "name"); got != "hello" {
		t.Errorf("getStr(name) = %q, want %q", got, "hello")
	}
	if got := getStr(m, "missing"); got != "" {
		t.Errorf("getStr(missing) = %q, want empty", got)
	}
	if got := getStr(m, "num"); got != "" {
		t.Errorf("getStr(num) = %q, want empty (wrong type)", got)
	}
	if got := getStr(nil, "any"); got != "" {
		t.Errorf("getStr(nil) = %q, want empty", got)
	}
}

func TestHelperGetInt32(t *testing.T) {
	m := map[string]any{
		"float": 42.0,
		"int":   10,
		"str":   "hello",
	}

	if got := getInt32(m, "float"); got != 42 {
		t.Errorf("getInt32(float) = %d, want 42", got)
	}
	if got := getInt32(m, "int"); got != 10 {
		t.Errorf("getInt32(int) = %d, want 10", got)
	}
	if got := getInt32(m, "str"); got != 0 {
		t.Errorf("getInt32(str) = %d, want 0", got)
	}
	if got := getInt32(nil, "any"); got != 0 {
		t.Errorf("getInt32(nil) = %d, want 0", got)
	}
}

func TestHelperToMap(t *testing.T) {
	// nil input.
	m, err := toMap(nil)
	if err != nil || m != nil {
		t.Errorf("toMap(nil) = %v, %v; want nil, nil", m, err)
	}

	// Already a map.
	input := map[string]any{"key": "val"}
	m, err = toMap(input)
	if err != nil {
		t.Fatalf("toMap(map) error: %v", err)
	}
	if m["key"] != "val" {
		t.Errorf("toMap(map) key = %v, want %q", m["key"], "val")
	}

	// Struct input (JSON roundtrip).
	type S struct {
		Name string `json:"name"`
	}
	m, err = toMap(S{Name: "test"})
	if err != nil {
		t.Fatalf("toMap(struct) error: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("toMap(struct) name = %v, want %q", m["name"], "test")
	}
}

// TestGRPCBridgeImplementsBridge verifies interface compliance at compile time.
func TestGRPCBridgeImplementsBridge(t *testing.T) {
	var _ Bridge = (*GRPCBridge)(nil)
	var _ ExtendedBridge = (*GRPCBridge)(nil)
}

// TestGRPCBridgeConnectFailure verifies behavior when connecting to a non-existent server.
func TestGRPCBridgeConnectFailure(t *testing.T) {
	b := NewGRPCBridge("localhost:0")
	b.Fallback = nil
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// connect() itself may succeed (gRPC lazy connects), but RPC will fail.
	_, err := b.FindSymbol(ctx, "test", "", "")
	if err == nil {
		t.Error("expected error when connecting to non-existent server")
	}
}

// TestPythonBridgeImplementsBridge verifies PythonBridge satisfies Bridge interface.
func TestPythonBridgeImplementsBridge(t *testing.T) {
	var _ Bridge = (*PythonBridge)(nil)
}

// BenchmarkGRPCBridgeRoundtrip benchmarks a FindSymbol round-trip through gRPC.
func BenchmarkGRPCBridgeRoundtrip(b *testing.B) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &mockBridgeServer{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	addr := lis.Addr().String()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewBridgeServiceClient(conn)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.FindSymbol(ctx, &pb.FindSymbolRequest{Name: "test"})
		if err != nil {
			b.Fatalf("FindSymbol: %v", err)
		}
	}
}
