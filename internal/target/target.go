// Package target runs the command that opens a repo somewhere.
package target

import (
	"os/exec"
	"strings"
)

// Target is a place a repo can be opened in. Command holds a {path}
// placeholder for the repo's absolute path.
type Target struct {
	Name    string
	Command string
}

// Run opens the repo at path in t, with the repo as the working folder.
func Run(t Target, path string) error {
	words := strings.Fields(t.Command)
	for i, w := range words {
		words[i] = strings.ReplaceAll(w, "{path}", path)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = path
	return cmd.Run()
}
