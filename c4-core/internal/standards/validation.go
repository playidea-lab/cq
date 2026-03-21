package standards

import "fmt"

// ValidationCmd represents a named validation command for a project.
type ValidationCmd struct {
	Name    string // e.g. "lint", "build", "unit"
	Command string // e.g. "go vet ./..."
}

// ValidationCommands returns the ordered list of validation commands for the
// given team and language selections. Commands from multiple languages are
// deduplicated by name, with later entries winning.
func ValidationCommands(team string, langs []string) ([]ValidationCmd, error) {
	m, err := Parse()
	if err != nil {
		return nil, fmt.Errorf("standards: ValidationCommands: %w", err)
	}

	// Resolve langs (use team defaults when langs is empty).
	resolvedLangs := langs
	if len(resolvedLangs) == 0 && team != "" {
		if tl, ok := m.Teams[team]; ok {
			resolvedLangs = tl.DefaultLangs
		}
	}

	// ordered dedup — preserve first occurrence order, later occurrence wins value.
	order := []string{}
	byName := map[string]string{}

	for _, lang := range resolvedLangs {
		_, ll := m.ResolveLanguage(lang)
		if ll == nil {
			continue
		}
		for name, cmd := range ll.Validation {
			if _, seen := byName[name]; !seen {
				order = append(order, name)
			}
			byName[name] = cmd
		}
	}

	cmds := make([]ValidationCmd, 0, len(order))
	for _, name := range order {
		cmds = append(cmds, ValidationCmd{Name: name, Command: byName[name]})
	}
	return cmds, nil
}
