// Package target runs the command that opens a repo somewhere.
package target

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Target is a place a repo can be opened in.
type Target struct {
	Name string
	Key  string   // the key that opens the selected repo here, or ""
	Args []string // the command split into words, with {path} and {name} placeholders
}

// Run opens the repo at path in t, with the repo as the working folder and
// the tool's own environment. {path} in an argument stands for path and
// {name} for the repo's name; no shell is involved.
func Run(t Target, path string) error {
	r := strings.NewReplacer("{path}", path, "{name}", filepath.Base(path))
	words := make([]string, len(t.Args))
	for i, a := range t.Args {
		words[i] = r.Replace(a)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = path
	return cmd.Run()
}
