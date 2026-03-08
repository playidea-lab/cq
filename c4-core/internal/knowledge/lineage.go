package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LineageBuilder queries hypothesis lineage from TypeDebate documents.
type LineageBuilder struct {
	store *Store
}

// NewLineageBuilder creates a LineageBuilder backed by store.
func NewLineageBuilder(store *Store) *LineageBuilder {
	return &LineageBuilder{store: store}
}

// BuildContext returns a lineage context string for the given hypothesisID.
// limit caps the number of debate entries returned (0 or negative defaults to 5).
// Entries are sorted by round descending (most recent first).
func (l *LineageBuilder) BuildContext(_ context.Context, hypothesisID string, limit int) (string, error) {
	if limit <= 0 {
		limit = 5
	}

	// List all debate docs; updated_at ordering may be unreliable for custom frontmatter,
	// so we collect all matching entries and sort by round.
	rows, err := l.store.List(string(TypeDebate), "", 1000)
	if err != nil {
		return "", fmt.Errorf("lineage list: %w", err)
	}

	type debateEntry struct {
		round      int
		valLoss    float64
		testMetric float64
		verdict    string
	}

	var all []debateEntry
	var parentHypID string

	docsDir := l.store.DocsDir()
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		// Read raw markdown to access custom frontmatter fields not in Document struct.
		data, err := os.ReadFile(filepath.Join(docsDir, id+".md"))
		if err != nil {
			continue
		}
		fm, _ := parseFrontmatter(string(data))
		hypID, _ := fm["hypothesis_id"].(string)
		if hypID != hypothesisID {
			continue
		}
		if parentHypID == "" {
			parentHypID, _ = fm["parent_hypothesis_id"].(string)
		}
		all = append(all, debateEntry{
			round:      fmInt(fm, "round"),
			valLoss:    fmFloat(fm, "val_loss"),
			testMetric: fmFloat(fm, "test_metric"),
			verdict:    fmString(fm, "verdict"),
		})
	}

	// Sort by round descending (most recent first), then take up to limit.
	sort.Slice(all, func(i, j int) bool { return all[i].round > all[j].round })
	entries := all
	if len(entries) > limit {
		entries = entries[:limit]
	}

	var sb strings.Builder
	sb.WriteString("## Lineage Context\n")
	if parentHypID != "" {
		sb.WriteString(fmt.Sprintf("부모 가설: %s\n", parentHypID))
	}
	if len(entries) > 0 {
		sb.WriteString("최근 실험 히스토리:\n")
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("- round %d: val_loss=%g, test_metric=%g, verdict=%s\n",
				e.round, e.valLoss, e.testMetric, e.verdict))
		}
		// Count consecutive null_result verdicts starting from the most recent (entries[0]).
		nullCount := 0
		for _, e := range entries {
			if e.verdict == "null_result" {
				nullCount++
			} else {
				break
			}
		}
		sb.WriteString(fmt.Sprintf("연속 null_result: %d회\n", nullCount))
	}
	return sb.String(), nil
}
