package standards

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	embeddedstd "github.com/changmin/c4-core/standards"
	"gopkg.in/yaml.v3"
)

// DiffStatus represents the comparison result of a standard file.
type DiffStatus string

const (
	DiffMatch    DiffStatus = "match"
	DiffModified DiffStatus = "modified"
	DiffMissing  DiffStatus = "missing"
	DiffExtra    DiffStatus = "extra"
)

// DiffResult holds the comparison result for one file.
type DiffResult struct {
	FileName  string     `json:"file_name"`
	Status    DiffStatus `json:"status"`
	LocalHash string     `json:"local_hash,omitempty"`
	EmbedHash string     `json:"embed_hash,omitempty"`
}

// LockFile represents .piki-lock.yaml
type LockFile struct {
	Team      string      `yaml:"team"`
	Langs     []string    `yaml:"langs"`
	AppliedAt string      `yaml:"applied_at"`
	Files     []LockEntry `yaml:"files"`
}

// LockEntry is one file record in the lock file.
type LockEntry struct {
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
	Hash string `yaml:"hash"`
}

// ReadLock reads and parses .piki-lock.yaml from projectDir.
func ReadLock(projectDir string) (*LockFile, error) {
	lockPath := filepath.Join(projectDir, ".piki-lock.yaml")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("reading lock file: %w", err)
	}
	var lock LockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parsing lock file: %w", err)
	}
	return &lock, nil
}

// Check compares project rules against embedded standards.
func Check(projectDir string) ([]DiffResult, error) {
	lockPath := filepath.Join(projectDir, ".piki-lock.yaml")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []DiffResult{{
				FileName: ".piki-lock.yaml",
				Status:   DiffMissing,
			}}, nil
		}
		return nil, fmt.Errorf("reading lock file: %w", err)
	}

	var lock LockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parsing lock file: %w", err)
	}

	var results []DiffResult
	for _, entry := range lock.Files {
		localPath := filepath.Join(projectDir, entry.Dst)
		localData, err := os.ReadFile(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				results = append(results, DiffResult{
					FileName:  entry.Dst,
					Status:    DiffMissing,
					EmbedHash: entry.Hash,
				})
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", localPath, err)
		}

		localHash := sha256sum(localData)

		// Get embedded hash
		embedData, err := fs.ReadFile(embeddedstd.FS, entry.Src)
		if err != nil {
			continue // embedded file removed in newer version
		}
		embedHash := sha256sum(embedData)

		status := DiffMatch
		if localHash != embedHash {
			status = DiffModified
		}
		results = append(results, DiffResult{
			FileName:  entry.Dst,
			Status:    status,
			LocalHash: localHash,
			EmbedHash: embedHash,
		})
	}

	// Check for extra local-*.md files (not managed)
	rulesDir := filepath.Join(projectDir, ".claude", "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "local-") && strings.HasSuffix(e.Name(), ".md") {
				results = append(results, DiffResult{
					FileName: filepath.Join(".claude", "rules", e.Name()),
					Status:   DiffExtra,
				})
			}
		}
	}

	return results, nil
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
