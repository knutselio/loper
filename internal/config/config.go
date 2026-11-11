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

	toml "github.com/pelletier/go-toml/v2"
)

// Settings is the complete configuration, with every default applied.
type Settings struct {
	Paths []string // absolute folders to scan, or repos to list directly
}

// file is the config file as written; a nil field was left out.
type file struct {
	Paths []string `toml:"paths"`
}

// checked is file with each value checking itself while it is decoded, so
// that a bad value is reported on its own line. It is decoded only after file
// succeeds: the library reports a wrong type without its line when the target
// checks itself.
type checked struct {
	Paths []folder `toml:"paths"`
}

// folder checks a paths entry: an absolute path, or one that starts with ~.
type folder struct{}

func (*folder) UnmarshalText(b []byte) error {
	p := string(b)
	if p != "~" && !strings.HasPrefix(p, "~/") && !filepath.IsAbs(p) {
		return errors.New("not an absolute path")
	}
	return nil
}

// Load reads the config file at path, or at the standard location when path
// is "". A missing file at the standard location means the defaults; a
// missing file named on the command line is an error. A leading ~ in a
// setting stands for home.
func Load(path, home string) (Settings, error) {
	standard := path == ""
	if standard {
		path = defaultPath(home)
	}
	var f file
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !standard {
		return Settings{}, fmt.Errorf("config file %s does not exist", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, err
	}
	for _, v := range []any{&f, &checked{}} {
		if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(v); err != nil {
			return Settings{}, describe(path, err)
		}
	}
	if f.Paths == nil {
		f.Paths = []string{"~/code"}
	}
	var s Settings
	for _, p := range f.Paths {
		s.Paths = append(s.Paths, expand(p, home))
	}
	return s, nil
}

// defaultPath is where the config file lives unless --config says otherwise.
func defaultPath(home string) string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(dir) { // unset, empty or relative, which XDG says to ignore
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "loper", "config.toml")
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

// describe turns a TOML error into one that names the file and line.
func describe(path string, err error) error {
	var missing *toml.StrictMissingError
	if errors.As(err, &missing) {
		e := missing.Errors[0]
		line, _ := e.Position()
		return fmt.Errorf("%s:%d: unknown setting %q", path, line, strings.Join(e.Key(), "."))
	}
	var bad *toml.DecodeError
	if errors.As(err, &bad) {
		line, _ := bad.Position()
		if want, ok := wants[strings.Join(bad.Key(), ".")]; ok {
			return fmt.Errorf("%s:%d: invalid value for %q: want %s", path, line, strings.Join(bad.Key(), "."), want)
		}
		return fmt.Errorf("%s:%d: %s", path, line, strings.TrimPrefix(bad.Error(), "toml: "))
	}
	return err
}

// wants says, for each setting, what a valid value looks like.
var wants = map[string]string{
	"paths": `a list of folders, each absolute or starting with ~/, such as ["~/code"]`,
}
