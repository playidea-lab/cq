package ontology

import "strings"

// BoostFromKnowledge updates the user's ontology based on tags from a recorded
// knowledge document. For each tag that matches an existing node (by label or
// path suffix), the node confidence is promoted to "verified". If no match is
// found for a tag, a new node is created under knowledge/<tag> with HIGH
// confidence. Changes are persisted to disk. Errors are returned but callers
// should treat them as non-fatal.
func BoostFromKnowledge(username string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	o, err := Load(username)
	if err != nil {
		return err
	}
	if o.Schema.Nodes == nil {
		o.Schema.Nodes = make(map[string]Node)
	}

	u := NewUpdater(o)
	changed := false

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		// Check if any existing node matches by path suffix or label.
		matchedPath := ""
		for path, node := range o.Schema.Nodes {
			if strings.EqualFold(node.Label, tag) || strings.HasSuffix(path, tag) {
				matchedPath = path
				break
			}
		}

		if matchedPath != "" {
			// Promote existing node to verified.
			existing := o.Schema.Nodes[matchedPath]
			if existing.NodeConfidence != ConfidenceVerified {
				existing.NodeConfidence = ConfidenceVerified
				o.Schema.Nodes[matchedPath] = existing
				changed = true
			}
		} else {
			// Create new node under knowledge/<tag> with HIGH confidence.
			nodePath := "knowledge/" + tag
			u.AddOrUpdate(nodePath, Node{
				Label:          tag,
				Tags:           []string{tag},
				NodeConfidence: ConfidenceHigh,
				SourceRole:     "knowledge",
			})
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return Save(username, o)
}
