package goast

import (
	"os"
	"path/filepath"
	"testing"
)

const testSrc = `package example

import "errors"

// Server handles HTTP requests.
type Server struct {
	addr string
	port int
}

// Handler is the request handler interface.
type Handler interface {
	ServeHTTP(w Writer, r *Request)
}

// StringAlias is a type alias.
type StringAlias = string

const DefaultPort = 8080

var ErrNotFound = errors.New("not found")

// NewServer creates a new server.
func NewServer(addr string, port int) *Server {
	return &Server{addr: addr, port: port}
}

// Start starts the server.
func (s *Server) Start() error {
	return nil
}

// Stop stops the server gracefully.
func (s *Server) Stop(timeout int) error {
	return nil
}
`

func writeTempGoFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFile(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	byKind := map[string]int{}
	for _, s := range symbols {
		byKind[s.Kind]++
	}

	if byKind["struct"] != 1 {
		t.Errorf("expected 1 struct, got %d", byKind["struct"])
	}
	if byKind["interface"] != 1 {
		t.Errorf("expected 1 interface, got %d", byKind["interface"])
	}
	if byKind["type"] != 1 {
		t.Errorf("expected 1 type alias, got %d", byKind["type"])
	}
	if byKind["const"] != 1 {
		t.Errorf("expected 1 const, got %d", byKind["const"])
	}
	if byKind["var"] != 1 {
		t.Errorf("expected 1 var, got %d", byKind["var"])
	}
	if byKind["function"] != 1 {
		t.Errorf("expected 1 function, got %d", byKind["function"])
	}
	if byKind["method"] != 2 {
		t.Errorf("expected 2 methods, got %d", byKind["method"])
	}
}

func TestParseFile_SymbolDetails(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Find Server struct
	var server *Symbol
	for i := range symbols {
		if symbols[i].Name == "Server" && symbols[i].Kind == "struct" {
			server = &symbols[i]
			break
		}
	}
	if server == nil {
		t.Fatal("Server struct not found")
	}
	if server.Doc != "Server handles HTTP requests." {
		t.Errorf("unexpected doc: %q", server.Doc)
	}
	if server.FullName != "example.Server" {
		t.Errorf("unexpected full_name: %q", server.FullName)
	}
	if server.Description != "type Server struct (2 fields)" {
		t.Errorf("unexpected description: %q", server.Description)
	}
	if server.ParentName != "example" {
		t.Errorf("unexpected parent_name: %q", server.ParentName)
	}

	// Find NewServer function
	var newServer *Symbol
	for i := range symbols {
		if symbols[i].Name == "NewServer" {
			newServer = &symbols[i]
			break
		}
	}
	if newServer == nil {
		t.Fatal("NewServer function not found")
	}
	if newServer.Kind != "function" {
		t.Errorf("expected function, got %s", newServer.Kind)
	}
	if newServer.Signature != "func NewServer(addr string, port int) *Server" {
		t.Errorf("unexpected signature: %q", newServer.Signature)
	}

	// Find Start method
	var start *Symbol
	for i := range symbols {
		if symbols[i].Name == "Start" {
			start = &symbols[i]
			break
		}
	}
	if start == nil {
		t.Fatal("Start method not found")
	}
	if start.Kind != "method" {
		t.Errorf("expected method, got %s", start.Kind)
	}
	if start.Receiver != "*Server" {
		t.Errorf("unexpected receiver: %q", start.Receiver)
	}
	if start.ParentName != "Server" {
		t.Errorf("expected parent Server, got %q", start.ParentName)
	}
}

func TestFindSymbolByName_Simple(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	results, err := FindSymbolByName("Server", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Kind != "struct" {
		t.Errorf("expected struct, got %s", results[0].Kind)
	}
}

func TestFindSymbolByName_WithParent(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	// "Server/Start" or "Server.Start" pattern
	results, err := FindSymbolByName("Server/Start", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for Server/Start, got %d", len(results))
	}
	if results[0].Kind != "method" {
		t.Errorf("expected method, got %s", results[0].Kind)
	}

	// Dot notation
	results2, err := FindSymbolByName("Server.Stop", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) != 1 {
		t.Fatalf("expected 1 result for Server.Stop, got %d", len(results2))
	}
}

func TestFindSymbolByName_NoMatch(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	results, err := FindSymbolByName("NonExistent", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSymbolsOverview(t *testing.T) {
	path := writeTempGoFile(t, testSrc)

	result, err := SymbolsOverview(path)
	if err != nil {
		t.Fatal(err)
	}

	if result["file"] != path {
		t.Errorf("unexpected file: %v", result["file"])
	}

	funcs, ok := result["functions"].([]map[string]any)
	if !ok || len(funcs) != 1 {
		t.Errorf("expected 1 function, got %v", result["functions"])
	}

	methods, ok := result["methods"].([]map[string]any)
	if !ok || len(methods) != 2 {
		t.Errorf("expected 2 methods, got %v", result["methods"])
	}

	structs, ok := result["structs"].([]map[string]any)
	if !ok || len(structs) != 1 {
		t.Errorf("expected 1 struct, got %v", result["structs"])
	}

	ifaces, ok := result["interfaces"].([]map[string]any)
	if !ok || len(ifaces) != 1 {
		t.Errorf("expected 1 interface, got %v", result["interfaces"])
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()

	// Create two Go files
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package a
func FuncA() {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	if err := os.WriteFile(filepath.Join(sub, "b.go"), []byte(`package b
func FuncB() {}
type MyStruct struct{}
`), 0644); err != nil {
		t.Fatal(err)
	}

	symbols, err := ParseDir(dir, 100)
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["FuncA"] {
		t.Error("FuncA not found")
	}
	if !names["FuncB"] {
		t.Error("FuncB not found")
	}
	if !names["MyStruct"] {
		t.Error("MyStruct not found")
	}
}

func TestParseDirMaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 5 Go files
	for i := 0; i < 5; i++ {
		content := []byte("package a\nfunc F" + string(rune('A'+i)) + "() {}\n")
		if err := os.WriteFile(filepath.Join(dir, "f"+string(rune('0'+i))+".go"), content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	symbols, err := ParseDir(dir, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Should only have symbols from 2 files
	if len(symbols) > 2 {
		t.Errorf("expected at most 2 symbols (maxFiles=2), got %d", len(symbols))
	}
}

func TestHasGoFiles(t *testing.T) {
	dir := t.TempDir()

	// Empty dir
	if HasGoFiles(dir) {
		t.Error("expected false for empty dir")
	}

	// Dir with .go file
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !HasGoFiles(dir) {
		t.Error("expected true for dir with .go file")
	}

	// Single .go file
	goFile := filepath.Join(dir, "main.go")
	if !HasGoFiles(goFile) {
		t.Error("expected true for .go file")
	}

	// Non-go file
	pyFile := filepath.Join(dir, "script.py")
	os.WriteFile(pyFile, []byte("print('hello')"), 0644)
	if HasGoFiles(pyFile) {
		t.Error("expected false for .py file")
	}
}

func TestExprString(t *testing.T) {
	// Test with a function that has various param types
	src := `package test
func Complex(m map[string][]int, ch chan error, fn func(int) bool, args ...string) ([]byte, error) {
	return nil, nil
}
`
	path := writeTempGoFile(t, src)
	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}

	expected := "func Complex(m map[string][]int, ch chan error, fn func(int) bool, args ...string) ([]byte, error)"
	if symbols[0].Signature != expected {
		t.Errorf("unexpected signature:\ngot:  %s\nwant: %s", symbols[0].Signature, expected)
	}
}

func TestParseFile_Generics(t *testing.T) {
	src := `package test
type Set[T comparable] struct {
	items map[T]struct{}
}
func Map[T, U any](s []T, f func(T) U) []U {
	return nil
}
`
	path := writeTempGoFile(t, src)
	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}

	// Struct with type params should parse without error
	if symbols[0].Kind != "struct" || symbols[0].Name != "Set" {
		t.Errorf("expected struct Set, got %s %s", symbols[0].Kind, symbols[0].Name)
	}
}
