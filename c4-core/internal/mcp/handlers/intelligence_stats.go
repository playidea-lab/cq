package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/ontology"
)

// IntelligenceStatsDeps holds optional dependencies for the intelligence stats handler.
// All fields are optional; missing sources yield zero values (non-fatal).
type IntelligenceStatsDeps struct {
	KnowledgeStore  *knowledge.Store     // nil if knowledge DB unavailable
	HitTracker      *KnowledgeHitTracker // nil if hit tracking unavailable
	ProjectRoot     string               // for L2 + L3 pending-uploads.json
	OntologyUser    string               // username for L1 ontology (~/.c4/personas/<user>/ontology.yaml)
}

// RegisterIntelligenceStatsHandler registers the c4_intelligence_stats MCP tool.
func RegisterIntelligenceStatsHandler(reg *mcp.Registry, deps *IntelligenceStatsDeps) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_intelligence_stats",
		Description: "Show knowledge + ontology + circulation monitoring stats. Returns doc counts, ontology node distributions, and knowledge flow metrics.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		return handleIntelligenceStats(deps)
	})
}

func handleIntelligenceStats(deps *IntelligenceStatsDeps) (any, error) {
	if deps == nil {
		deps = &IntelligenceStatsDeps{}
	}

	var l1 *ontology.Ontology
	if deps.OntologyUser != "" {
		if o, err := ontology.Load(deps.OntologyUser); err == nil {
			l1 = o
		} else {
			fmt.Fprintf(os.Stderr, "c4_intelligence_stats: load L1 ontology: %v\n", err)
		}
	}

	return map[string]any{
		"knowledge":   buildKnowledgeStats(deps),
		"ontology":    buildOntologyStats(deps, l1),
		"circulation": buildCirculationStats(deps, l1),
	}, nil
}

// buildKnowledgeStats returns total_docs, by_type, and search_hit_rate.
func buildKnowledgeStats(deps *IntelligenceStatsDeps) map[string]any {
	stats := map[string]any{
		"total_docs":      0,
		"by_type":         map[string]int{},
		"search_hit_rate": 0.0,
	}

	if deps.KnowledgeStore != nil {
		docs, err := deps.KnowledgeStore.List("", "", 10000)
		if err == nil {
			typeCounts := map[string]int{}
			for _, d := range docs {
				dt, _ := d["type"].(string)
				typeCounts[dt]++
			}
			stats["total_docs"] = len(docs)
			stats["by_type"] = typeCounts
		} else {
			fmt.Fprintf(os.Stderr, "c4_intelligence_stats: knowledge list: %v\n", err)
		}
	}

	if deps.HitTracker != nil {
		r := deps.HitTracker.Report()
		stats["search_hit_rate"] = r.HitRate
		stats["search_total"] = r.TotalSearches
		stats["search_hits"] = r.Hits
		stats["search_misses"] = r.Misses
	}

	return stats
}

// buildOntologyStats reads L1 (user persona ontology), L2 (project ontology), and L3 (hub) stats.
func buildOntologyStats(deps *IntelligenceStatsDeps, l1 *ontology.Ontology) map[string]any {
	stats := map[string]any{
		"l1_nodes":       0,
		"l1_confidence":  map[string]int{},
		"l2_nodes":       0,
		"l3_uploaded":    0,
		"l3_downloaded":  0,
	}

	// L1: user personal ontology
	if l1 != nil {
		stats["l1_nodes"] = len(l1.Schema.Nodes)
		confDist := map[string]int{}
		for _, node := range l1.Schema.Nodes {
			conf := string(node.NodeConfidence)
			if conf == "" {
				conf = "unset"
			}
			confDist[conf]++
		}
		stats["l1_confidence"] = confDist
	}

	// L2: project ontology
	if deps.ProjectRoot != "" {
		if o, err := ontology.LoadProject(deps.ProjectRoot); err == nil {
			stats["l2_nodes"] = len(o.Schema.Nodes)
		} else {
			fmt.Fprintf(os.Stderr, "c4_intelligence_stats: load L2 ontology: %v\n", err)
		}
	}

	// L3: hub upload pending queue (best-effort indicator for upload activity)
	if deps.ProjectRoot != "" {
		pendingPath := filepath.Join(deps.ProjectRoot, ".c4", "pending-uploads.json")
		if data, err := os.ReadFile(pendingPath); err == nil {
			var pending []any
			if jsonErr := json.Unmarshal(data, &pending); jsonErr == nil {
				stats["l3_uploaded"] = 0       // successfully uploaded patterns (not tracked persistently)
				stats["l3_pending_queue"] = len(pending) // queued for retry
			}
		}
		// l3_downloaded is not tracked persistently; report 0
	}

	return stats
}

// buildCirculationStats reports knowledge flow metrics derived from ontology files.
func buildCirculationStats(deps *IntelligenceStatsDeps, l1 *ontology.Ontology) map[string]any {
	stats := map[string]any{
		"knowledge_boosts":          0,
		"personalized_searches":     0,
		"cross_position_detections": 0,
	}

	// knowledge_boosts: count L1 nodes with source_role="knowledge"
	// These nodes were created by BoostFromKnowledge calls.
	if l1 != nil {
		boostCount := 0
		crossCount := 0
		highCount := 0
		for _, node := range l1.Schema.Nodes {
			if node.SourceRole == "knowledge" {
				boostCount++
			}
			// cross-position nodes are tagged with scope "cross:*→*"
			if len(node.Scope) > 5 && node.Scope[:5] == "cross" {
				crossCount++
			}
			// personalized_searches: count L1 HIGH/verified nodes (these influence personalized ranking)
			// This is a proxy metric — actual personalized search calls are not tracked persistently.
			if node.NodeConfidence == ontology.ConfidenceHigh || node.NodeConfidence == ontology.ConfidenceVerified {
				highCount++
			}
		}
		stats["knowledge_boosts"] = boostCount
		stats["cross_position_detections"] = crossCount
		stats["personalized_searches"] = highCount
	}

	return stats
}
