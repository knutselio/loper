// Package scanner finds the git repos under the configured paths.
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Scan returns every repo folder found under paths. A repo is a folder that
// contains a .git directory; the scan does not descend into a repo or into a
// folder whose name starts with a dot, and does not follow symlinks.
func Scan(paths []string) []string {
	var repos []string
	for _, root := range paths {
		// Errors are dropped on purpose: a folder that cannot be read, or a
		// root that does not exist, is skipped so the rest of the list loads.
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if info, err := os.Lstat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
				repos = append(repos, path)
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}
