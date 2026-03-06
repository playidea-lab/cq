package drive

import (
	"io/fs"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// WalkEntry represents a file found during directory traversal.
type WalkEntry struct {
	Path    string // absolute path
	RelPath string // relative to root
	Size    int64
}

// WalkDir walks root recursively, applying .gitignore rules from all ancestor
// directories (root down to each entry's immediate parent) and optionally an
// extra ignore file (e.g. .cqdriveignore). Symlinks are skipped.
// SHA256 is NOT computed here — that is the caller's responsibility.
func WalkDir(root string, extraIgnore string) ([]WalkEntry, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Step 1: Pre-collect all .gitignore matchers, keyed by relative directory
	// path from absRoot. This avoids re-reading the same file multiple times.
	dirMatchers := map[string]*gitignore.GitIgnore{}
	if err := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		gitignorePath := filepath.Join(path, ".gitignore")
		if m, e := gitignore.CompileIgnoreFile(gitignorePath); e == nil {
			relDir, _ := filepath.Rel(absRoot, path)
			dirMatchers[filepath.ToSlash(relDir)] = m
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Load extra ignore rules (e.g. .cqdriveignore).
	var extra *gitignore.GitIgnore
	if extraIgnore != "" {
		if m, e := gitignore.CompileIgnoreFile(extraIgnore); e == nil {
			extra = m
		}
	}

	// Step 2: Walk and filter using all ancestor matchers.
	var entries []WalkEntry
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		if relPath == "." {
			return nil
		}
		relSlash := filepath.ToSlash(relPath)

		// Check all ancestor directory matchers (from root down to file's parent).
		for dirRel, m := range dirMatchers {
			if m == nil {
				continue
			}
			// Compute path relative to this matcher's directory.
			var relToMatcher string
			if dirRel == "." {
				relToMatcher = relSlash
			} else {
				prefix := dirRel + "/"
				if !strings.HasPrefix(relSlash, prefix) {
					continue // entry is not under this matcher's directory
				}
				relToMatcher = relSlash[len(prefix):]
			}
			if m.MatchesPath(relToMatcher) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Check extra ignore file.
		if extra != nil && extra.MatchesPath(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect regular files.
		if d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}

		entries = append(entries, WalkEntry{
			Path:    path,
			RelPath: relSlash,
			Size:    info.Size(),
		})
		return nil
	})

	return entries, err
}
