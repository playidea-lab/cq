// Package goast provides Go source code symbol parsing using go/ast.
// This enables c4_find_symbol and c4_get_symbols_overview for Go files,
// complementing the Python-based LSP tools (Jedi/multilspy) for Python/JS/TS.
package goast

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Symbol represents a parsed Go symbol.
type Symbol struct {
	Name        string `json:"name"`
	Kind        string `json:"type"`
	Line        int    `json:"line"`
	EndLine     int    `json:"end_line"`
	Column      int    `json:"column"`
	FilePath    string `json:"module_path"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	ParentType  string `json:"parent_type"`
	ParentName  string `json:"parent_name"`
	Receiver    string `json:"receiver,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Doc         string `json:"docstring,omitempty"`
}

// ParseFile parses a single Go file and returns its symbols.
func ParseFile(filePath string) ([]Symbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}

	pkgName := f.Name.Name
	var symbols []Symbol

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name:       d.Name.Name,
				Line:       fset.Position(d.Pos()).Line,
				EndLine:    fset.Position(d.End()).Line,
				Column:     fset.Position(d.Pos()).Column - 1,
				FilePath:   filePath,
				ParentType: "package",
				ParentName: pkgName,
			}

			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = "method"
				sym.Receiver = exprString(d.Recv.List[0].Type)
				sym.FullName = fmt.Sprintf("%s.%s.%s", pkgName, sym.Receiver, d.Name.Name)
				sym.Description = fmt.Sprintf("func (%s) %s%s", sym.Receiver, d.Name.Name, funcParams(d.Type))
				sym.Signature = sym.Description
				sym.ParentName = strings.TrimPrefix(sym.Receiver, "*")
				sym.ParentType = "type"
			} else {
				sym.Kind = "function"
				sym.FullName = fmt.Sprintf("%s.%s", pkgName, d.Name.Name)
				sym.Description = fmt.Sprintf("func %s%s", d.Name.Name, funcParams(d.Type))
				sym.Signature = sym.Description
			}

			if d.Doc != nil {
				sym.Doc = strings.TrimSpace(d.Doc.Text())
				if len(sym.Doc) > 500 {
					sym.Doc = sym.Doc[:500] + "..."
				}
			}

			symbols = append(symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := Symbol{
						Name:       s.Name.Name,
						Line:       fset.Position(s.Pos()).Line,
						EndLine:    fset.Position(s.End()).Line,
						Column:     fset.Position(s.Pos()).Column - 1,
						FilePath:   filePath,
						FullName:   fmt.Sprintf("%s.%s", pkgName, s.Name.Name),
						ParentType: "package",
						ParentName: pkgName,
					}

					switch t := s.Type.(type) {
					case *ast.StructType:
						sym.Kind = "struct"
						n := 0
						if t.Fields != nil {
							n = len(t.Fields.List)
						}
						sym.Description = fmt.Sprintf("type %s struct (%d fields)", s.Name.Name, n)
					case *ast.InterfaceType:
						sym.Kind = "interface"
						n := 0
						if t.Methods != nil {
							n = len(t.Methods.List)
						}
						sym.Description = fmt.Sprintf("type %s interface (%d methods)", s.Name.Name, n)
					default:
						sym.Kind = "type"
						sym.Description = fmt.Sprintf("type %s %s", s.Name.Name, exprString(s.Type))
					}

					if s.Doc != nil {
						sym.Doc = strings.TrimSpace(s.Doc.Text())
					} else if d.Doc != nil {
						sym.Doc = strings.TrimSpace(d.Doc.Text())
					}
					if len(sym.Doc) > 500 {
						sym.Doc = sym.Doc[:500] + "..."
					}

					symbols = append(symbols, sym)

				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.Name == "_" {
							continue
						}
						sym := Symbol{
							Name:       name.Name,
							Kind:       kind,
							Line:       fset.Position(name.Pos()).Line,
							EndLine:    fset.Position(s.End()).Line,
							Column:     fset.Position(name.Pos()).Column - 1,
							FilePath:   filePath,
							FullName:   fmt.Sprintf("%s.%s", pkgName, name.Name),
							ParentType: "package",
							ParentName: pkgName,
						}
						if s.Type != nil {
							sym.Description = fmt.Sprintf("%s %s %s", kind, name.Name, exprString(s.Type))
						} else {
							sym.Description = fmt.Sprintf("%s %s", kind, name.Name)
						}
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}

	return symbols, nil
}

// ParseDir parses all Go files in a directory (recursively).
func ParseDir(dirPath string, maxFiles int) ([]Symbol, error) {
	if maxFiles <= 0 {
		maxFiles = 200
	}

	var allSymbols []Symbol
	count := 0

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "testdata" || base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if count >= maxFiles {
			return filepath.SkipAll
		}
		count++

		symbols, parseErr := ParseFile(path)
		if parseErr != nil {
			return nil // skip unparseable files
		}
		allSymbols = append(allSymbols, symbols...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return allSymbols, nil
}

// SymbolsOverview returns symbols grouped by kind for a Go file or directory.
func SymbolsOverview(path string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	var symbols []Symbol
	if info.IsDir() {
		symbols, err = ParseDir(path, 200)
	} else {
		symbols, err = ParseFile(path)
	}
	if err != nil {
		return nil, err
	}

	return groupByKind(symbols, path), nil
}

// FindSymbolByName finds symbols matching the given name in the path.
// Name supports exact match and "Parent/Method" or "Parent.Method" patterns.
func FindSymbolByName(name, path string) ([]Symbol, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	var symbols []Symbol
	if info.IsDir() {
		symbols, err = ParseDir(path, 200)
	} else {
		symbols, err = ParseFile(path)
	}
	if err != nil {
		return nil, err
	}

	// Parse name pattern: "Name", "Parent/Name", "Parent.Name"
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		parts = strings.Split(name, ".")
	}
	targetName := parts[len(parts)-1]
	parentName := ""
	if len(parts) > 1 {
		parentName = parts[len(parts)-2]
	}

	var results []Symbol
	for _, s := range symbols {
		if s.Name != targetName {
			continue
		}
		if parentName != "" {
			recv := strings.TrimPrefix(s.Receiver, "*")
			if recv != parentName && s.ParentName != parentName {
				continue
			}
		}
		results = append(results, s)
	}

	return results, nil
}

// HasGoFiles checks if the given path is or contains Go files.
func HasGoFiles(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return strings.HasSuffix(path, ".go")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	// Check one level of subdirectories
	for _, e := range entries {
		if e.IsDir() && e.Name() != ".git" && e.Name() != "vendor" {
			subEntries, err := os.ReadDir(filepath.Join(path, e.Name()))
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if !se.IsDir() && strings.HasSuffix(se.Name(), ".go") {
					return true
				}
			}
		}
	}
	return false
}

// --- helpers ---

func groupByKind(symbols []Symbol, path string) map[string]any {
	groups := map[string][]map[string]any{}

	for _, s := range symbols {
		item := map[string]any{
			"name":        s.Name,
			"line":        s.Line,
			"end_line":    s.EndLine,
			"description": s.Description,
		}
		if s.Signature != "" {
			item["signature"] = s.Signature
		}
		if s.Receiver != "" {
			item["receiver"] = s.Receiver
		}
		if s.Doc != "" {
			item["docstring"] = s.Doc
		}

		key := kindToGroup(s.Kind)
		groups[key] = append(groups[key], item)
	}

	result := map[string]any{
		"file": path,
	}
	for k, v := range groups {
		result[k] = v
	}
	return result
}

func kindToGroup(kind string) string {
	switch kind {
	case "function":
		return "functions"
	case "method":
		return "methods"
	case "struct":
		return "structs"
	case "interface":
		return "interfaces"
	case "type":
		return "types"
	case "const":
		return "constants"
	case "var":
		return "variables"
	default:
		return "other"
	}
}

// exprString converts an ast.Expr to a simple string representation.
func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprString(e.X)
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprString(e.Elt)
		}
		return "[...]" + exprString(e.Elt)
	case *ast.MapType:
		return "map[" + exprString(e.Key) + "]" + exprString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func" + funcParams(e)
	case *ast.ChanType:
		switch e.Dir {
		case ast.SEND:
			return "chan<- " + exprString(e.Value)
		case ast.RECV:
			return "<-chan " + exprString(e.Value)
		default:
			return "chan " + exprString(e.Value)
		}
	case *ast.Ellipsis:
		return "..." + exprString(e.Elt)
	case *ast.IndexExpr:
		return exprString(e.X) + "[" + exprString(e.Index) + "]"
	case *ast.IndexListExpr:
		parts := make([]string, len(e.Indices))
		for i, idx := range e.Indices {
			parts[i] = exprString(idx)
		}
		return exprString(e.X) + "[" + strings.Join(parts, ", ") + "]"
	case *ast.StructType:
		return "struct{}"
	case nil:
		return ""
	default:
		return "?"
	}
}

// funcParams formats function parameters and return types.
func funcParams(ft *ast.FuncType) string {
	var b strings.Builder
	b.WriteString("(")
	if ft.Params != nil {
		writeFieldList(&b, ft.Params.List)
	}
	b.WriteString(")")

	if ft.Results != nil && len(ft.Results.List) > 0 {
		if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
			b.WriteString(" ")
			b.WriteString(exprString(ft.Results.List[0].Type))
		} else {
			b.WriteString(" (")
			writeFieldList(&b, ft.Results.List)
			b.WriteString(")")
		}
	}

	return b.String()
}

func writeFieldList(b *strings.Builder, fields []*ast.Field) {
	for i, f := range fields {
		if i > 0 {
			b.WriteString(", ")
		}
		if len(f.Names) > 0 {
			for j, n := range f.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(n.Name)
			}
			b.WriteString(" ")
		}
		b.WriteString(exprString(f.Type))
	}
}
