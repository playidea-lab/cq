package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/cqdata"
	"github.com/changmin/c4-core/internal/ontology"
	"github.com/changmin/c4-core/internal/task"
)

// enrichWithConfig applies config-based branch and worktree assignment.
func (s *SQLiteStore) enrichWithConfig(assignment *TaskAssignment, workerID string) {
	if s.config == nil {
		return
	}

	cfg := s.config.GetConfig()
	if cfg.WorkBranchPrefix != "" {
		assignment.Branch = cfg.WorkBranchPrefix + assignment.TaskID
	}

	if cfg.Worktree.Enabled && s.projectRoot != "" {
		wtPath := filepath.Join(s.projectRoot, ".c4", "worktrees", workerID)
		assignment.WorktreePath = wtPath
		// Auto-create worktree (best-effort, skip if already exists)
		if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
			branch := assignment.Branch
			if branch == "" {
				branch = "c4-" + assignment.TaskID
			}
			if _, wtErr := runGit(s.projectRoot, "worktree", "add", wtPath, "-b", branch); wtErr != nil {
				fmt.Fprintf(os.Stderr, "c4: warning: failed to create worktree %s: %v\n", wtPath, wtErr)
			}
		}
	}
}

// enrichWithSoulContext injects soul context (best-effort).
func (s *SQLiteStore) enrichWithSoulContext(assignment *TaskAssignment) {
	if s.projectRoot != "" {
		s.injectSoulContext(assignment)
	}
}

// enrichWithLighthouse injects lighthouse spec for T-LH- tasks.
func (s *SQLiteStore) enrichWithLighthouse(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "T-LH-") {
		return
	}

	// Extract lighthouse name: T-LH-{name}-{ver}
	parts := strings.TrimPrefix(assignment.TaskID, "T-LH-")
	if idx := strings.LastIndex(parts, "-"); idx > 0 {
		lhName := parts[:idx]
		lh, lhErr := s.getLighthouse(lhName)
		if lhErr == nil {
			assignment.LighthouseSpec = &LighthouseContext{
				Name:        lh.Name,
				Spec:        lh.Spec,
				InputSchema: lh.InputSchema,
				Description: lh.Description,
			}
		} else {
			fmt.Fprintf(os.Stderr, "c4: warning: task %s has T-LH- prefix but lighthouse '%s' not found\n", assignment.TaskID, lhName)
		}
	}
}

// enrichWithReviewContext injects parent T's review context for R- tasks.
func (s *SQLiteStore) enrichWithReviewContext(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "R-") {
		return
	}

	_, baseID, ver, _ := task.ParseTaskID(assignment.TaskID)
	parentID := fmt.Sprintf("T-%s-%d", baseID, ver)
	var commitSHA, filesChangedCol, legacyBranchFiles, handoff string
	if err := s.db.QueryRow("SELECT commit_sha, files_changed, branch, handoff FROM c4_tasks WHERE task_id=?", parentID).Scan(&commitSHA, &filesChangedCol, &legacyBranchFiles, &handoff); err != nil {
		if err != sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "c4: assign-task: review context for %s: %v\n", parentID, err)
		}
	}
	// Prefer the dedicated files_changed column, then handoff JSON, then legacy branch field.
	filesChanged := filesChangedCol
	if filesChanged == "" {
		filesChanged = extractFilesChangedFromHandoff(handoff)
	}
	if filesChanged == "" {
		// Backward compatibility for legacy rows that stored files in branch.
		filesChanged = legacyBranchFiles
	}
	// Extract evidence from parent T handoff
	var evidence []HandoffEvidence
	if handoff != "" {
		if ho := parseHandoff(handoff); len(ho.Evidence) > 0 {
			evidence = ho.Evidence
		}
	}

	if commitSHA != "" || filesChanged != "" || len(evidence) > 0 {
		rc := &ReviewContext{
			ParentTaskID: parentID,
			CommitSHA:    commitSHA,
			FilesChanged: filesChanged,
		}
		if len(evidence) > 0 {
			rc.Evidence = evidence
		}
		assignment.ReviewContext = rc
	}
}

// extractFilesChangedFromHandoff is a helper used by enrichWithReviewContext.
func extractFilesChangedFromHandoff(handoff string) string {
	if strings.TrimSpace(handoff) == "" {
		return ""
	}
	var payload struct {
		Type         string   `json:"type"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(handoff), &payload); err != nil {
		return ""
	}
	if payload.Type != "" && payload.Type != "direct_report" {
		return ""
	}
	if len(payload.FilesChanged) == 0 {
		return ""
	}
	return strings.Join(payload.FilesChanged, ",")
}

// enrichUnified combines knowledge, project ontology, and personal ontology into
// a single coherent context section. Instead of 3 separate blocks, it groups
// information by relevance so workers get a unified view of:
// - Developer coding style (personal ontology HIGH nodes)
// - Project conventions (project ontology nodes)
// - Past solutions (knowledge search results)
func (s *SQLiteStore) enrichUnified(assignment *TaskAssignment) {
	var b strings.Builder
	b.WriteString("## Context (auto-injected)\n")

	// 1. Developer Profile (personal ontology HIGH nodes)
	username := os.Getenv("USER")
	if username != "" {
		if personal, err := ontology.Load(username); err == nil && personal != nil {
			var profileLines []string
			for path, node := range personal.Schema.Nodes {
				if node.NodeConfidence != ontology.ConfidenceHigh {
					continue
				}
				line := fmt.Sprintf("  - %s: %s", path, node.Description)
				profileLines = append(profileLines, line)
			}
			if len(profileLines) > 0 {
				b.WriteString("\n### Developer Style\n")
				for _, l := range profileLines {
					b.WriteString(l)
					b.WriteByte('\n')
				}
			}
		}
	}

	// 2. Project Conventions (project ontology nodes)
	if s.projectRoot != "" {
		if proj, err := ontology.LoadProject(s.projectRoot); err == nil && proj != nil {
			domain := assignment.Domain
			var projLines []string
			for path, node := range proj.Schema.Nodes {
				if node.Scope == "project" || (domain != "" && node.SourceRole == domain) {
					line := fmt.Sprintf("  - %s: %s", path, node.Description)
					projLines = append(projLines, line)
				}
			}
			if len(projLines) > 0 {
				b.WriteString("\n### Project Conventions\n")
				for _, l := range projLines {
					b.WriteString(l)
					b.WriteByte('\n')
				}
			}
		}
	}

	// 3. Past Solutions (knowledge search with ontology-based personalization)
	if s.knowledgeSearch != nil {
		query := assignment.Title
		if assignment.Domain != "" {
			query = assignment.Domain + " " + query
		}
		// Boost query with HIGH-confidence personal ontology tags for personalized ranking.
		// Non-fatal: if ontology load fails, fall back to original query.
		if boostUsername := os.Getenv("USER"); boostUsername != "" {
			if boostOntology, boostErr := ontology.Load(boostUsername); boostErr == nil && boostOntology != nil {
				var boostTerms []string
				for _, node := range boostOntology.Schema.Nodes {
					if node.NodeConfidence != ontology.ConfidenceHigh {
						continue
					}
					boostTerms = append(boostTerms, node.Tags...)
				}
				if len(boostTerms) > 0 {
					query = query + " " + strings.Join(boostTerms, " ")
				}
			}
		}
		results, err := s.knowledgeSearch.Search(query, 3, nil)
		if s.knowledgeHitTracker != nil {
			resultCount := len(results)
			if err != nil {
				resultCount = 0
			}
			s.knowledgeHitTracker.Record(assignment.TaskID, query, resultCount)
		}

		// Cloud team search: supplement local results with team knowledge
		if s.cloudSearchFunc != nil {
			cloudResults, used := s.cloudSearchFunc(query, "", 3)
			if used && len(cloudResults) > 0 {
				// Deduplicate by ID
				seen := make(map[string]bool, len(results))
				for _, r := range results {
					seen[r.ID] = true
				}
				for _, cr := range cloudResults {
					if !seen[cr.ID] {
						results = append(results, cr)
						seen[cr.ID] = true
					}
				}
			}
		}

		if err == nil && len(results) > 0 {
			b.WriteString("\n### Past Solutions\n")
			for _, r := range results {
				fmt.Fprintf(&b, "  - [%s] %s", r.Type, r.Title)
				if s.knowledgeReader != nil {
					if body, err := s.knowledgeReader.GetBody(r.ID); err == nil && body != "" {
						if len(body) > 150 {
							body = body[:150] + "..."
						}
						fmt.Fprintf(&b, " — %s", body)
					}
				}
				b.WriteByte('\n')
			}
		}

		// Scope-warning injection: past review rejections for this scope
		if assignment.Scope != "" {
			warnings, werr := s.knowledgeSearch.Search("scope-warning "+assignment.Scope, 5, nil)
			if werr == nil && len(warnings) > 0 {
				b.WriteString("\n### Past Review Warnings (this scope)\n")
				for _, w := range warnings {
					fmt.Fprintf(&b, "  - **%s**", w.Title)
					if s.knowledgeReader != nil {
						if body, berr := s.knowledgeReader.GetBody(w.ID); berr == nil && body != "" {
							if len(body) > 200 {
								body = body[:200] + "..."
							}
							fmt.Fprintf(&b, ": %s", body)
						}
					}
					b.WriteByte('\n')
				}

				if len(warnings) >= 3 {
					fmt.Fprintf(&b, "\n⚠️ **Repeated rejection pattern** (%d warnings in this scope) — consider adding a validation rule\n", len(warnings))
					s.recordRepeatedPattern(assignment.Scope, assignment.TaskID, warnings)
				}
			}

			// Validation-rule injection: auto-promoted rules for this scope
			rules, rerr := s.knowledgeSearch.Search("validation-rule "+assignment.Scope, 2, nil)
			if rerr == nil && len(rules) > 0 {
				b.WriteString("\n### Validation Rules (enforced)\n")
				for _, r := range rules {
					if s.knowledgeReader != nil {
						if body, berr := s.knowledgeReader.GetBody(r.ID); berr == nil && body != "" {
							if len(body) > 300 {
								body = body[:300] + "..."
							}
							fmt.Fprintf(&b, "%s\n", body)
						}
					}
				}
			}
		}

		// Similarity-based prevention: search for similar past rejections using DoD embedding
		if assignment.DoD != "" {
			similar, serr := s.knowledgeSearch.Search(assignment.DoD, 2, map[string]string{"doc_type": "scope-warning"})
			if serr == nil && len(similar) > 0 {
				b.WriteString("\n### Similar Past Rejections\n")
				for _, sim := range similar {
					fmt.Fprintf(&b, "  - **%s**", sim.Title)
					if s.knowledgeReader != nil {
						if body, berr := s.knowledgeReader.GetBody(sim.ID); berr == nil && body != "" {
							if len(body) > 150 {
								body = body[:150] + "..."
							}
							fmt.Fprintf(&b, ": %s", body)
						}
					}
					b.WriteByte('\n')
				}
			}
		}
	}

	ctx := b.String()
	if ctx != "## Context (auto-injected)\n" { // has content beyond header
		assignment.KnowledgeContext = ctx
	}
}

// enrichWithPersonalOntology appends the user's L1 HIGH-confidence ontology nodes
// as a "Developer Profile" section to KnowledgeContext. This lets workers adapt
// their coding style to the user's established patterns (e.g. error handling, naming).
// Best-effort: errors are silently ignored so task assignment never blocks.
func (s *SQLiteStore) enrichWithPersonalOntology(assignment *TaskAssignment) {
	username := os.Getenv("USER")
	if username == "" {
		return
	}
	personal, err := ontology.Load(username)
	if err != nil || personal == nil || len(personal.Schema.Nodes) == 0 {
		return
	}

	var lines []string
	for path, node := range personal.Schema.Nodes {
		if node.NodeConfidence != ontology.ConfidenceHigh {
			continue
		}
		line := fmt.Sprintf("- %s: %s", path, node.Label)
		if node.Description != "" {
			line += " — " + node.Description
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return
	}

	section := "\n\n## Developer Profile (auto-injected)\n" + strings.Join(lines, "\n")
	assignment.KnowledgeContext += section
}

// enrichWithOntology appends project ontology nodes to KnowledgeContext.
// Filters nodes by scope ("project") or source_role matching the task's domain.
// Best-effort: errors are silently ignored so task assignment never blocks.
func (s *SQLiteStore) enrichWithOntology(assignment *TaskAssignment) {
	if s.projectRoot == "" {
		return
	}
	proj, err := ontology.LoadProject(s.projectRoot)
	if err != nil || proj == nil || len(proj.Schema.Nodes) == 0 {
		return
	}

	domain := assignment.Domain

	var lines []string
	for path, node := range proj.Schema.Nodes {
		if node.Scope == "project" || (domain != "" && node.SourceRole == domain) {
			line := fmt.Sprintf("- %s: %s", path, node.Label)
			if node.Description != "" {
				line += " — " + node.Description
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return
	}

	section := "\n\n## Project Ontology (auto-injected)\n" + strings.Join(lines, "\n")
	assignment.KnowledgeContext += section
}

// recordRepeatedPattern records a repeated scope-warning pattern as knowledge.
// Separated from enrichUnified to keep the read path free of write side-effects.
func (s *SQLiteStore) recordRepeatedPattern(scope, taskID string, warnings []KnowledgeSearchResult) {
	slog.Info("learn-loop: scope-warning pattern detected",
		"scope", scope, "count", len(warnings), "task", taskID)

	if s.knowledgeWriter == nil {
		return
	}
	titles := make([]string, 0, len(warnings))
	for _, w := range warnings {
		titles = append(titles, w.Title)
	}
	count := len(warnings)
	go func() {
		metadata := map[string]any{
			"title":    fmt.Sprintf("Repeated rejection pattern: %s", scope),
			"doc_type": "pattern",
			"tags":     []string{"scope-warning-pattern", scope, "auto-promoted"},
		}
		body := fmt.Sprintf("## Repeated Rejection Pattern\n\nScope: %s\nCount: %d\n\nWarnings:\n", scope, count)
		for _, t := range titles {
			body += fmt.Sprintf("- %s\n", t)
		}
		body += "\nConsider adding a validation rule for this pattern."
		s.knowledgeWriter.CreateExperiment(metadata, body)
	}()
}

// enrichWithDatasetContext loads .cqdata and populates dataset_context.
func (s *SQLiteStore) enrichWithDatasetContext(assignment *TaskAssignment) {
	if s.projectRoot == "" {
		return
	}
	cd, err := cqdata.Load(s.projectRoot)
	if err != nil || cd == nil || len(cd.Datasets) == 0 {
		return
	}
	ctx := make(map[string]string, len(cd.Datasets))
	for key, entry := range cd.Datasets {
		ctx[key] = entry.Name + "@" + entry.Version
	}
	assignment.DatasetContext = ctx
}

