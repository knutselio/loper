// Package cache reads and writes the cached repo list, so the list can be
// shown at launch before the scan has run.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/knutselio/loper/internal/wholefile"
	"github.com/knutselio/loper/internal/xdg"
)

// List is the cached repo list with the data for every column, when it was
// scanned and from which configured paths.
type List struct {
	ScannedAt time.Time `json:"scanned_at"`
	Paths     []string  `json:"paths"`
	Repos     []Repo    `json:"repos"`
}

// Repo is one repo of the list. Its name and folder come from its path, the
// path the root-path style shows from its path and root, and its last-opened
// time from the history.
type Repo struct {
	Path   string `json:"path"`
	Root   string `json:"root"` // the configured path it was found under, which the tool always writes as its path or a folder above it
	Branch string `json:"branch"`
}

// Store is the cache file.
type Store struct {
	path string
}

// New returns the store in the cache folder: $XDG_CACHE_HOME/loper, or
// ~/.cache/loper.
func New(home string) Store {
	dir := xdg.Dir("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	return Store{path: filepath.Join(dir, "loper", "repos.json")}
}

// Load returns the cached list. A file that is missing, cannot be read, is
// corrupt or was written before repos had a root reads as an empty list: the
// refresh at launch rebuilds it, so losing it must never stop the tool.
func (s Store) Load() List {
	var l List
	b, err := os.ReadFile(s.path)
	if err != nil || json.Unmarshal(b, &l) != nil || slices.ContainsFunc(l.Repos, func(r Repo) bool { return r.Root == "" }) {
		return List{}
	}
	return l
}

// Save replaces the cache file with l, written whole.
func (s Store) Save(l List) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("saving the cache: %w", err)
	}
	if err := wholefile.Write(s.path, b); err != nil {
		return fmt.Errorf("saving the cache to %s: %w", s.path, err)
	}
	return nil
}
