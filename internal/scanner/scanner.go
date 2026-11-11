// Package scanner finds the git repos under the configured paths.
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Scan returns every repo folder found under paths. A repo is a folder that
// contains a .git directory; the scan does not descend into a repo.
func Scan(paths []string) []string {
	var repos []string
	for _, root := range paths {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
				repos = append(repos, path)
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}
