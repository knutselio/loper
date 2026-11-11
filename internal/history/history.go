// Package history reads and writes the time each repo was last opened,
// keyed by the repo's absolute path.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/knutselio/loper/internal/wholefile"
	"github.com/knutselio/loper/internal/xdg"
)

// Store is the history file.
type Store struct {
	path string
}

// New returns the store in the state folder: $XDG_STATE_HOME/loper, or
// ~/.local/state/loper.
func New(home string) Store {
	dir := xdg.Dir("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return Store{path: filepath.Join(dir, "loper", "history.json")}
}

// Load returns the last-opened times. A file that cannot be read reads as
// an empty history: the history only labels the rows, so losing it must
// never stop the tool.
func (s Store) Load() map[string]time.Time {
	h, _ := s.read() // the error is for Record, which must not overwrite such a file
	return h
}

// read returns the last-opened times, never nil. A missing or corrupt file
// reads as empty without an error, as there is nothing in it to keep. Any
// other failure to read is an error along with an empty history.
func (s Store) read() (map[string]time.Time, error) {
	h := map[string]time.Time{}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	var read map[string]time.Time
	if json.Unmarshal(b, &read) != nil {
		return h, nil
	}
	maps.Copy(h, read)
	return h, nil
}

// Record sets repo's last-opened time to at, keeping every other entry in
// the file. The file is written whole, through a temporary file renamed over
// it, so a reader never sees half of it.
func (s Store) Record(repo string, at time.Time) error {
	h, err := s.read()
	if err != nil {
		return fmt.Errorf("saving the history: %w", err)
	}
	h[repo] = at
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("saving the history: %w", err)
	}
	if err := wholefile.Write(s.path, b); err != nil {
		return fmt.Errorf("saving the history to %s: %w", s.path, err)
	}
	return nil
}
