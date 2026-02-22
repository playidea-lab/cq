package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// --- Lighthouse Auto-Promote ---

// autoPromoteLighthouse checks if a completed task is a T-LH- task
// and auto-promotes the linked lighthouse from stub to implemented.
// Best-effort: failures are logged but don't block task completion.
func (s *SQLiteStore) autoPromoteLighthouse(taskID, workerID string) {
	if !strings.HasPrefix(taskID, "T-LH-") {
		return
	}
	// Extract lighthouse name: T-LH-{name}-{ver}
	parts := strings.TrimPrefix(taskID, "T-LH-")
	idx := strings.LastIndex(parts, "-")
	if idx <= 0 {
		return
	}
	lhName := parts[:idx]

	lh, err := s.getLighthouse(lhName)
	if err != nil || lh == nil || lh.Status != "stub" {
		return
	}

	// Promote lighthouse in DB
	if err := s.promoteLighthouse(lhName, workerID); err != nil {
		fmt.Fprintf(os.Stderr, "c4: warning: auto-promote lighthouse '%s' failed: %v\n", lhName, err)
		return
	}

	// Remove stub handler from MCP registry (real handler registered on next restart)
	if s.registry != nil {
		s.registry.Unregister(lhName)
	}

	s.logTrace("lighthouse_auto_promote", workerID, lhName,
		fmt.Sprintf("auto-promoted via task %s completion", taskID))

	// Notify EventBus
	s.notifyEventBus("lighthouse.promoted", map[string]any{
		"lighthouse": lhName,
		"task_id":    taskID,
		"worker_id":  workerID,
	})
}

// --- Knowledge Auto-Record ---

// autoRecordKnowledge records task completion as a knowledge experiment (best-effort).
// Uses native knowledge writer if available, falls back to proxy.
func (s *SQLiteStore) autoRecordKnowledge(task *Task, summary string, filesChanged []string, handoff string) {
	if task == nil {
		return
	}
	if s.knowledgeWriter == nil && s.proxy == nil {
		return
	}

	// Parse handoff to extract structured data
	ho := parseHandoff(handoff)
	if ho.Summary != "" && summary == "submitted via worker" {
		summary = ho.Summary
	}
	if len(ho.FilesChanged) > 0 && len(filesChanged) == 0 {
		filesChanged = ho.FilesChanged
	}

	// Build rich content with rationale and discoveries
	var b strings.Builder
	fmt.Fprintf(&b, "## Task: %s\n\n**Summary**: %s\n\n**Status**: done\n", task.Title, summary)
	if len(filesChanged) > 0 {
		fmt.Fprintf(&b, "\n**Files changed**: %s\n", strings.Join(filesChanged, ", "))
	}
	if len(ho.Discoveries) > 0 {
		b.WriteString("\n## Discoveries\n")
		for _, d := range ho.Discoveries {
			fmt.Fprintf(&b, "- %s\n", d)
		}
	}
	if len(ho.Concerns) > 0 {
		b.WriteString("\n## Concerns\n")
		for _, c := range ho.Concerns {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	if ho.Rationale != "" {
		fmt.Fprintf(&b, "\n## Rationale\n%s\n", ho.Rationale)
	}
	content := b.String()

	tags := []string{}
	if task.Domain != "" {
		tags = append(tags, task.Domain)
	}
	if task.WorkerID != "" {
		tags = append(tags, task.WorkerID)
	}
	tags = append(tags, "auto-recorded")

	title := fmt.Sprintf("Task %s: %s", task.ID, task.Title)

	// Prefer native knowledge writer over proxy
	if s.knowledgeWriter != nil {
		go func() {
			metadata := map[string]any{
				"title":   title,
				"domain":  task.Domain,
				"tags":    tags,
				"task_id": task.ID,
			}
			if _, err := s.knowledgeWriter.CreateExperiment(metadata, content); err != nil {
				fmt.Fprintf(os.Stderr, "c4: auto-record knowledge failed for %s: %v\n", task.ID, err)
			}
		}()
		return
	}

	// Fallback to proxy
	params := map[string]any{
		"doc_type": "experiment",
		"title":    title,
		"content":  content,
		"tags":     tags,
	}
	go func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			if _, err := s.proxy.Call("KnowledgeRecord", params); err != nil {
				fmt.Fprintf(os.Stderr, "c4: auto-record knowledge failed for %s: %v\n", task.ID, err)
			}
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			fmt.Fprintf(os.Stderr, "c4: auto-record knowledge timed out for %s\n", task.ID)
		}
	}()
}

// HandoffEvidence attaches CDP or test artifacts to a submit.
type HandoffEvidence struct {
	Type        string `json:"type"`         // enum: "screenshot"|"log"|"test_result"
	ArtifactID  string `json:"artifact_id"`  // c4_artifact_save로 저장된 ID
	Description string `json:"description"`
}

// Evidence type constants
const (
	EvidenceTypeScreenshot = "screenshot"
	EvidenceTypeLog        = "log"
	EvidenceTypeTestResult = "test_result"
)

// isValidEvidenceType checks if the evidence type is one of the allowed values.
func isValidEvidenceType(t string) bool {
	return t == EvidenceTypeScreenshot || t == EvidenceTypeLog || t == EvidenceTypeTestResult
}

// handoffData holds structured data parsed from a handoff JSON string.
type handoffData struct {
	Summary      string            `json:"summary"`
	FilesChanged []string          `json:"files_changed"`
	Discoveries  []string          `json:"discoveries"`
	Concerns     []string          `json:"concerns"`
	Rationale    string            `json:"rationale"`
	Evidence     []HandoffEvidence `json:"evidence,omitempty"` // CDP/test artifact references
}

// parseHandoff extracts structured fields from a handoff JSON string.
func parseHandoff(handoff string) handoffData {
	if strings.TrimSpace(handoff) == "" {
		return handoffData{}
	}
	var ho handoffData
	if err := json.Unmarshal([]byte(handoff), &ho); err != nil {
		return handoffData{Summary: handoff}
	}
	return ho
}
