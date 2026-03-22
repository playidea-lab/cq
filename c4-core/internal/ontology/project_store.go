package ontology

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const projectOntologyFile = "project-ontology.yaml"

// projectOntologyPath returns the path to .c4/project-ontology.yaml relative to the
// given project root. If root is empty, the current working directory is used.
func projectOntologyPath(root string) (string, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working dir: %w", err)
		}
	}
	return filepath.Join(root, ".c4", projectOntologyFile), nil
}

// LoadProject reads the project ontology from <root>/.c4/project-ontology.yaml.
// If the file does not exist, an empty ProjectOntology with the default version is returned.
func LoadProject(root string) (*ProjectOntology, error) {
	path, err := projectOntologyPath(root)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectOntology{Version: defaultVersion}, nil
		}
		return nil, fmt.Errorf("read project ontology: %w", err)
	}

	var o ProjectOntology
	if err := yaml.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("parse project ontology: %w", err)
	}
	return &o, nil
}

// SaveProject writes the project ontology to <root>/.c4/project-ontology.yaml.
// It updates UpdatedAt before writing.
func SaveProject(root string, o *ProjectOntology) error {
	path, err := projectOntologyPath(root)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create project ontology dir: %w", err)
	}

	o.UpdatedAt = time.Now().UTC()

	out, err := yaml.Marshal(o)
	if err != nil {
		return fmt.Errorf("marshal project ontology: %w", err)
	}

	return os.WriteFile(path, out, 0644)
}

// BackupProject copies the project ontology file to <path>.bak.
// If the file does not exist, BackupProject is a no-op.
func BackupProject(root string) error {
	path, err := projectOntologyPath(root)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read project ontology for backup: %w", err)
	}

	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("write project ontology backup: %w", err)
	}
	return nil
}
