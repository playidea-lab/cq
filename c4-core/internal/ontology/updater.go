package ontology

// Updater provides methods to add, update, and promote nodes in an Ontology.
type Updater struct {
	ontology *Ontology
}

// NewUpdater creates an Updater for the given Ontology.
// It initialises the Nodes map if nil.
func NewUpdater(o *Ontology) *Updater {
	if o.Schema.Nodes == nil {
		o.Schema.Nodes = make(map[string]Node)
	}
	return &Updater{ontology: o}
}

// Ontology returns the underlying ontology being updated.
func (u *Updater) Ontology() *Ontology {
	return u.ontology
}

// AddOrUpdate inserts a new node at path or, if a node with the same path
// already exists, increments its frequency and merges non-empty fields.
// When frequency reaches PromotionThreshold the confidence is auto-promoted
// to HIGH.
func (u *Updater) AddOrUpdate(path string, incoming Node) Node {
	existing, found := u.ontology.Schema.Nodes[path]
	if !found {
		// New node — set defaults.
		if incoming.Frequency == 0 {
			incoming.Frequency = 1
		}
		if incoming.NodeConfidence == "" {
			incoming.NodeConfidence = ConfidenceLow
		}
		u.ontology.Schema.Nodes[path] = incoming
		return incoming
	}

	// Duplicate path — increment frequency and merge fields.
	existing.Frequency++

	if incoming.Label != "" {
		existing.Label = incoming.Label
	}
	if incoming.Description != "" {
		existing.Description = incoming.Description
	}
	if len(incoming.Tags) > 0 {
		existing.Tags = mergeTags(existing.Tags, incoming.Tags)
	}
	if len(incoming.Properties) > 0 {
		if existing.Properties == nil {
			existing.Properties = make(map[string]string)
		}
		for k, v := range incoming.Properties {
			existing.Properties[k] = v
		}
	}

	// Auto-promote confidence when threshold is reached.
	if existing.Frequency >= PromotionThreshold {
		existing.NodeConfidence = ConfidenceHigh
	}

	u.ontology.Schema.Nodes[path] = existing
	return existing
}

// mergeTags returns a deduplicated union of a and b, preserving order of a.
func mergeTags(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, t := range a {
		seen[t] = true
	}
	merged := append([]string(nil), a...)
	for _, t := range b {
		if !seen[t] {
			merged = append(merged, t)
			seen[t] = true
		}
	}
	return merged
}
