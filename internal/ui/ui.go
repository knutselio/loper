// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/target"
)

// Deps is everything the UI needs from the outside world.
type Deps struct {
	Home   string   // shown as ~ in folders
	Paths  []string // what to scan for repos
	Scan   func(paths []string) []string
	Branch func(repo string) string
	Open   func(t target.Target, path string) error
	Target target.Target // what enter opens the selected repo in
	Now    func() time.Time
}

// Model is the program state.
type Model struct {
	deps     Deps
	repos    []repo
	selected int
}

// repo is one row of the list.
type repo struct {
	path   string
	branch string
}

type scannedMsg []repo

func New(deps Deps) Model {
	return Model{deps: deps}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		var repos []repo
		for _, path := range m.deps.Scan(m.deps.Paths) {
			repos = append(repos, repo{path: path, branch: m.deps.Branch(path)})
		}
		return scannedMsg(repos)
	}
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
			path := m.repos[m.selected].path
			return m, func() tea.Msg {
				m.deps.Open(m.deps.Target, path)
				return nil
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	nameWidth, folderWidth := 0, 0
	for _, r := range m.repos {
		nameWidth = max(nameWidth, ansi.StringWidth(filepath.Base(r.path)))
		folderWidth = max(folderWidth, ansi.StringWidth(m.shorten(filepath.Dir(r.path))))
	}
	var b strings.Builder
	for i, r := range m.repos {
		marker := "  "
		if i == m.selected {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%s  %s  %s\n", marker, pad(filepath.Base(r.path), nameWidth), pad(m.shorten(filepath.Dir(r.path)), folderWidth), r.branch)
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// pad fills s with spaces to width display cells.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
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
