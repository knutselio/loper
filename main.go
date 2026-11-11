// Command loper is a keyboard-driven terminal picker for git repos.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "loper:", err)
		os.Exit(1)
	}
	m := ui.New(ui.Deps{
		Home:   home,
		Paths:  []string{filepath.Join(home, "code")},
		Scan:   scanner.Scan,
		Open:   target.Run,
		Target: target.Target{Name: "Zed", Command: "zed {path}"},
		Now:    time.Now,
	})
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "loper:", err)
		os.Exit(1)
	}
}
