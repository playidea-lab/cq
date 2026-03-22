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

// summarizeDiff parses unified diff output and returns a human-readable,
// code-free summary of what changed in each file.
func summarizeDiff(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}

	type fileStat struct {
		added   int
		removed int
		// identifiers seen in +/- lines (func/type/const/var names)
		addedIDs   []string
		removedIDs []string
	}

	files := map[string]*fileStat{}
	var order []string // preserve insertion order for deterministic output

	var currentFile string

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

		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			// header lines — skip

		case strings.HasPrefix(line, "+") && currentFile != "":
			st := files[currentFile]
			st.added++
			if id := extractIdentifier(line[1:]); id != "" {
				st.addedIDs = append(st.addedIDs, id)
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
		parts := buildFileSummary(file, st.added, st.removed, st.addedIDs, st.removedIDs)
		fmt.Fprintf(&sb, "%s: %s\n", file, strings.Join(parts, ", "))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// buildFileSummary returns the ordered list of change descriptors for one file.
func buildFileSummary(file string, added, removed int, addedIDs, removedIDs []string) []string {
	var parts []string

	// Net line-count change
	net := added - removed
	switch {
	case net > 0:
		parts = append(parts, fmt.Sprintf("+%d줄", net))
	case net < 0:
		parts = append(parts, fmt.Sprintf("%d줄", net))
	default:
		// equal add/remove → refactor hint
		if added > 0 {
			parts = append(parts, fmt.Sprintf("리팩토링(%d줄 교체)", added))
		}
	}

	// Newly added identifiers
	unique := dedup(addedIDs)
	if len(unique) > 0 {
		parts = append(parts, fmt.Sprintf("%s 추가", strings.Join(unique, "/")))
	}

	// Removed identifiers
	uniqueRm := dedup(removedIDs)
	if len(uniqueRm) > 0 {
		parts = append(parts, fmt.Sprintf("%s 제거", strings.Join(uniqueRm, "/")))
	}

	// Infer structural pattern from file extension
	if pat := inferPattern(file, added, removed); pat != "" {
		parts = append(parts, pat)
	}

	return parts
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
