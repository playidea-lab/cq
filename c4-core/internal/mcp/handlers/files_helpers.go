package handlers

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolvePath resolves a path relative to rootDir, preventing directory traversal.
func resolvePath(rootDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		cleaned := filepath.Clean(path)
		if !strings.HasPrefix(cleaned, filepath.Clean(rootDir)) {
			return "", fmt.Errorf("absolute path escapes project root: %s", path)
		}
		return cleaned, nil
	}
	resolved := filepath.Join(rootDir, path)
	resolved = filepath.Clean(resolved)
	if !strings.HasPrefix(resolved, filepath.Clean(rootDir)) {
		return "", fmt.Errorf("path escapes project root: %s", path)
	}
	return resolved, nil
}
