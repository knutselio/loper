// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/target"
)

// Deps is everything the UI needs from the outside world.
type Deps struct {
	Home   string   // shown as ~ in folders
	Paths  []string // what to scan for repos
	Scan   func(paths []string) []string
	Open   func(t target.Target, path string) error
	Target target.Target // what enter opens the selected repo in
	Now    func() time.Time
}

// Model is the program state.
type Model struct {
	deps     Deps
	repos    []string
	selected int
}

type scannedMsg []string

func New(deps Deps) Model {
	return Model{deps: deps}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return scannedMsg(m.deps.Scan(m.deps.Paths)) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scannedMsg:
		m.repos = msg
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.selected = min(m.selected+1, max(len(m.repos)-1, 0))
		case "k", "up":
			m.selected = max(m.selected-1, 0)
		case "enter":
			if len(m.repos) == 0 {
				return m, nil
			}
			repo := m.repos[m.selected]
			return m, func() tea.Msg {
				m.deps.Open(m.deps.Target, repo)
				return nil
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	width := 0
	for _, r := range m.repos {
		width = max(width, len(filepath.Base(r)))
	}
	var b strings.Builder
	for i, r := range m.repos {
		marker := "  "
		if i == m.selected {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%-*s  %s\n", marker, width, filepath.Base(r), m.shorten(filepath.Dir(r)))
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// shorten replaces the home folder at the start of path with ~.
func (m Model) shorten(path string) string {
	if path == m.deps.Home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, m.deps.Home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
