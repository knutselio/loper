// Package wholefile writes files that others read, so a reader never sees
// half of one.
package wholefile

import (
	"os"
	"path/filepath"
)

// Write replaces the file at path with b, through a temporary file renamed
// over it, creating its folder private to the user, as XDG asks of the
// cache and state folders. A file replaced keeps its mode; a new one is
// private to the user.
func Write(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		err = f.Chmod(info.Mode().Perm())
	}
	if err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name()) // the write already failed; a leftover temp file is harmless
	}
	return err
}
