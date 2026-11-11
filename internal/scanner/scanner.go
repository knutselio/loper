// Package scanner finds the git repos under the configured paths.
package scanner

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/knutselio/loper/internal/repoinfo"
)

// Repo is a repo found by a scan: its folder, and the configured path it was
// found under, its root, both as written in the config.
type Repo struct {
	Path, Root string
}

// Scan returns every repo found under paths. A repo is a folder that
// contains a .git directory; the scan does not descend into a repo or into a
// folder whose name starts with a dot, and follows no symlink but a root. A
// repo found under overlapping paths is listed once, at its first
// occurrence, with the root of that occurrence, unless it is also listed
// directly: then it has that entry as its root.
func Scan(paths []string) []Repo {
	var repos []Repo
	seen := map[string]int{} // the index in repos of each repo found
	for _, root := range paths {
		// A root may be a symlink, which is followed; the repos under it are
		// reported under the root as written. A root that cannot be resolved
		// is skipped, and Missing reports it if it does not exist; an entry
		// that is a file simply holds no repos.
		walkRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		// Errors are dropped on purpose: a folder that cannot be read is
		// skipped so the rest of the list loads.
		filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != walkRoot && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if repoinfo.Exists(path) {
				rel, _ := filepath.Rel(walkRoot, path)
				repo := filepath.Join(root, rel)
				if i, ok := seen[repo]; !ok {
					seen[repo] = len(repos)
					repos = append(repos, Repo{Path: repo, Root: root})
				} else if path == walkRoot {
					repos[i].Root = root // listed directly as well: that root, whatever the order
				}
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}

// Missing returns the paths that do not exist. Only a missing path is a
// warning, as the likely typo; any other failure, such as no permission, is
// skipped by Scan like an unreadable folder.
func Missing(paths []string) []string {
	var missing []string
	for _, root := range paths {
		if _, err := filepath.EvalSymlinks(root); errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, root)
		}
	}
	return missing
}
