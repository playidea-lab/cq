package ontology

import "fmt"

// GlobalUsername is the reserved username for the shared global ontology.
const GlobalUsername = "_global"

// SeedFromProject copies scope="project" nodes from the project ontology into
// the L1 (personal) ontology of the given user. Nodes are merged, not
// overwritten, via Updater.AddOrUpdate. If the project ontology is empty or
// contains no project-scoped nodes, returns (0, nil).
func SeedFromProject(username, projectRoot string) (int, error) {
	if username == "" || username == GlobalUsername {
		return 0, fmt.Errorf("invalid seed target: %q", username)
	}

	proj, err := LoadProject(projectRoot)
	if err != nil {
		return 0, fmt.Errorf("load project ontology: %w", err)
	}

	// Filter only scope="project" nodes.
	projectNodes := make(map[string]Node)
	for path, node := range proj.Schema.Nodes {
		if node.Scope == "project" {
			projectNodes[path] = node
		}
	}
	if len(projectNodes) == 0 {
		return 0, nil
	}

	local, err := Load(username)
	if err != nil {
		return 0, fmt.Errorf("load local ontology: %w", err)
	}

	u := NewUpdater(local)
	for path, node := range projectNodes {
		u.AddOrUpdate(path, node)
	}

	if err := Save(username, local); err != nil {
		return 0, fmt.Errorf("save local ontology: %w", err)
	}
	return len(projectNodes), nil
}

// SeedFromGlobal copies the global ontology into the local ontology for the
// given username. If the user already has nodes, the global nodes are merged
// (not overwritten) via Updater.AddOrUpdate. If no global ontology exists,
// this is a no-op and returns (0, nil).
func SeedFromGlobal(username string) (int, error) {
	if username == "" || username == GlobalUsername {
		return 0, fmt.Errorf("invalid seed target: %q", username)
	}

	global, err := Load(GlobalUsername)
	if err != nil {
		return 0, fmt.Errorf("load global ontology: %w", err)
	}
	if len(global.Schema.Nodes) == 0 {
		return 0, nil
	}

	local, err := Load(username)
	if err != nil {
		return 0, fmt.Errorf("load local ontology: %w", err)
	}

	u := NewUpdater(local)
	count := 0
	for path, node := range global.Schema.Nodes {
		u.AddOrUpdate(path, node)
		count++
	}

	if err := Save(username, local); err != nil {
		return 0, fmt.Errorf("save local ontology: %w", err)
	}
	return count, nil
}

// MergeToGlobal merges the ontology of the given username into the global
// ontology. Duplicate nodes are deduplicated via Updater.AddOrUpdate (frequency
// increment + field merge). Returns the number of nodes processed.
func MergeToGlobal(username string) (int, error) {
	if username == "" || username == GlobalUsername {
		return 0, fmt.Errorf("invalid merge source: %q", username)
	}

	local, err := Load(username)
	if err != nil {
		return 0, fmt.Errorf("load local ontology: %w", err)
	}
	if len(local.Schema.Nodes) == 0 {
		return 0, nil
	}

	global, err := Load(GlobalUsername)
	if err != nil {
		return 0, fmt.Errorf("load global ontology: %w", err)
	}

	u := NewUpdater(global)
	count := 0
	for path, node := range local.Schema.Nodes {
		u.AddOrUpdate(path, node)
		count++
	}

	if err := Save(GlobalUsername, global); err != nil {
		return 0, fmt.Errorf("save global ontology: %w", err)
	}
	return count, nil
}
