// Package dartast provides Go-native Dart symbol parsing using regex-based analysis.
// This enables c4_find_symbol and c4_get_symbols_overview for Dart/Flutter files
// without requiring external tools like dart analyzer.
package dartast

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Symbol represents a parsed Dart symbol.
type Symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // class, mixin, enum, extension, typedef, function, method, constructor, getter, setter
	Line       int    `json:"line"`
	EndLine    int    `json:"end_line"`
	Column     int    `json:"column"`
	Signature  string `json:"signature"`
	Docstring  string `json:"docstring,omitempty"`
	ParentName string `json:"parent_name,omitempty"`
	ParentType string `json:"parent_type,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
}

var (
	reClass     = regexp.MustCompile(`^(?:(?:abstract|sealed|base|final|interface)\s+)*(?:class|mixin\s+class)\s+(\w+)`)
	reMixin     = regexp.MustCompile(`^mixin\s+(\w+)`)
	reEnum      = regexp.MustCompile(`^enum\s+(\w+)`)
	reExtType   = regexp.MustCompile(`^extension\s+type\s+(\w+)`)
	reExtension = regexp.MustCompile(`^extension\s+(\w+)\s+on\s+`)
	reTypedef   = regexp.MustCompile(`^typedef\s+(\w+)`)
	reGetter    = regexp.MustCompile(`^(?:static\s+)?(?:\w[\w<>,?\s]*?\s+)?get\s+(\w+)`)
	reSetter    = regexp.MustCompile(`^(?:static\s+)?set\s+(\w+)\s*\(`)
	reOperator  = regexp.MustCompile(`operator\s+(\S+)\s*\(`)
	reSkipLine  = regexp.MustCompile(`^(?:import|export|part|library)\s+`)

	controlKW = map[string]bool{
		"if": true, "else": true, "for": true, "while": true, "do": true,
		"switch": true, "case": true, "catch": true, "try": true, "finally": true,
		"return": true, "throw": true, "yield": true, "assert": true, "await": true,
		"super": true, "this": true, "new": true, "import": true, "export": true,
		"part": true, "library": true, "show": true, "hide": true,
	}

	skipDirs = map[string]bool{
		".dart_tool": true, "build": true, ".git": true,
		".idea": true, "node_modules": true,
	}
)

// ParseFile parses a single Dart file and returns its symbols.
func ParseFile(filePath string) ([]Symbol, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return parseContent(string(data), filePath)
}

// ParseDir parses all Dart files in a directory recursively.
func ParseDir(dirPath string, maxFiles int) ([]Symbol, error) {
	if maxFiles <= 0 {
		maxFiles = 200
	}
	var all []Symbol
	count := 0
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".dart") {
			return nil
		}
		if count >= maxFiles {
			return filepath.SkipAll
		}
		count++
		syms, parseErr := ParseFile(path)
		if parseErr != nil {
			return nil
		}
		all = append(all, syms...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}

// SymbolsOverview returns symbols grouped by kind for display.
func SymbolsOverview(path string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
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

// FindSymbolByName finds symbols matching the given name.
// Supports "Parent/method" and "Parent.method" notation.
func FindSymbolByName(name, path string) ([]Symbol, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
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

	parent, member := "", name
	if idx := strings.IndexAny(name, "/."); idx >= 0 {
		parent = name[:idx]
		member = name[idx+1:]
	}

	var matches []Symbol
	for _, sym := range symbols {
		if parent != "" {
			if sym.Name == member && sym.ParentName == parent {
				matches = append(matches, sym)
			}
		} else {
			if sym.Name == name {
				matches = append(matches, sym)
			}
		}
	}
	return matches, nil
}

// HasDartFiles checks if the given path is or contains Dart files.
func HasDartFiles(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return strings.HasSuffix(path, ".dart")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".dart") {
			return true
		}
	}
	// Check one level of subdirectories
	for _, e := range entries {
		if e.IsDir() && !skipDirs[e.Name()] {
			sub, _ := os.ReadDir(filepath.Join(path, e.Name()))
			for _, se := range sub {
				if !se.IsDir() && strings.HasSuffix(se.Name(), ".dart") {
					return true
				}
			}
		}
	}
	return false
}

// --- Internal scanner ---

func parseContent(content, filePath string) ([]Symbol, error) {
	lines := strings.Split(content, "\n")
	sc := &scan{lines: lines, filePath: filePath}
	return sc.run(), nil
}

type ctr struct {
	name  string
	kind  string
	depth int
}

type scan struct {
	lines    []string
	filePath string
	symbols  []Symbol
	depth    int
	blockCmt bool
	docBuf   []string
	stack    []ctr
}

func (s *scan) cur() *ctr {
	if len(s.stack) == 0 {
		return nil
	}
	return &s.stack[len(s.stack)-1]
}

func (s *scan) run() []Symbol {
	for i, raw := range s.lines {
		ln := i + 1
		t := strings.TrimSpace(raw)

		// Block comment tracking
		if s.blockCmt {
			if idx := strings.Index(t, "*/"); idx >= 0 {
				s.blockCmt = false
				t = strings.TrimSpace(t[idx+2:])
				if t == "" {
					continue
				}
			} else {
				continue
			}
		}
		if strings.HasPrefix(t, "/*") {
			if !strings.Contains(t[2:], "*/") {
				s.blockCmt = true
			}
			continue
		}

		// Doc comments
		if strings.HasPrefix(t, "///") {
			s.docBuf = append(s.docBuf, strings.TrimSpace(t[3:]))
			continue
		}
		if strings.HasPrefix(t, "//") {
			continue
		}
		// Annotations — preserve doc buffer
		if strings.HasPrefix(t, "@") {
			continue
		}
		if t == "" {
			s.docBuf = nil
			continue
		}
		if reSkipLine.MatchString(t) {
			s.docBuf = nil
			s.depth += netBraces(t)
			continue
		}

		sym := s.matchDecl(t, ln, raw)
		if sym != nil {
			sym.Docstring = strings.Join(s.docBuf, "\n")
			sym.FilePath = s.filePath
			sym.EndLine = s.endLine(i)
			s.symbols = append(s.symbols, *sym)
		}
		s.docBuf = nil

		prev := s.depth
		s.depth += netBraces(t)

		// Push container if depth increased on a container declaration
		if sym != nil && isCtr(sym.Kind) && s.depth > prev {
			s.stack = append(s.stack, ctr{name: sym.Name, kind: sym.Kind, depth: prev})
		}
		// Pop containers when depth returns
		for len(s.stack) > 0 && s.depth <= s.stack[len(s.stack)-1].depth {
			s.stack = s.stack[:len(s.stack)-1]
		}
	}
	return s.symbols
}

func (s *scan) matchDecl(t string, ln int, raw string) *Symbol {
	col := len(raw) - len(strings.TrimLeft(raw, " \t"))
	c := s.cur()

	// --- Container types ---
	if m := reClass.FindStringSubmatch(t); m != nil {
		return &Symbol{Name: m[1], Kind: "class", Line: ln, Column: col, Signature: cleanSig(t)}
	}
	if !strings.Contains(t, "mixin class") {
		if m := reMixin.FindStringSubmatch(t); m != nil {
			return &Symbol{Name: m[1], Kind: "mixin", Line: ln, Column: col, Signature: cleanSig(t)}
		}
	}
	if m := reEnum.FindStringSubmatch(t); m != nil {
		return &Symbol{Name: m[1], Kind: "enum", Line: ln, Column: col, Signature: cleanSig(t)}
	}
	if m := reExtType.FindStringSubmatch(t); m != nil {
		return &Symbol{Name: m[1], Kind: "extension", Line: ln, Column: col, Signature: cleanSig(t)}
	}
	if m := reExtension.FindStringSubmatch(t); m != nil {
		return &Symbol{Name: m[1], Kind: "extension", Line: ln, Column: col, Signature: cleanSig(t)}
	}
	if m := reTypedef.FindStringSubmatch(t); m != nil {
		return &Symbol{Name: m[1], Kind: "typedef", Line: ln, Column: col, Signature: cleanSig(t)}
	}

	// --- Getter / Setter / Operator ---
	if m := reGetter.FindStringSubmatch(t); m != nil {
		sy := &Symbol{Name: m[1], Kind: "getter", Line: ln, Column: col, Signature: cleanSig(t)}
		if c != nil {
			sy.ParentName = c.name
			sy.ParentType = c.kind
		}
		return sy
	}
	if m := reSetter.FindStringSubmatch(t); m != nil {
		sy := &Symbol{Name: m[1], Kind: "setter", Line: ln, Column: col, Signature: cleanSig(t)}
		if c != nil {
			sy.ParentName = c.name
			sy.ParentType = c.kind
		}
		return sy
	}
	if m := reOperator.FindStringSubmatch(t); m != nil && c != nil {
		return &Symbol{
			Name: "operator " + m[1], Kind: "method", Line: ln, Column: col,
			Signature: cleanSig(t), ParentName: c.name, ParentType: c.kind,
		}
	}

	// Skip enum values: lines ending with , or ; that look like enum value declarations
	// Enum values: "red," "red('Red')," "blue('Blue');" (last value before methods)
	if c != nil && c.kind == "enum" {
		if strings.HasSuffix(t, ",") {
			return nil
		}
		// Last enum value ends with ; (e.g., "blue('Blue');")
		// Distinguish from methods: methods have return types or known patterns
		// Simple heuristic: if line is just "identifier;" or "identifier(args);"
		// without a return type, it's an enum value
		stripped := strings.TrimSuffix(t, ";")
		if pi := strings.Index(stripped, "("); pi > 0 {
			before := strings.TrimSpace(stripped[:pi])
			// Enum value: no spaces (just the name), e.g. "blue('Blue')"
			if !strings.Contains(before, " ") {
				return nil
			}
		} else if !strings.Contains(stripped, " ") && stripped != "" {
			// Just "blue;" with no spaces — enum value
			return nil
		}
	}

	// --- Function / Method / Constructor ---
	if name, signature, ok := funcLike(t); ok {
		sy := &Symbol{Name: name, Line: ln, Column: col, Signature: signature}
		if c != nil {
			sy.ParentName = c.name
			sy.ParentType = c.kind
			if name == c.name || strings.HasPrefix(name, c.name+".") {
				sy.Kind = "constructor"
			} else {
				sy.Kind = "method"
			}
		} else {
			sy.Kind = "function"
		}
		return sy
	}

	return nil
}

func funcLike(t string) (string, string, bool) {
	clean := t
	for _, p := range []string{"factory ", "static ", "external "} {
		clean = strings.TrimPrefix(clean, p)
	}

	pi := strings.Index(clean, "(")
	if pi <= 0 {
		return "", "", false
	}

	before := clean[:pi]
	before = strings.TrimSpace(before)
	if before == "" {
		return "", "", false
	}

	// Strip only trailing generics on the function name: name<T>(
	// NOT return type generics like Future<void> init(
	// Check if before ends with >: "Future<void> init" does NOT end with >
	// but "init<T>" DOES end with >
	if strings.HasSuffix(before, ">") {
		if lt := strings.LastIndex(before, "<"); lt >= 0 {
			before = strings.TrimSpace(before[:lt])
		}
	}

	words := strings.Fields(before)
	if len(words) == 0 {
		return "", "", false
	}
	name := words[len(words)-1]

	if controlKW[name] || len(name) == 0 {
		return "", "", false
	}
	for _, ch := range name {
		if !isIdent(ch) && ch != '.' {
			return "", "", false
		}
	}

	return name, cleanSig(t), true
}

func isIdent(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// cleanSig strips body/arrow/initializer from a declaration line to produce a clean signature.
func cleanSig(line string) string {
	s := line
	for _, m := range []string{" async {", " async* {", " sync* {", " {"} {
		if idx := strings.Index(s, m); idx > 0 {
			s = s[:idx]
			break
		}
	}
	if strings.HasSuffix(s, "{") {
		s = strings.TrimSpace(s[:len(s)-1])
	}
	if idx := strings.Index(s, " => "); idx > 0 {
		s = s[:idx]
	}
	// Strip initializer list: Counter() : _count = 0
	if idx := strings.Index(s, ") :"); idx > 0 {
		s = s[:idx+1]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), ";")
	return s
}

func (s *scan) endLine(startIdx int) int {
	depth := 0
	opened := false
	for i := startIdx; i < len(s.lines); i++ {
		stripped := stripForBraces(s.lines[i])
		for _, ch := range stripped {
			if ch == '{' {
				depth++
				opened = true
			} else if ch == '}' {
				depth--
				if opened && depth == 0 {
					return i + 1
				}
			}
		}
		if !opened && i == startIdx {
			trimmed := strings.TrimSpace(s.lines[i])
			if strings.HasSuffix(trimmed, ";") {
				return i + 1
			}
		}
	}
	return startIdx + 1
}

// stripForBraces removes string literals and line comments for brace counting.
func stripForBraces(line string) string {
	var buf strings.Builder
	inSingle, inDouble, escaped := false, false, false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}
		if !inSingle && !inDouble && ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}
		if !inDouble && ch == '\'' {
			inSingle = !inSingle
			continue
		}
		if !inSingle && ch == '"' {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble {
			buf.WriteByte(ch)
		}
	}
	return buf.String()
}

func netBraces(line string) int {
	n := 0
	for _, ch := range stripForBraces(line) {
		if ch == '{' {
			n++
		} else if ch == '}' {
			n--
		}
	}
	return n
}

func isCtr(kind string) bool {
	return kind == "class" || kind == "mixin" || kind == "enum" || kind == "extension"
}

func groupByKind(symbols []Symbol, path string) map[string]any {
	groups := make(map[string][]map[string]any)
	for _, sym := range symbols {
		g := kindGroup(sym.Kind)
		entry := map[string]any{
			"name": sym.Name, "line": sym.Line, "end_line": sym.EndLine, "signature": sym.Signature,
		}
		if sym.Docstring != "" {
			entry["docstring"] = sym.Docstring
		}
		if sym.ParentName != "" {
			entry["description"] = fmt.Sprintf("%s (%s.%s)", sym.Signature, sym.ParentName, sym.Name)
		} else {
			entry["description"] = sym.Signature
		}
		groups[g] = append(groups[g], entry)
	}
	result := map[string]any{"file": path, "success": true}
	for k, v := range groups {
		result[k] = v
	}
	return result
}

func kindGroup(kind string) string {
	switch kind {
	case "class", "mixin", "extension":
		return "classes"
	case "enum":
		return "enums"
	case "typedef":
		return "typedefs"
	default:
		return "functions"
	}
}
