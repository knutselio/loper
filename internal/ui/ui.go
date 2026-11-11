// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

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
	Targets []target.Target // in the order the menu lists them
	Now     func() time.Time
	// CloseOnOpen quits the program after a successful open.
	CloseOnOpen bool
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
	notice   string       // shown in place of the footer keys until the next key press
	menu     bool         // the target menu is open
	choice   int          // the target highlighted in the menu
	menuTop  int          // the first target shown, so the highlighted one stays in view
	failure  *errorDialog // the error dialog for a failed open, or nil
	opening  bool         // an open is running, so targets do nothing until it finishes
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

// failedMsg reports that a repo did not open in a target.
type failedMsg struct {
	repo, target string
	target.Failure
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
		m.opening = false
		m.notice = "Opened " + msg.repo + " in " + msg.target
		if m.deps.CloseOnOpen && m.failure == nil { // never quit while an error is showing
			return m, tea.Quit
		}
	case failedMsg:
		m.opening = false
		m.menu, m.failure = false, newErrorDialog(msg) // the dialog replaces a menu opened while the target ran
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu = m.menu && m.menuRows() > 0
		m = m.scrollMenu()
	case tea.KeyPressMsg:
		m.notice = ""
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.failure != nil {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc":
				m.failure = nil
			}
			return m, nil
		}
		if m.menu {
			m, cmd := m.menuKey(msg)
			return m.scrollMenu(), cmd
		}
		switch msg.String() {
		case "q":
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
			m.menu, m.choice, m.menuTop = len(m.repos) > 0 && m.menuRows() > 0, 0, 0
		default:
			if t, ok := m.keyed(msg); ok {
				return m.open(t)
			}
		}
	}
	m.offset = min(m.offset, m.selected, max(len(m.repos)-m.listHeight(), 0))
	m.offset = max(m.offset, m.selected-m.listHeight()+1)
	return m, nil
}

// menuKey handles a key press while the target menu is open.
func (m Model) menuKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "j", "down":
		m.choice = min(m.choice+1, len(m.deps.Targets)-1)
	case "k", "up":
		m.choice = max(m.choice-1, 0)
	case "enter":
		return m.open(m.deps.Targets[m.choice])
	case "esc":
		m.menu = false
	default:
		if t, ok := m.keyed(msg); ok {
			return m.open(t)
		}
	}
	return m, nil
}

// menuRows is the number of targets the menu has room to show between its
// borders, or 0 when the list is too short for the menu.
func (m Model) menuRows() int {
	return max(min(len(m.deps.Targets), m.listHeight()-2), 0)
}

// scrollMenu moves the menu's first shown target so the highlighted one is
// in view.
func (m Model) scrollMenu() Model {
	rows := max(m.menuRows(), 1)
	m.menuTop = min(m.menuTop, m.choice, len(m.deps.Targets)-rows)
	m.menuTop = max(m.menuTop, m.choice-rows+1)
	return m
}

// keyed returns the target whose key was pressed, if any.
func (m Model) keyed(msg tea.KeyPressMsg) (target.Target, bool) {
	for _, t := range m.deps.Targets {
		if t.Key != "" && msg.Text == t.Key {
			return t, true
		}
	}
	return target.Target{}, false
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
	switch {
	case m.failure != nil:
		lines = m.overlayFailure(lines)
	case m.menu:
		lines = m.overlayMenu(lines)
	}
	lines = append(lines, rule, keys)
	lines = lines[:min(len(lines), m.height)] // never taller than the terminal
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// open opens the selected repo in t in the background and closes the menu,
// unless another open is still running: then it does nothing, so that one
// open's result can never hide another's.
func (m Model) open(t target.Target) (Model, tea.Cmd) {
	if len(m.repos) == 0 || m.opening {
		return m, nil
	}
	m.menu, m.opening = false, true
	path := m.repos[m.selected].path
	return m, func() tea.Msg {
		if err := m.deps.Open(t, path); err != nil {
			f := &target.Failure{Output: err.Error()} // in case the error is not a Failure
			errors.As(err, &f)
			return failedMsg{repo: filepath.Base(path), target: t.Name, Failure: *f}
		}
		return openedMsg{repo: filepath.Base(path), target: t.Name}
	}
}

// overlayMenu draws the target menu centred over the list part of lines,
// which stays visible around it.
func (m Model) overlayMenu(lines []string) []string {
	keyWidth := 0
	for _, t := range m.deps.Targets {
		keyWidth = max(keyWidth, ansi.StringWidth(t.Key))
	}
	title := " Open " + filepath.Base(m.repos[m.selected].path) + " "
	rows := make([]string, len(m.deps.Targets))
	width := 0
	for i, t := range m.deps.Targets {
		marker := "  "
		if i == m.choice {
			marker = "> "
		}
		rows[i] = " " + marker + pad(t.Key, keyWidth) + "  " + t.Name + " "
		width = max(width, ansi.StringWidth(rows[i]))
	}
	return m.overlay(lines, m.box(title, rows[m.menuTop:m.menuTop+m.menuRows()], width))
}

// overlayFailure draws the error dialog centred over the list part of
// lines: the command line that ran, then what it printed and why it failed,
// cut to the space there is.
func (m Model) overlayFailure(lines []string) []string {
	space := m.listHeight() - 2 // the rows between the borders
	if space < 1 {
		return lines // no room: the dialog shows again once there is
	}
	f := m.failure
	return m.overlay(lines, m.box(f.title, f.rows[:min(len(f.rows), space)], f.width))
}

// errorDialog is the error dialog's content, worked out once when the
// failure arrives rather than on every render.
type errorDialog struct {
	title string
	rows  []string // the command line that ran, then its output and why it failed
	width int      // of the widest row
}

func newErrorDialog(msg failedMsg) *errorDialog {
	d := &errorDialog{title: " Could not open " + msg.repo + " in " + msg.target + " "}
	d.rows = []string{" $ " + clean(msg.Command) + " "}
	for _, l := range strings.Split(strings.TrimRight(msg.Output, "\n"), "\n") {
		d.rows = append(d.rows, " "+clean(l)+" ")
	}
	for _, r := range d.rows {
		d.width = max(d.width, ansi.StringWidth(r))
	}
	return d
}

// clean makes s safe to draw on one line: escape sequences and control
// characters, which would move the cursor or change the screen, are
// dropped, and a tab becomes spaces.
func clean(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// box draws rows in a rounded border with title in its top edge. width is
// what the rows want inside the border; the box grows to fit the title and
// shrinks to fit the screen, cutting rows that are too long.
func (m Model) box(title string, rows []string, width int) []string {
	width = max(width, ansi.StringWidth(title)+2) // a stroke of the border either side of the title
	width = max(min(width, m.width-2), 0)         // inside the two side borders
	head := ansi.Truncate("─"+title, width, "")
	box := []string{"╭" + head + strings.Repeat("─", width-ansi.StringWidth(head)) + "╮"}
	for _, r := range rows {
		box = append(box, "│"+pad(ansi.Truncate(r, width, "…"), width)+"│")
	}
	return append(box, "╰"+strings.Repeat("─", width)+"╯")
}

// overlay places box centred over the list part of lines, which stays
// visible around it. The box must fit in the list's height.
func (m Model) overlay(lines, box []string) []string {
	width := ansi.StringWidth(box[0]) - 2
	top := 2 + max(m.listHeight()-len(box), 0)/2
	left := max(m.width-width-2, 0) / 2
	for i, b := range box {
		line := lines[top+i]
		// A wide character cut by either edge of the box becomes a space, so
		// the rest of the row stays on its cells.
		before := pad(ansi.Truncate(line, left, ""), left)
		cut := left + width + 2
		after := ansi.TruncateLeft(line, cut, "")
		if ansi.StringWidth(after) > ansi.StringWidth(line)-cut {
			after = " " + ansi.TruncateLeft(after, 2, "")
		}
		lines[top+i] = ansi.Truncate(before+b+after, m.width, "")
	}
	return lines
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
