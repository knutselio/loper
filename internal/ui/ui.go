// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/matcher"
	"github.com/knutselio/loper/internal/reltime"
	"github.com/knutselio/loper/internal/target"
)

// Deps is everything the UI needs from the outside world.
type Deps struct {
	Home       string                        // shown as ~ in folders
	ConfigFile string                        // named when no repos are found
	Paths      []string                      // what to scan for repos
	Scan       func(paths []string) []string // the repos under paths
	Missing    func(paths []string) []string // the paths that do not exist
	Branch     func(repo string) string
	Exists     func(repo string) bool // whether a cached repo is still a repo
	Open       func(t target.Target, path string) error
	Targets    []target.Target // in the order the menu lists them
	History    history.Store
	Cache      cache.Store
	Now        func() time.Time
	// Tick is tea.Tick, which drives the refresh spinner. Tests deliver its
	// ticks themselves, so the spinner needs no real time.
	Tick func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd
	// CloseOnOpen quits the program after a successful open.
	CloseOnOpen bool
	// SortByName starts the list sorted by name rather than by last opened.
	SortByName bool
	// CacheMaxAge is how old the cache may be before a launch scans again
	// rather than re-reading the cached repos; 0 means every launch scans.
	CacheMaxAge time.Duration
	// Color highlights matched letters; it is off when NO_COLOR is set.
	Color bool
}

// Model is the program state.
type Model struct {
	deps     Deps
	all      []repo               // every repo, in the current sort
	repos    []repo               // the rows of the list: all, or the matches while searching
	missing  []string             // configured paths that do not exist
	history  map[string]time.Time // when each repo was last opened, by path
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
	byName   bool         // sorted by name rather than by last opened
	// refreshing is true from launch until the launch refresh has finished,
	// and again while a refresh started with r runs. Only one refresh runs at
	// a time, as each one rewrites the cache file.
	refreshing bool
	loaded     bool // the cache has been read
	// follow is set by the first key pressed with a list on screen. From then
	// on a change to the list keeps the selection on the selected repo;
	// before it the top row stays selected, so the list opens on the repo
	// opened most recently however the cache, the history and the refresh
	// arrive.
	follow bool
	frame  int // the spinner's frame
	// ticking is true while a spinner tick is on its way, so a refresh
	// started before that tick arrives never starts a second chain of ticks.
	ticking bool
	// searching is true from / until esc. While it is, the list shows only
	// the repos matching query, best match first, and hits holds where each
	// row matched.
	searching bool
	query     string
	hits      []matcher.Match
	// before is the repo selected when the search started, selected again
	// when it ends. If it disappears meanwhile it becomes the nearest repo.
	before string
}

// repo is one row of the list.
type repo struct {
	path   string
	branch string
}

// cachedMsg carries the repo list as cached by the last refresh, with when
// and from which paths it was scanned.
type cachedMsg struct {
	repos     []repo
	scannedAt time.Time
	paths     []string
}

// scannedMsg carries the repo list as the refresh found it.
type scannedMsg struct {
	repos   []repo
	missing []string
}

// historyMsg carries the history as loaded at launch.
type historyMsg map[string]time.Time

// openedMsg reports that a repo opened in a target at a time, which is
// already recorded in the history unless saving it failed.
type openedMsg struct {
	path, target string
	at           time.Time
	saveErr      error
}

// failedMsg reports that a repo did not open in a target.
type failedMsg struct {
	repo, target string
	target.Failure
}

// tickMsg moves the spinner on a frame.
type tickMsg struct{}

// spinner is the frames of the refresh spinner.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// matchOn and matchOff start and end the highlight of a matched letter.
var (
	matchOn  = ansi.Style{}.Underline(true).String()
	matchOff = ansi.Style{}.NoUnderline().String()
)

func New(deps Deps) Model {
	return Model{deps: deps, byName: deps.SortByName, refreshing: true, ticking: true}
}

// Init reads the cache and the history. The refresh starts once the cached
// list has arrived, so the list is on screen before the refresh's result.
func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg {
		list := m.deps.Cache.Load()
		c := cachedMsg{scannedAt: list.ScannedAt, paths: list.Paths}
		for _, r := range list.Repos {
			c.repos = append(c.repos, repo{path: r.Path, branch: r.Branch})
		}
		return c
	}, func() tea.Msg { return historyMsg(m.deps.History.Load()) }, m.tick())
}

// tick moves the spinner on after a while.
func (m Model) tick() tea.Cmd {
	return m.deps.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// refresh is the full refresh: it scans the configured paths, reads each
// repo's branch and rewrites the cache.
func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		missing := m.deps.Missing(m.deps.Paths)
		return scannedMsg{repos: m.save(m.deps.Scan(m.deps.Paths), m.deps.Now()), missing: missing}
	}
}

// reread is the light refresh: it drops the cached repos that are gone and
// reads the branch of the rest, without scanning; it checks only that the
// configured paths exist. The cache keeps the time of its scan, so it still
// expires.
func (m Model) reread(c cachedMsg) tea.Cmd {
	return func() tea.Msg {
		var paths []string
		for _, r := range c.repos {
			if m.deps.Exists(r.path) {
				paths = append(paths, r.path)
			}
		}
		return scannedMsg{repos: m.save(paths, c.scannedAt), missing: m.deps.Missing(m.deps.Paths)}
	}
}

// save reads the branch of each repo in paths and rewrites the cache with
// them, as scanned at scannedAt from the configured paths.
func (m Model) save(paths []string, scannedAt time.Time) []repo {
	list := cache.List{ScannedAt: scannedAt, Paths: m.deps.Paths}
	var repos []repo
	for _, path := range paths {
		r := repo{path: path, branch: m.deps.Branch(path)}
		repos = append(repos, r)
		list.Repos = append(list.Repos, cache.Repo{Path: r.path, Branch: r.branch})
	}
	// A cache that cannot be saved costs only the next launch its head
	// start, as that launch scans again; the list shown is still right.
	_ = m.deps.Cache.Save(list)
	return repos
}

// expired reports whether the cached list must be scanned again rather than
// re-read: with no max age, when it is older than that, when it was scanned
// from other paths (the same ones in another order count as other paths),
// or when its scan time is later than now, as after the clock was turned
// back, since such a cache would otherwise never age.
func (m Model) expired(c cachedMsg) bool {
	age := m.deps.Now().Sub(c.scannedAt)
	return m.deps.CacheMaxAge == 0 || age > m.deps.CacheMaxAge || age < 0 ||
		!slices.Equal(c.paths, m.deps.Paths)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case cachedMsg:
		m.loaded = true
		m = m.relist(msg.repos)
		if m.expired(msg) {
			cmd = m.refresh()
		} else {
			cmd = m.reread(msg)
		}
	case scannedMsg:
		m.refreshing = false
		m.missing = msg.missing
		m = m.relist(msg.repos)
	case historyMsg:
		// An open recorded before the history arrived is newer than the file
		// that was read, so it stays.
		h := map[string]time.Time{}
		maps.Copy(h, msg)
		maps.Copy(h, m.history)
		m.history = h
		m = m.relist(m.all)
	case openedMsg:
		m.opening = false
		h := map[string]time.Time{} // a new map: an older model may still hold the old one
		maps.Copy(h, m.history)
		h[msg.path] = msg.at
		m.history = h
		m = m.relist(m.all)
		m.notice = "Opened " + filepath.Base(msg.path) + " in " + msg.target
		if msg.saveErr != nil { // the footer has one line: room to name the problem, not its cause
			m.notice += ", but the history could not be saved"
		}
		if m.deps.CloseOnOpen && m.failure == nil { // never quit while an error is showing
			return m, tea.Quit
		}
	case failedMsg:
		m.opening = false
		m.menu, m.failure = false, newErrorDialog(msg) // the dialog replaces a menu opened while the target ran
	case tickMsg:
		m.ticking = m.refreshing // the spinner stops with the refresh
		if m.refreshing {
			m.frame = (m.frame + 1) % len(spinner)
			cmd = m.tick()
		}
	case tea.PasteMsg:
		if m.searching && !m.menu && m.failure == nil { // as typed text, which only a search takes
			m = m.search(m.query + clean(msg.Content))
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu = m.menu && m.menuRows() > 0
		m = m.scrollMenu()
	case tea.KeyPressMsg:
		m.notice = ""
		m.follow = m.follow || len(m.repos) > 0
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
		if m.searching {
			var handled bool
			if m, handled = m.searchKey(msg); handled {
				break
			}
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
		case "/":
			m.searching, m.before = true, ""
			if len(m.repos) > 0 {
				m.before = m.repos[m.selected].path
			}
			m.repos, m.hits = m.filter() // an empty query matches every repo, in the order shown
		case "s":
			m.byName = !m.byName
			m = m.relist(m.all)
		case "r":
			if !m.refreshing {
				m.refreshing = true
				cmd = m.refresh()
				if !m.ticking {
					m.ticking = true
					cmd = tea.Batch(cmd, m.tick())
				}
			}
		default:
			if t, ok := m.keyed(msg); ok {
				return m.open(t)
			}
		}
	}
	m.offset = min(m.offset, m.selected, max(len(m.repos)-m.listHeight(), 0))
	m.offset = max(m.offset, m.selected-m.listHeight()+1)
	return m, cmd
}

// searchKey handles a key press while searching and reports whether it did.
// Printable characters go to the query, so target keys and list keys are
// text. Keys it does not handle, such as the arrows and enter, act as in the
// list.
func (m Model) searchKey(msg tea.KeyPressMsg) (Model, bool) {
	if msg.Text != "" {
		return m.search(m.query + msg.Text), true
	}
	switch msg.String() {
	case "backspace":
		if q := []rune(m.query); len(q) > 0 {
			m = m.search(string(q[:len(q)-1]))
		}
	case "ctrl+n":
		m.selected = min(m.selected+1, max(len(m.repos)-1, 0))
	case "ctrl+p":
		m.selected = max(m.selected-1, 0)
	case "esc":
		m.searching, m.query = false, ""
		m.repos, m.hits = m.filter()
		m.selected = max(slices.IndexFunc(m.repos, func(r repo) bool { return r.path == m.before }), 0)
	default:
		return m, false
	}
	return m, true
}

// search shows the repos matching query, with the best match selected.
func (m Model) search(query string) Model {
	m.query = query
	m.repos, m.hits = m.filter()
	m.selected = 0
	return m
}

// filter returns the rows of the list: every repo, or while searching the
// repos matching the query, best match first, with where each matched.
func (m Model) filter() ([]repo, []matcher.Match) {
	if !m.searching {
		return m.all, nil
	}
	rows := make([]matcher.Row, len(m.all))
	for i, r := range m.all {
		rows[i] = matcher.Row{Name: filepath.Base(r.path), Folder: m.shorten(filepath.Dir(r.path)), Branch: r.branch}
	}
	hits := matcher.Find(m.query, rows)
	repos := make([]repo, len(hits))
	for i, h := range hits {
		repos[i] = m.all[h.Row]
	}
	return repos, hits
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
	nameWidth, folderWidth, branchWidth := 0, 0, 0
	for _, r := range m.all { // all, so the columns stay put while searching
		nameWidth = max(nameWidth, ansi.StringWidth(filepath.Base(r.path)))
		folderWidth = max(folderWidth, ansi.StringWidth(m.shorten(filepath.Dir(r.path))))
		branchWidth = max(branchWidth, ansi.StringWidth(r.branch))
	}
	rule := " " + strings.Repeat("─", max(m.width-2, 0)) + " "
	header := fmt.Sprintf(" repos  %d", len(m.repos))
	for _, path := range m.missing {
		header += "  " + m.shorten(path) + " not found"
	}
	right := "  sort: last opened "
	if m.byName {
		right = "  sort: name "
	}
	if m.searching { // the search line replaces the count, the warnings and the sort
		header, right = " /"+m.query, ""
	}
	if m.refreshing && len(m.all) > 0 { // with no list yet, the spinner is in the list
		right += "  " + spinner[m.frame] + " refreshing "
	}
	// The sort and the spinner keep their place on the right; warnings that
	// do not fit are cut.
	room := max(m.width-ansi.StringWidth(right), 0)
	header = ansi.Truncate(pad(ansi.Truncate(header, room, "…"), room)+right, m.width, "")
	lines := []string{header, rule}
	var empty []string // what an empty list says instead
	switch {
	case len(m.repos) > 0:
	case m.searching && len(m.all) > 0:
		empty = []string{"No matches"}
	case !m.refreshing:
		paths := make([]string, len(m.deps.Paths))
		for i, p := range m.deps.Paths {
			paths[i] = m.shorten(p)
		}
		empty = []string{"No repos found in " + strings.Join(paths, ", "), "Change paths in " + m.shorten(m.deps.ConfigFile)}
	case m.loaded:
		empty = []string{spinner[m.frame] + " scanning…"}
	}
	now := m.deps.Now()
	for i := m.offset; i < m.offset+m.listHeight(); i++ {
		if i >= len(m.repos) {
			line := ""
			if i < len(empty) {
				line = ansi.Truncate("   "+empty[i], m.width, "…")
			}
			lines = append(lines, line)
			continue
		}
		r := m.repos[i]
		marker := "   "
		if i == m.selected {
			marker = " > "
		}
		lastOpened := "-"
		if at, ok := m.history[r.path]; ok {
			lastOpened = reltime.Format(at, now)
		}
		name, folder, branch := filepath.Base(r.path), m.shorten(filepath.Dir(r.path)), r.branch
		if m.hits != nil {
			h := m.hits[i]
			name, folder, branch = m.highlight(name, h.Name), m.highlight(folder, h.Folder), m.highlight(branch, h.Branch)
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s  %s  %s", marker, pad(name, nameWidth), pad(folder, folderWidth), pad(branch, branchWidth), lastOpened))
	}
	keys := " enter open"
	for _, t := range m.deps.Targets {
		if t.Key != "" {
			keys += "  " + t.Key + " " + t.Name
		}
	}
	keys += "  / search  s sort  r refresh  q quit"
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

// highlight marks the characters of s at the positions pos, which are in
// ascending order, unless colour is off.
func (m Model) highlight(s string, pos []int) string {
	if !m.deps.Color || len(pos) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if len(pos) > 0 && pos[0] == i {
			b.WriteString(matchOn + string(r) + matchOff)
			pos = pos[1:]
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
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
		// Recorded before the open is reported, so it is on disk even when the
		// tool quits on the report. Round(0) keeps only the wall clock, as the
		// file does: a monotonic reading can stop while the computer sleeps.
		at := m.deps.Now().Round(0)
		return openedMsg{path: path, target: t.Name, at: at, saveErr: m.deps.History.Record(path, at)}
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

// relist replaces the list with repos, in the current sort. Until the
// selection follows the selected repo it keeps its row. After that it stays
// on the selected repo; if that repo is gone, it moves to the nearest row of
// the old list that is still there. The repo to select when a search ends
// moves the same way. The target menu stays open only while its repo is
// still selected.
func (m Model) relist(repos []repo) Model {
	old, at, oldAll := m.repos, m.selected, m.all
	m.all = m.sorted(repos)
	m.repos, m.hits = m.filter()
	m.selected = min(m.selected, max(len(m.repos)-1, 0))
	if m.follow {
		if j := nearest(old, at, m.repos); j >= 0 {
			m.selected = j
		}
	}
	if m.searching {
		if i := slices.IndexFunc(oldAll, func(r repo) bool { return r.path == m.before }); i >= 0 {
			if j := nearest(oldAll, i, m.all); j >= 0 {
				m.before = m.all[j].path
			}
		}
	}
	m.menu = m.menu && len(m.repos) > 0 && m.repos[m.selected].path == old[at].path
	return m
}

// nearest returns the index in repos of old[at] or, if repos lacks it, of
// the nearest repo of old that repos has, below before above; -1 if there
// is none.
func nearest(old []repo, at int, repos []repo) int {
	for d := range len(old) {
		for _, i := range []int{at + d, at - d} {
			if i < 0 || i >= len(old) {
				continue
			}
			if j := slices.IndexFunc(repos, func(r repo) bool { return r.path == old[i].path }); j >= 0 {
				return j
			}
		}
	}
	return -1
}

// sorted returns repos in a new slice, as an older model may still hold
// the old one, in the current sort: by name, or most recently opened first
// and then the repos never opened by name.
func (m Model) sorted(repos []repo) []repo {
	repos = slices.Clone(repos)
	if m.byName {
		slices.SortFunc(repos, m.compareNames)
		return repos
	}
	slices.SortFunc(repos, func(a, b repo) int {
		at, aOpened := m.history[a.path]
		bt, bOpened := m.history[b.path]
		switch {
		case aOpened && bOpened:
			if c := bt.Compare(at); c != 0 {
				return c
			}
		case aOpened:
			return -1
		case bOpened:
			return 1
		}
		return m.compareNames(a, b)
	})
	return repos
}

// compareNames orders two repos by name, then by folder as shown. Case is
// ignored, so Backend and backend sit together; the full path breaks any tie,
// so the order never depends on the order the repos arrived in.
func (m Model) compareNames(a, b repo) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(filepath.Base(a.path)), strings.ToLower(filepath.Base(b.path))),
		cmp.Compare(strings.ToLower(m.shorten(filepath.Dir(a.path))), strings.ToLower(m.shorten(filepath.Dir(b.path)))),
		cmp.Compare(a.path, b.path),
	)
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
