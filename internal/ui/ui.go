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
	Home    string   // shown as ~ in folders
	Paths   []string // what to scan for repos
	Scan    func(paths []string) (repos, missing []string)
	Branch  func(repo string) string
	Open    func(t target.Target, path string) error
	Targets []target.Target // the first is what enter opens the selected repo in
	Now     func() time.Time
}

// Model is the program state.
type Model struct {
	deps     Deps
	repos    []repo
	missing  []string // configured paths that do not exist
	selected int
	offset   int // the first repo shown, so the selected one stays in view
	width    int
	height   int
	notice   string // shown in place of the footer keys until the next key press
}

// repo is one row of the list.
type repo struct {
	path   string
	branch string
}

type scannedMsg struct {
	repos   []repo
	missing []string
}

// openedMsg reports that a repo opened in a target.
type openedMsg struct {
	repo, target string
}

func New(deps Deps) Model {
	return Model{deps: deps}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		paths, missing := m.deps.Scan(m.deps.Paths)
		var repos []repo
		for _, path := range paths {
			repos = append(repos, repo{path: path, branch: m.deps.Branch(path)})
		}
		return scannedMsg{repos: repos, missing: missing}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scannedMsg:
		m.repos, m.missing = msg.repos, msg.missing
	case openedMsg:
		m.notice = "Opened " + msg.repo + " in " + msg.target
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		m.notice = ""
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.selected = min(m.selected+1, max(len(m.repos)-1, 0))
		case "k", "up":
			m.selected = max(m.selected-1, 0)
		case "g":
			m.selected = 0
		case "G":
			m.selected = max(len(m.repos)-1, 0)
		case "ctrl+d":
			m.selected = min(m.selected+max(m.listHeight()/2, 1), max(len(m.repos)-1, 0))
		case "ctrl+u":
			m.selected = max(m.selected-max(m.listHeight()/2, 1), 0)
		case "enter":
			return m, m.open(m.deps.Targets[0])
		default:
			for _, t := range m.deps.Targets {
				if t.Key != "" && msg.Text == t.Key {
					return m, m.open(t)
				}
			}
		}
	}
	m.offset = min(m.offset, m.selected, max(len(m.repos)-m.listHeight(), 0))
	m.offset = max(m.offset, m.selected-m.listHeight()+1)
	return m, nil
}

func (m Model) View() tea.View {
	nameWidth, folderWidth := 0, 0
	for _, r := range m.repos {
		nameWidth = max(nameWidth, ansi.StringWidth(filepath.Base(r.path)))
		folderWidth = max(folderWidth, ansi.StringWidth(m.shorten(filepath.Dir(r.path))))
	}
	rule := " " + strings.Repeat("─", max(m.width-2, 0)) + " "
	header := fmt.Sprintf(" repos  %d", len(m.repos))
	for _, path := range m.missing {
		header += "  " + m.shorten(path) + " not found"
	}
	lines := []string{header, rule}
	for i := m.offset; i < m.offset+m.listHeight(); i++ {
		if i >= len(m.repos) {
			lines = append(lines, "")
			continue
		}
		r := m.repos[i]
		marker := "   "
		if i == m.selected {
			marker = " > "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s  %s", marker, pad(filepath.Base(r.path), nameWidth), pad(m.shorten(filepath.Dir(r.path)), folderWidth), r.branch))
	}
	keys := " enter open"
	for _, t := range m.deps.Targets {
		if t.Key != "" {
			keys += "  " + t.Key + " " + t.Name
		}
	}
	keys += "  q quit"
	if m.notice != "" {
		keys = " " + m.notice
	}
	lines = append(lines, rule, keys)
	lines = lines[:min(len(lines), m.height)] // never taller than the terminal
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// open opens the selected repo in t in the background.
func (m Model) open(t target.Target) tea.Cmd {
	if len(m.repos) == 0 {
		return nil
	}
	path := m.repos[m.selected].path
	return func() tea.Msg {
		if err := m.deps.Open(t, path); err != nil {
			return nil // a failed open stays silent for now
		}
		return openedMsg{repo: filepath.Base(path), target: t.Name}
	}
}

// listHeight is the number of screen lines left for the list between the
// header, the footer and their rules.
func (m Model) listHeight() int {
	return max(m.height-4, 0)
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
