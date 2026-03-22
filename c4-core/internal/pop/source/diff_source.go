// Package source provides MessageSource implementations for the POP engine.
package source

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/pop"
)

// DiffSource converts raw git diff output into privacy-safe action summaries.
// Code content is stripped; only filename, change type, and structural patterns
// (line count deltas, added/removed identifiers) are retained.
// This makes the output safe to send to an LLM without leaking source code.
type DiffSource struct {
	// diff is the raw output of "git diff" or equivalent.
	diff string
	// createdAt is the timestamp attached to the generated message.
	createdAt time.Time
}

// NewDiffSource constructs a DiffSource from raw git diff text.
func NewDiffSource(diff string) *DiffSource {
	return &DiffSource{diff: diff, createdAt: time.Now().UTC()}
}

// RecentMessages implements pop.MessageSource.
// It ignores the after/limit parameters because a git diff is a snapshot;
// callers can pass zero values for both.
func (d *DiffSource) RecentMessages(_ context.Context, _ time.Time, _ int) ([]pop.Message, error) {
	summary := summarizeDiff(d.diff)
	if summary == "" {
		return nil, nil
	}
	return []pop.Message{
		{
			ID:        "diff-summary",
			Content:   summary,
			CreatedAt: d.createdAt,
		},
	}, nil
}

// fileStat accumulates change statistics and coding-pattern flags for one file.
type fileStat struct {
	added   int
	removed int
	// identifiers seen in +/- lines (func/type/const/var names)
	addedIDs   []string
	removedIDs []string
	// coding pattern flags (detected from added lines only)
	hasErrCheck  bool // "if err != nil"
	hasErrWrap   bool // fmt.Errorf with %w
	hasSentinel  bool // errors.New / errors.As / errors.Is
	hasTableTest bool // []struct{ … } table-driven test
	hasSubTest   bool // t.Run(
	hasMock      bool // mock.* usage
	addedImports []string
	// approximate line count of added func bodies
	funcAddedLines int
	inAddedFunc    bool
}

// summarizeDiff parses unified diff output and returns a human-readable,
// code-free summary of what changed in each file.
func summarizeDiff(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}

	files := map[string]*fileStat{}
	var order []string // preserve insertion order for deterministic output

	var currentFile string
	inImportBlock := false

	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "diff --git "):
			// "diff --git a/foo.go b/foo.go"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// b/foo.go → foo.go
				currentFile = strings.TrimPrefix(parts[3], "b/")
			}
			if _, exists := files[currentFile]; !exists {
				files[currentFile] = &fileStat{}
				order = append(order, currentFile)
			}
			inImportBlock = false

		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			// header lines — skip

		case strings.HasPrefix(line, "+") && currentFile != "":
			st := files[currentFile]
			st.added++
			body := line[1:]
			trimmed := strings.TrimSpace(body)

			if id := extractIdentifier(body); id != "" {
				st.addedIDs = append(st.addedIDs, id)
			}

			// Import block tracking
			if trimmed == "import (" {
				inImportBlock = true
			} else if inImportBlock && trimmed == ")" {
				inImportBlock = false
			} else if inImportBlock {
				if pkg := extractImportPkg(trimmed); pkg != "" {
					st.addedImports = append(st.addedImports, pkg)
				}
			} else if strings.HasPrefix(trimmed, "import ") && !strings.Contains(trimmed, "(") {
				// single-line: import "fmt"
				if pkg := extractImportPkg(strings.TrimPrefix(trimmed, "import ")); pkg != "" {
					st.addedImports = append(st.addedImports, pkg)
				}
			}

			// Error-handling patterns
			if strings.Contains(trimmed, "err != nil") {
				st.hasErrCheck = true
			}
			if strings.Contains(trimmed, "fmt.Errorf") && strings.Contains(trimmed, "%w") {
				st.hasErrWrap = true
			}
			if strings.Contains(trimmed, "errors.New(") ||
				strings.Contains(trimmed, "errors.As(") ||
				strings.Contains(trimmed, "errors.Is(") {
				st.hasSentinel = true
			}

			// Test patterns
			if strings.Contains(trimmed, "[]struct{") || strings.Contains(trimmed, "[]struct {") {
				st.hasTableTest = true
			}
			if strings.Contains(trimmed, "t.Run(") {
				st.hasSubTest = true
			}
			if strings.Contains(trimmed, "mock.") || strings.Contains(trimmed, "Mock(") ||
				strings.Contains(trimmed, "MockCtrl") {
				st.hasMock = true
			}

			// Function-size tracking: count added lines that belong to a func body
			if strings.HasPrefix(trimmed, "func ") {
				st.inAddedFunc = true
				st.funcAddedLines++ // declaration line counts
			} else if st.inAddedFunc {
				st.funcAddedLines++
			}

		case strings.HasPrefix(line, "-") && currentFile != "":
			st := files[currentFile]
			st.removed++
			if id := extractIdentifier(line[1:]); id != "" {
				st.removedIDs = append(st.removedIDs, id)
			}
		}
	}

	var sb strings.Builder
	for _, file := range order {
		st := files[file]
		if st.added == 0 && st.removed == 0 {
			continue
		}
		parts := buildFileSummary(file, st)
		fmt.Fprintf(&sb, "%s: %s\n", file, strings.Join(parts, ", "))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// buildFileSummary returns the ordered list of change descriptors for one file.
func buildFileSummary(file string, st *fileStat) []string {
	var parts []string

	// Net line-count change
	net := st.added - st.removed
	switch {
	case net > 0:
		parts = append(parts, fmt.Sprintf("+%d줄", net))
	case net < 0:
		parts = append(parts, fmt.Sprintf("%d줄", net))
	default:
		// equal add/remove → refactor hint
		if st.added > 0 {
			parts = append(parts, fmt.Sprintf("리팩토링(%d줄 교체)", st.added))
		}
	}

	// Newly added identifiers
	unique := dedup(st.addedIDs)
	if len(unique) > 0 {
		parts = append(parts, fmt.Sprintf("%s 추가", strings.Join(unique, "/")))
	}

	// Removed identifiers
	uniqueRm := dedup(st.removedIDs)
	if len(uniqueRm) > 0 {
		parts = append(parts, fmt.Sprintf("%s 제거", strings.Join(uniqueRm, "/")))
	}

	// Infer structural pattern from change size
	if pat := inferPattern(file, st.added, st.removed); pat != "" {
		parts = append(parts, pat)
	}

	// Approximate func size for added funcs
	if st.funcAddedLines > 0 {
		parts = append(parts, fmt.Sprintf("func~%d줄", st.funcAddedLines))
	}

	// Error-handling patterns
	if st.hasErrWrap {
		parts = append(parts, "error-wrapping(fmt.Errorf %w)")
	} else if st.hasErrCheck {
		parts = append(parts, "error-check(if err!=nil)")
	}
	if st.hasSentinel {
		parts = append(parts, "sentinel-error")
	}

	// Test patterns
	if st.hasTableTest {
		parts = append(parts, "table-driven-test")
	}
	if st.hasSubTest {
		parts = append(parts, "sub-test(t.Run)")
	}
	if st.hasMock {
		parts = append(parts, "mock-usage")
	}

	// New imports
	uniqueImports := dedup(st.addedImports)
	if len(uniqueImports) > 0 {
		parts = append(parts, fmt.Sprintf("import+(%s)", strings.Join(uniqueImports, ",")))
	}

	return parts
}

// extractImportPkg extracts the package name from an import line token.
// Handles: `"fmt"`, `alias "pkg/path"`, `. "pkg"`, `_ "pkg"`.
func extractImportPkg(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	// Find the quoted import path (first opening quote to last closing quote)
	open := strings.Index(token, `"`)
	close := strings.LastIndex(token, `"`)
	if open < 0 || close <= open {
		return ""
	}
	path := token[open+1 : close]
	if path == "" {
		return ""
	}
	// Package name is the last path element
	parts := strings.Split(path, "/")
	pkg := parts[len(parts)-1]
	if pkg == "" || pkg == "_" || pkg == "." {
		return ""
	}
	return pkg
}

// extractIdentifier returns the top-level identifier name if the line declares
// a Go func, type, const, or var. Returns "" otherwise.
func extractIdentifier(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, kw := range []string{"func ", "type ", "const ", "var "} {
		if strings.HasPrefix(trimmed, kw) {
			rest := trimmed[len(kw):]
			// Take the first token (identifier name), strip receiver if any.
			if kw == "func " && strings.HasPrefix(rest, "(") {
				// receiver method: func (r Recv) Name(
				idx := strings.Index(rest, ")")
				if idx >= 0 && idx+2 < len(rest) {
					rest = strings.TrimSpace(rest[idx+1:])
				}
			}
			name := strings.FieldsFunc(rest, func(r rune) bool {
				return r == '(' || r == ' ' || r == '\t' || r == '{'
			})
			if len(name) > 0 {
				return name[0]
			}
		}
	}
	return ""
}

// inferPattern returns a short structural label based on the change size and
// file type, without any code content.
func inferPattern(file string, added, removed int) string {
	switch {
	case added > 0 && removed == 0:
		return "신규 추가"
	case added == 0 && removed > 0:
		return "삭제"
	case added > removed*2:
		return "대폭 확장"
	case removed > added*2:
		return "대폭 축소"
	default:
		return ""
	}
}

// dedup removes duplicate strings while preserving first-occurrence order,
// and caps the result at 3 entries to keep summaries concise.
func dedup(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) == 3 {
			break
		}
	}
	return out
}
