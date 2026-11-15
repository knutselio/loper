package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// firstRun is the config file the picker creates when there is none.
const firstRun = "[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}\"\nin_place = true\n"

// Open is Load for the picker. When path is "" and the standard location
// has no file, it first creates one holding firstRun, its folder included,
// private to the user, and Created says so. When that fails, the settings
// are firstRun's for this run and Notice says why. An existing file is never
// written.
func Open(path, home string) (Settings, error) {
	var created bool
	if path == "" {
		var err error
		if created, err = create(Path("", home)); err != nil {
			s := FirstRun(home)
			s.Notice = "The config file could not be written: " + err.Error()
			return s, nil
		}
	}
	s, err := Load(path, home)
	s.Created = created
	return s, err
}

// FirstRun returns the settings of the config file Open creates.
func FirstRun(home string) Settings {
	s, _ := parse(Path("", home), home, []byte(firstRun)) // cannot fail: the text is fixed
	return s
}

// step runs before each step of create, with its name, so that a test can
// stop the process there or make the step fail with the error it returns. In
// the tool it does nothing.
var step = func(string) error { return nil }

// create writes firstRun to path unless something is there already. The text
// goes to a temporary file in the same folder, which is then linked to path.
// A link fails when anything is at path, so the file appears whole or not at
// all, and whatever appeared there meanwhile is neither replaced nor written
// through. A link to nothing at path counts as a failure. It reports whether
// it made the file; the error is the reason only, without the path.
func create(path string) (bool, error) {
	dir, temp := filepath.Dir(path), filepath.Base(path)+".first-run."
	err := step("make the folder")
	if err == nil {
		err = os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return false, reason(err)
	}
	removeTemps(dir, temp)
	if err := step("look"); err != nil {
		return false, err
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		return false, there(path, err)
	}
	var f *os.File
	if err = step("create the temporary file"); err == nil {
		f, err = os.CreateTemp(dir, temp+"*") // private to the user
	}
	if err != nil {
		return false, reason(err)
	}
	if err = step("write"); err == nil {
		_, err = f.WriteString(firstRun)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = step("link")
	}
	if err == nil {
		err = os.Link(f.Name(), path)
	}
	if step("remove the temporary file") == nil {
		os.Remove(f.Name()) // one left behind goes on the next start
	}
	if errors.Is(err, fs.ErrExist) {
		return false, there(path, nil)
	}
	return err == nil, reason(err)
}

// there is create's result when something is at path, or err when looking
// failed: nothing to do, except for a link to nothing, which is not written
// through.
func there(path string, err error) error {
	if err != nil {
		return reason(err)
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		// The link can be gone by now; then dest is "", the notice names no
		// file, and the next start creates the config file.
		dest, _ := os.Readlink(path)
		return fmt.Errorf("it is a link to %s, which does not exist", dest)
	}
	return nil
}

// removeTemps removes the temporary files in dir left by first runs that
// were stopped before they removed their own: regular files whose names
// start with prefix and that hold the start of firstRun, or all of it. Any
// other file, folder or link is the user's and stays. Taking one from a
// first run still going only makes that run fail.
func removeTemps(dir, prefix string) {
	entries, _ := os.ReadDir(dir) // a file that stays is harmless
	for _, e := range entries {
		name := filepath.Join(dir, e.Name())
		if !strings.HasPrefix(e.Name(), prefix) || !e.Type().IsRegular() {
			continue
		}
		if b, err := os.ReadFile(name); err == nil && strings.HasPrefix(firstRun, string(b)) {
			os.Remove(name)
		}
	}
}

// reason returns err without the operation and paths a *fs.PathError or an
// *os.LinkError adds.
func reason(err error) error {
	var l *os.LinkError
	if errors.As(err, &l) {
		return l.Err
	}
	var p *fs.PathError
	if errors.As(err, &p) {
		return p.Err
	}
	return err
}
