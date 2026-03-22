package ontology

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// teamConfig is a minimal representation of .c4/team.yaml used for role lookup.
type teamConfig struct {
	Members map[string]teamMember `yaml:"members"`
}

// teamMember holds the role information for a user in team.yaml.
type teamMember struct {
	Role string `yaml:"role"`
}

// loadTeamConfig reads .c4/team.yaml from the project root.
// If the file does not exist, an empty teamConfig is returned.
func loadTeamConfig(root string) (teamConfig, error) {
	path := filepath.Join(root, ".c4", "team.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return teamConfig{}, nil
		}
		return teamConfig{}, fmt.Errorf("read team.yaml: %w", err)
	}
	var tc teamConfig
	if err := yaml.Unmarshal(data, &tc); err != nil {
		return teamConfig{}, fmt.Errorf("parse team.yaml: %w", err)
	}
	return tc, nil
}

// ExtractHighConfidence reads the L1 ontology for the given username, filters
// nodes whose confidence is HIGH, tags each node with the user's role from
// team.yaml (source_role), and merges the result into the project ontology at
// <projectRoot>/.c4/project-ontology.yaml.
//
// Returns the number of nodes merged. If the L1 ontology has no HIGH-confidence
// nodes, the project ontology is left unchanged and (0, nil) is returned.
func ExtractHighConfidence(username, projectRoot string) (int, error) {
	if username == "" {
		return 0, fmt.Errorf("username must not be empty")
	}

	// Load L1 personal ontology.
	l1, err := Load(username)
	if err != nil {
		return 0, fmt.Errorf("load L1 ontology for %q: %w", username, err)
	}

	// Collect HIGH-confidence nodes.
	highNodes := make(map[string]Node)
	for path, node := range l1.Schema.Nodes {
		if node.NodeConfidence == ConfidenceHigh {
			highNodes[path] = node
		}
	}
	if len(highNodes) == 0 {
		return 0, nil
	}

	// Determine source_role from team.yaml.
	tc, err := loadTeamConfig(projectRoot)
	if err != nil {
		return 0, fmt.Errorf("load team config: %w", err)
	}
	sourceRole := ""
	if m, ok := tc.Members[username]; ok {
		sourceRole = m.Role
	}

	// Load existing project ontology.
	proj, err := LoadProject(projectRoot)
	if err != nil {
		return 0, fmt.Errorf("load project ontology: %w", err)
	}
	if proj.Schema.Nodes == nil {
		proj.Schema.Nodes = make(map[string]Node)
	}

	// Merge HIGH-confidence nodes into the project ontology.
	count := 0
	for path, node := range highNodes {
		node.Scope = "project"
		if sourceRole != "" {
			node.SourceRole = sourceRole
		}
		proj.Schema.Nodes[path] = node
		count++
	}

	if err := SaveProject(projectRoot, proj); err != nil {
		return 0, fmt.Errorf("save project ontology: %w", err)
	}
	return count, nil
}
