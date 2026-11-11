// Package xdg finds the base folders of the XDG Base Directory spec.
package xdg

import (
	"os"
	"path/filepath"
)

// Dir returns the folder in the environment variable name, or fallback when
// it is unset, empty or relative, which the spec says to ignore.
func Dir(name, fallback string) string {
	if dir := os.Getenv(name); filepath.IsAbs(dir) {
		return dir
	}
	return fallback
}
