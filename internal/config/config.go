// Package config loads and validates the config file and applies defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/xdg"
)

// Settings is the complete configuration, with every default applied.
type Settings struct {
	File        string          // the config file, read or, when missing, where it would be
	Paths       []string        // absolute folders to scan, or repos to list directly
	Targets     []target.Target // in the order they are written, perhaps none
	CloseOnOpen bool            // quit after a successful open
	SortByName  bool            // start sorted by name rather than by last opened
	Columns     []string        // the columns shown, in order; "name" is one of them
	RowStyle    string          // how name and folder are drawn: "name-folder", "root-path" or "name"
	Selection   string          // how the selected row is marked: "bar" or "marker"
	// CacheMaxAge is how old the cache may be before a launch scans again;
	// 0 means every launch scans.
	CacheMaxAge time.Duration
	// Notice says why the config file could not be created on a first run,
	// as a notice, or is "".
	Notice string
	// Created is true when the picker created the config file on this run.
	Created bool
}

// file is the config file as written; a nil field was left out.
type file struct {
	Paths       []string     `toml:"paths"`
	Targets     []targetFile `toml:"targets"`
	CloseOnOpen bool         `toml:"close_on_open"`
	Sort        string       `toml:"sort"`
	CacheMaxAge string       `toml:"cache_max_age"`
	Columns     []string     `toml:"columns"`
	RowStyle    string       `toml:"row_style"`
	Selection   string       `toml:"selection"`
}

type targetFile struct {
	Name    string `toml:"name"`
	Key     string `toml:"key,omitempty"` // omitempty for WriteTarget, which leaves out a key not given
	Command string `toml:"command"`
	Close   *bool  `toml:"close,omitempty"` // nil when left out
	InPlace bool   `toml:"in_place,omitempty"`
}

// checked is file with each value checking itself while it is decoded, so
// that a bad value is reported on its own line. It is decoded only after file
// succeeds: the library reports a wrong type without its line when the target
// checks itself.
type checked struct {
	Paths       []folder        `toml:"paths"`
	Targets     []checkedTarget `toml:"targets"`
	CloseOnOpen bool            `toml:"close_on_open"`
	Sort        sortName        `toml:"sort"`
	CacheMaxAge maxAge          `toml:"cache_max_age"`
	Columns     []column        `toml:"columns"`
	RowStyle    rowStyle        `toml:"row_style"`
	Selection   selection       `toml:"selection"`
}

type checkedTarget struct {
	Name    targetName `toml:"name"`
	Key     key        `toml:"key"`
	Command command    `toml:"command"`
	Close   bool       `toml:"close"`
	InPlace bool       `toml:"in_place"`
}

// Load reads the config file at path, or at the standard location when path
// is "". A missing file at the standard location means the defaults; a
// missing file named on the command line is an error. A leading ~ in a
// setting stands for home.
func Load(path, home string) (Settings, error) {
	standard := path == ""
	path = Path(path, home)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !standard {
		return Settings{}, fmt.Errorf("config file %s does not exist", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, err
	}
	return parse(path, home, b)
}

// parse is Load for b, the text of the config file at path.
func parse(path, home string, b []byte) (Settings, error) {
	var f file
	for _, v := range []any{&f, &checked{}} {
		if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(v); err != nil {
			return Settings{}, describe(path, err)
		}
	}
	if f.Paths == nil {
		f.Paths = []string{"~/code"}
	}
	if err := checkTargets(path, b, f.Targets); err != nil {
		return Settings{}, err
	}
	if err := checkColumns(path, b, f.Columns); err != nil {
		return Settings{}, err
	}
	if f.Columns == nil {
		f.Columns = []string{"name", "folder", "branch", "last_opened"}
	}
	if f.RowStyle == "" {
		f.RowStyle = "name-folder"
	}
	if f.Selection == "" {
		f.Selection = "bar"
	}
	s := Settings{File: path, CloseOnOpen: f.CloseOnOpen, SortByName: f.Sort == "name", Columns: f.Columns, RowStyle: f.RowStyle, Selection: f.Selection}
	if f.CacheMaxAge != "" {
		s.CacheMaxAge, _ = parseMaxAge(f.CacheMaxAge) // cannot fail: the value was checked while decoding
	}
	for _, t := range f.Targets {
		args, _ := split(t.Command) // cannot fail: the command was checked while decoding
		s.Targets = append(s.Targets, target.Target{Name: t.Name, Key: t.Key, Args: args, Command: t.Command, Close: t.Close, InPlace: t.InPlace})
	}
	for _, p := range f.Paths {
		s.Paths = append(s.Paths, expand(p, home))
	}
	return s, nil
}

// Path returns the config file Load reads: path, as given to --config, or
// the standard location when path is "".
func Path(path, home string) string {
	if path != "" {
		return path
	}
	return filepath.Join(xdg.Dir("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "loper", "config.toml")
}

// expand replaces a leading ~ with the home folder.
func expand(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}
