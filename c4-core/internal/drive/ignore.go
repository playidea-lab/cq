package drive

import (
	"os"
	"path/filepath"

	gitignore "github.com/sabhiram/go-gitignore"
)

// WalkEntry represents a file found during directory traversal.
type WalkEntry struct {
	Path    string // absolute path
	RelPath string // relative to root
	Size    int64
}

// WalkDir walks root recursively, applying .gitignore rules from each directory
// and optionally an extra ignore file (e.g. .cqdriveignore). Symlinks are skipped.
// SHA256 is NOT computed here — that is the caller's responsibility.
func WalkDir(root string, extraIgnore string) ([]WalkEntry, error) {
	var entries []WalkEntry

	// Load extra ignore rules upfront (e.g. .cqdriveignore).
	var extra *gitignore.GitIgnore
	if extraIgnore != "" {
		if ig, err := gitignore.CompileIgnoreFile(extraIgnore); err == nil {
			extra = ig
		}
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		// Skip the root itself.
		if relPath == "." {
			return nil
		}

		// Skip symlinks.
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Load .gitignore from the directory containing this entry.
		dir := filepath.Dir(path)
		gitignorePath := filepath.Join(dir, ".gitignore")
		var ig *gitignore.GitIgnore
		if _, statErr := os.Stat(gitignorePath); statErr == nil {
			ig, _ = gitignore.CompileIgnoreFile(gitignorePath)
		}

		// Check ignore rules using relPath (forward slashes for gitignore matching).
		relFwd := filepath.ToSlash(relPath)
		if ig != nil && ig.MatchesPath(relFwd) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if extra != nil && extra.MatchesPath(relFwd) {
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
			RelPath: relPath,
			Size:    info.Size(),
		})
		return nil
	})

	return entries, err
}
