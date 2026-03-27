package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
)

// Importer orchestrates importing conversation sessions into the knowledge store.
type Importer struct {
	Store         *knowledge.Store
	Summarizer    *Summarizer
	MaxConcurrent int // default 2
}

// ImportSessions processes sessions: dedup check -> summarize -> store.
// Individual session errors are collected in the result rather than aborting.
func (imp *Importer) ImportSessions(ctx context.Context, sessions []Session) (*ImportResult, error) {
	if imp.Store == nil {
		return nil, fmt.Errorf("knowledge store is required")
	}

	maxConc := imp.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 2
	}

	result := &ImportResult{Total: len(sessions)}

	// Bounded concurrency via semaphore channel.
	sem := make(chan struct{}, maxConc)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, sess := range sessions {
		// Check context before starting each session.
		if ctx.Err() != nil {
			break
		}

		// Progress output.
		fmt.Printf("Processing %d/%d...\n", i+1, len(sessions))

		// Dedup: check if already imported.
		if imp.isDuplicate(sess) {
			mu.Lock()
			result.Skipped++
			mu.Unlock()
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(sess Session) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := imp.importOne(ctx, sess); err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, ImportError{
					SessionID: sess.ID,
					Err:       err,
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			result.Imported++
			mu.Unlock()
		}(sess)
	}

	wg.Wait()
	return result, nil
}

// isDuplicate checks if a session was already imported by searching for its session_id in the FTS index.
// The session ID is stored in tags_text (as "sid:<id>") and indexed by FTS5.
func (imp *Importer) isDuplicate(sess Session) bool {
	// Search by the unique session tag in FTS (indexed in tags_text field).
	tag := "sid:" + sess.ID
	results, err := imp.Store.SearchFTS(tag, 5)
	if err != nil || len(results) == 0 {
		// FTS might fail on special characters; fall back to List scan.
		return imp.isDuplicateByList(sess)
	}

	// FTS returned results; verify by title match to avoid false positives.
	for _, r := range results {
		title, _ := r["title"].(string)
		if strings.Contains(title, sess.ID) {
			return true
		}
	}
	return false
}

// isDuplicateByList is the fallback dedup check via listing recent insight docs.
func (imp *Importer) isDuplicateByList(sess Session) bool {
	docs, err := imp.Store.List("insight", "session", 200)
	if err != nil {
		return false
	}
	for _, d := range docs {
		title, _ := d["title"].(string)
		if strings.Contains(title, sess.ID) {
			return true
		}
	}
	return false
}

// importOne summarizes and stores a single session.
func (imp *Importer) importOne(ctx context.Context, sess Session) error {
	var body string
	var err error

	if imp.Summarizer != nil {
		body, err = imp.Summarizer.Summarize(ctx, sess)
		if err != nil {
			// Fallback: store raw truncated conversation text.
			body = fallbackBody(sess)
		}
	} else {
		body = fallbackBody(sess)
	}

	project := sess.Project
	if project == "" {
		project = "unknown"
	}
	date := sess.StartedAt.Format("2006-01-02")
	title := fmt.Sprintf("세션 임포트: %s [%s] (%s, %s)", project, sess.ID, sess.Source, date)

	meta := map[string]any{
		"title":  title,
		"domain": "session",
		"tags":   []string{sess.Source, "imported", "session-summary", "sid:" + sess.ID},
	}

	_, err = imp.Store.Create(knowledge.TypeInsight, meta, body)
	if err != nil {
		return fmt.Errorf("knowledge store create: %w", err)
	}
	return nil
}

// fallbackBody produces a summary body without LLM when summarization is unavailable.
func fallbackBody(sess Session) string {
	conv := formatConversation(sess)
	conv = truncateToTokens(conv, maxInputTokens)

	project := sess.Project
	if project == "" {
		project = "unknown"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("## 세션: %s (%s)\n\nImported at %s\n\n%s", project, sess.Source, now, conv)
}
