package ontology

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EvidenceThreshold is the minimum frequency for a node to trigger a piki proposal.
const EvidenceThreshold = 10

// ProposalCooldown is the minimum interval between proposal runs.
const ProposalCooldown = 7 * 24 * time.Hour

// pikiFrontMatter extracts lines beginning with "- " from a markdown rule file.
// These represent bullet-point rules that can be matched against ontology nodes.
func pikiFrontMatter(content string) []string {
	var rules []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			rules = append(rules, strings.TrimPrefix(line, "- "))
		}
	}
	return rules
}

// PikiRule represents a single extracted rule from piki standards.
type PikiRule struct {
	// File is the source markdown file (relative path).
	File string
	// Text is the rule text.
	Text string
}

// Conflict describes a mismatch between a project ontology node and piki rules.
type Conflict struct {
	// NodePath is the key of the ontology node.
	NodePath string
	// NodeLabel is the human-readable label of the node.
	NodeLabel string
	// EvidenceCount is the frequency of the node.
	EvidenceCount int
	// NodeDescription is the description of the ontology node.
	NodeDescription string
	// ConflictingRules are the piki rules that appear to contradict the node.
	ConflictingRules []PikiRule
}

// Proposal is the YAML file written to .c4/piki-proposals/.
type Proposal struct {
	// GeneratedAt is when this proposal was created.
	GeneratedAt time.Time `yaml:"generated_at"`
	// Conflicts lists each detected conflict with evidence.
	Conflicts []ProposalConflict `yaml:"conflicts"`
}

// ProposalConflict is the YAML representation of a single Conflict.
type ProposalConflict struct {
	NodePath        string   `yaml:"node_path"`
	NodeLabel       string   `yaml:"node_label"`
	EvidenceCount   int      `yaml:"evidence_count"`
	NodeDescription string   `yaml:"node_description,omitempty"`
	ConflictSources []string `yaml:"conflict_sources"`
	SuggestedUpdate string   `yaml:"suggested_update"`
}

// runStatePath returns the path of the run-state file used for rate limiting.
func runStatePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".c4", "piki-proposals", ".last-run")
}

// proposalDir returns the directory where proposals are written.
func proposalDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".c4", "piki-proposals")
}

// loadLastRun reads the timestamp of the last successful proposal run.
// Returns zero time if the state file does not exist.
func loadLastRun(projectRoot string) (time.Time, error) {
	data, err := os.ReadFile(runStatePath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("read last-run: %w", err)
	}
	var t time.Time
	if err := yaml.Unmarshal(data, &t); err != nil {
		return time.Time{}, fmt.Errorf("parse last-run: %w", err)
	}
	return t, nil
}

// saveLastRun writes the current time as the last run timestamp.
func saveLastRun(projectRoot string, now time.Time) error {
	dir := proposalDir(projectRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create proposals dir: %w", err)
	}
	out, err := yaml.Marshal(now)
	if err != nil {
		return fmt.Errorf("marshal last-run: %w", err)
	}
	return os.WriteFile(runStatePath(projectRoot), out, 0644)
}

// LoadPikiRules reads markdown files from the .claude/rules/ directory in the
// project root and extracts bullet-point rules from each file.
func LoadPikiRules(projectRoot string) ([]PikiRule, error) {
	rulesDir := filepath.Join(projectRoot, ".claude", "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read rules dir: %w", err)
	}

	var rules []PikiRule
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(rulesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read rule file %s: %w", e.Name(), err)
		}
		for _, text := range pikiFrontMatter(string(data)) {
			rules = append(rules, PikiRule{
				File: e.Name(),
				Text: text,
			})
		}
	}
	return rules, nil
}

// negationPrefixes are words that indicate a node negates or contradicts a rule.
var negationPrefixes = []string{"not ", "no ", "never ", "without ", "skip ", "avoid ", "ignore ", "disable ", "bypass ", "override "}

// detectConflicts compares project ontology nodes against piki rules.
// A conflict is raised when a node has frequency >= EvidenceThreshold and its
// label or description uses a negation pattern while sharing keywords with a
// piki rule (indicating the node contradicts the rule).
// The detection is keyword-based to remain dependency-free.
func detectConflicts(proj *ProjectOntology, rules []PikiRule) []Conflict {
	var conflicts []Conflict
	for path, node := range proj.Schema.Nodes {
		if node.Frequency < EvidenceThreshold {
			continue
		}
		nodeText := strings.ToLower(node.Label + " " + node.Description)

		// Check whether the node text contains any negation prefix.
		hasNegation := false
		for _, neg := range negationPrefixes {
			if strings.Contains(nodeText, neg) {
				hasNegation = true
				break
			}
		}
		if !hasNegation {
			continue
		}

		// Find rules whose keywords overlap with the node text, indicating
		// the node may be contradicting a piki rule.
		var matching []PikiRule
		for _, rule := range rules {
			ruleText := strings.ToLower(rule.Text)
			if keywordOverlap(nodeText, ruleText) {
				matching = append(matching, rule)
			}
		}
		if len(matching) > 0 {
			conflicts = append(conflicts, Conflict{
				NodePath:         path,
				NodeLabel:        node.Label,
				EvidenceCount:    node.Frequency,
				NodeDescription:  node.Description,
				ConflictingRules: matching,
			})
		}
	}
	return conflicts
}

// keywordOverlap returns true if the two lower-cased strings share at least one
// meaningful word (length > 3).
func keywordOverlap(a, b string) bool {
	words := strings.Fields(a)
	for _, w := range words {
		if len(w) > 3 && strings.Contains(b, w) {
			return true
		}
	}
	return false
}

// buildProposal converts a slice of Conflict into a Proposal.
func buildProposal(conflicts []Conflict, now time.Time) Proposal {
	p := Proposal{GeneratedAt: now}
	for _, c := range conflicts {
		var sources []string
		seen := map[string]bool{}
		for _, r := range c.ConflictingRules {
			if !seen[r.File] {
				sources = append(sources, r.File)
				seen[r.File] = true
			}
		}
		p.Conflicts = append(p.Conflicts, ProposalConflict{
			NodePath:        c.NodePath,
			NodeLabel:       c.NodeLabel,
			EvidenceCount:   c.EvidenceCount,
			NodeDescription: c.NodeDescription,
			ConflictSources: sources,
			SuggestedUpdate: fmt.Sprintf(
				"Node %q (evidence=%d) contradicts piki standard(s) in %s. "+
					"Consider updating the piki rule or retiring this node.",
				c.NodeLabel, c.EvidenceCount, strings.Join(sources, ", "),
			),
		})
	}
	return p
}

// writeProposal serialises the Proposal to a timestamped YAML file in the
// proposals directory.
func writeProposal(projectRoot string, p Proposal) (string, error) {
	dir := proposalDir(projectRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create proposals dir: %w", err)
	}
	name := fmt.Sprintf("proposal-%s.yaml", p.GeneratedAt.UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	out, err := yaml.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal proposal: %w", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return "", fmt.Errorf("write proposal: %w", err)
	}
	return path, nil
}

// DetectPikiConflicts is the main entry point for the PikiConflictDetector.
// It loads the project ontology and piki rules, detects conflicts on nodes
// with evidence_count >= EvidenceThreshold, and — when conflicts exist —
// writes a proposal YAML file to .c4/piki-proposals/.
//
// The function is rate-limited to once per ProposalCooldown (default: 7 days).
// If cooldown has not elapsed, it returns (0, "", nil) immediately.
//
// Returns the number of conflicts found and the path of the written proposal
// (empty when no proposal was written).
func DetectPikiConflicts(projectRoot string) (int, string, error) {
	// Rate-limit check.
	last, err := loadLastRun(projectRoot)
	if err != nil {
		return 0, "", fmt.Errorf("load last run: %w", err)
	}
	now := time.Now().UTC()
	if !last.IsZero() && now.Sub(last) < ProposalCooldown {
		return 0, "", nil
	}

	// Load project ontology.
	proj, err := LoadProject(projectRoot)
	if err != nil {
		return 0, "", fmt.Errorf("load project ontology: %w", err)
	}

	// Load piki rules.
	rules, err := LoadPikiRules(projectRoot)
	if err != nil {
		return 0, "", fmt.Errorf("load piki rules: %w", err)
	}

	// Detect conflicts.
	conflicts := detectConflicts(proj, rules)
	n := len(conflicts)

	// Save last-run timestamp regardless so we don't hammer the FS.
	if err := saveLastRun(projectRoot, now); err != nil {
		return n, "", fmt.Errorf("save last-run: %w", err)
	}

	if n == 0 {
		return 0, "", nil
	}

	// Write proposal.
	proposal := buildProposal(conflicts, now)
	path, err := writeProposal(projectRoot, proposal)
	if err != nil {
		return n, "", fmt.Errorf("write proposal: %w", err)
	}
	return n, path, nil
}
