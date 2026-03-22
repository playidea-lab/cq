package ontology

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultVersion = "1.0.0"

// ontologyPath returns the canonical path for a user's ontology file.
// If username is empty, "default" is used.
func ontologyPath(username string) (string, error) {
	if username == "" {
		username = "default"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "personas", username, "ontology.yaml"), nil
}

// Load reads the ontology for the given username from disk.
// If the file does not exist, an empty Ontology with the default version is returned.
func Load(username string) (*Ontology, error) {
	path, err := ontologyPath(username)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Ontology{Version: defaultVersion}, nil
		}
		return nil, fmt.Errorf("read ontology: %w", err)
	}

	var o Ontology
	if err := yaml.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("parse ontology: %w", err)
	}
	return &o, nil
}

// Save writes the ontology to disk for the given username.
// It updates UpdatedAt before writing.
func Save(username string, o *Ontology) error {
	path, err := ontologyPath(username)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ontology dir: %w", err)
	}

	o.UpdatedAt = time.Now().UTC()

	out, err := yaml.Marshal(o)
	if err != nil {
		return fmt.Errorf("marshal ontology: %w", err)
	}

	return os.WriteFile(path, out, 0644)
}

// Backup copies the current ontology file to <path>.bak.
// If the ontology file does not exist, Backup is a no-op.
func Backup(username string) error {
	path, err := ontologyPath(username)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read ontology for backup: %w", err)
	}

	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// Restore replaces the ontology file with the backup (<path>.bak).
// Returns an error if the backup does not exist.
func Restore(username string) error {
	path, err := ontologyPath(username)
	if err != nil {
		return err
	}

	backupPath := path + ".bak"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup not found: %s", backupPath)
		}
		return fmt.Errorf("read backup: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ontology dir: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
