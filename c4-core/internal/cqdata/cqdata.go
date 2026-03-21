// Package cqdata manages the .cqdata YAML file for a project directory.
// It stores dataset and artifact references (name + version) keyed by a string key.
package cqdata

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const filename = ".cqdata"

// Entry holds a name and version string.
type Entry struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// CQData is the in-memory representation of the .cqdata file.
type CQData struct {
	Datasets  map[string]Entry `yaml:"datasets,omitempty"`
	Artifacts map[string]Entry `yaml:"artifacts,omitempty"`
}

// Load reads the .cqdata file from dir.
// If the file does not exist, an empty *CQData is returned without error.
func Load(dir string) (*CQData, error) {
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &CQData{}, nil
		}
		return nil, err
	}
	var cd CQData
	if err := yaml.Unmarshal(data, &cd); err != nil {
		return nil, err
	}
	return &cd, nil
}

// Save writes the CQData to the .cqdata file in dir.
func (cd *CQData) Save(dir string) error {
	path := filepath.Join(dir, filename)
	data, err := yaml.Marshal(cd)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// GetDataset returns the dataset entry for key.
// ok is false if the key does not exist.
func (cd *CQData) GetDataset(key string) (name, version string, ok bool) {
	if cd.Datasets == nil {
		return "", "", false
	}
	e, ok := cd.Datasets[key]
	return e.Name, e.Version, ok
}

// SetDataset stores or updates the dataset entry for key.
func (cd *CQData) SetDataset(key, name, version string) {
	if cd.Datasets == nil {
		cd.Datasets = make(map[string]Entry)
	}
	cd.Datasets[key] = Entry{Name: name, Version: version}
}
