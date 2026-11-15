// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/matcher"
	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
)

// Deps is everything the UI needs from the outside world.
type Deps struct {
	Home       string                              // shown as ~ in folders
	ConfigFile string                              // named when no repos are found, and where the dialog writes targets
	Paths      []string                            // what to scan for repos
	Scan       func(paths []string) []scanner.Repo // the repos under paths
	Missing    func(paths []string) []string       // the paths that do not exist
	Branch     func(repo string) string
	Exists     func(repo string) bool // whether a cached repo is still a repo
	Open       func(t target.Target, path string) error
	Targets    []target.Target // in the order the menu lists them
	History    history.Store
	Cache      cache.Store
	Now        func() time.Time
	// Tick is tea.Tick, which drives the refresh spinner and ends notices.
	// Tests deliver its ticks themselves, so neither needs real time.
	Tick func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd
	// Notice is shown at the top right at launch, as a notice, or is "".
	Notice string
	// Created says the config file was created at launch, which a notice
	// then says.
	Created bool
	// NoticeFor is how long a notice shows.
	NoticeFor time.Duration
	// SpinFor is how long the refresh spinner shows at least, so a refresh
	// that ends at once is still seen.
	SpinFor time.Duration
	// EnterPause is how long after the dialog's confirmation appears an
	// enter is ignored, so a quick second enter cannot write unseen.
	EnterPause time.Duration
	// CloseOnOpen quits the program after a successful open.
	CloseOnOpen bool
	// SortByName starts the list sorted by name, A to Z, rather than most
	// recently opened first.
	SortByName bool
	// CacheMaxAge is how old the cache may be before a launch scans again
	// rather than re-reading the cached repos; 0 means every launch scans.
	CacheMaxAge time.Duration
	// Styles turns the styles on: the bold selection marker, the bar of the
	// selection, dimmed headings, folders, rules, borders, counts and the
	// search hint, underlined matches, and bold keys in the footer, the top
	// row, the help and the menu. Without it the output has no escape codes
	// at all.
	Styles bool
	// Color, with Styles, draws the keys in the accent colour and problems
	// in red. It is off when NO_COLOR is set and non-empty, and then no
	// colour is drawn.
	Color bool
	// Columns are the columns to show, in order, out of name, folder, branch
	// and last_opened.
	Columns []string
	// RowStyle draws name and folder: "name-folder", "root-path" or "name".
	RowStyle string
	// CheckTarget returns, for a change to the targets of the config file,
	// the lines it replaces and the lines it writes, or why it is refused;
	// WriteTarget makes it and returns the targets the file then loads to
	// and the index of the target added or changed, or of where the removed
	// one was.
	CheckTarget func(file, home string, c target.Change) (before, after string, err error)
	WriteTarget func(file, home string, c target.Change) ([]target.Target, int, error)
	// CheckFile returns why the config file cannot take a target, if it
	// cannot, before the add dialog's fields open.
	CheckFile func(file, home string) error
}

// Model is the program state.
type Model struct {
	deps     Deps
	columns  []string             // the columns shown, in order
	all      []repo               // every repo, in the current sort
	repos    []repo               // the rows of the list: all, or the matches while searching
	missing  []string             // configured paths that do not exist
	history  map[string]time.Time // when each repo was last opened, by path
	selected int
	offset   int // the first repo shown, so the selected one stays in view
	width    int
	height   int
	notice   string       // shown at the top right for a moment, or until the next key press
	notices  int          // how many notices have shown, so the end of an earlier one's moment leaves a later one
	next     string       // a notice that shows once the moment of the one showing ends, or ""
	menu     bool         // the target menu is open
	choice   int          // the target highlighted in the menu
	menuTop  int          // the first target shown, so the highlighted one stays in view
	failure  *errorDialog // the error dialog for a failed open, or nil
	add      addDialog    // the dialog that adds, edits or removes a target
	confirms int          // how many confirmations have shown, so the end of an earlier one's moment leaves a later one
	help     bool         // the help overlay is open
	helpTop  int          // the first row of the help shown, so the help scrolls when it does not fit
	opening  bool         // an open is running, so targets do nothing until it finishes
	quitting bool         // q was pressed while an open was running, so the tool quits once it succeeds
	order    int          // the sort: byRecent, byName, byNameReversed or byOldest
	// helpLines and helpWidth are the help's rows and the width of the
	// widest, worked out once in New rather than on every render.
	helpLines []string
	helpWidth int
	// refreshing is true from launch until the launch refresh has finished,
	// and again while a refresh started with r runs. Only one refresh runs at
	// a time, as each one rewrites the cache file.
	refreshing bool
	// holding is true while the spinner of the latest refresh has not yet
	// shown for SpinFor, and holds counts the refreshes, so the end of an
	// earlier one's hold leaves a later one.
	holding bool
	holds   int
	loaded  bool // the cache has been read
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
	// handOver is the command to run in the tool's place once it has ended,
	// or nil.
	handOver *target.HandOver
}

// HandOver returns the command to run in the tool's place once the program
// has ended, if an in-place target was chosen.
func (m Model) HandOver() (target.HandOver, bool) {
	if m.handOver == nil {
		return target.HandOver{}, false
	}
	return *m.handOver, true
}

// openedMsg reports that a repo opened in a target at a time, which is
// already recorded in the history unless saving it failed.
type openedMsg struct {
	path, target string
	at           time.Time
	saveErr      error
	close        bool // the tool quits on it, as the target or close_on_open says
}

// handOverMsg asks for the tool to end and h to run in its place.
type handOverMsg struct{ h target.HandOver }

// tickMsg moves the spinner on a frame.
type tickMsg struct{}

// noticeEndMsg ends the moment of the notice it counts.
type noticeEndMsg int

// holdEndMsg ends the hold of the spinner of the refresh it counts.
type holdEndMsg int

// spinner is the frames of the refresh spinner.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func New(deps Deps) Model {
	m := Model{deps: deps, order: byRecent, refreshing: true, ticking: true, holding: true, holds: 1}
	if deps.SortByName {
		m.order = byName
	}
	for _, c := range deps.Columns {
		if c != "folder" || deps.RowStyle == "name-folder" { // the other styles have no folder column
			m.columns = append(m.columns, c)
		}
	}
	m.helpLines, m.helpWidth = m.helpContent()
	notice := deps.Notice
	if deps.Created {
		notice = "Created " + safe(m.shorten(deps.ConfigFile)) + " with a cd target on o"
	}
	if notice != "" {
		m.notice, m.notices = notice, 1
	}
	return m
}

// Init reads the cache and the history. The refresh starts once the cached
// list has arrived, so the list is on screen before the refresh's result.
func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg {
		list := m.deps.Cache.Load()
		c := cachedMsg{scannedAt: list.ScannedAt, paths: list.Paths}
		for _, r := range list.Repos {
			c.repos = append(c.repos, repo{path: r.Path, root: r.Root, branch: r.Branch})
		}
		return c
	}, func() tea.Msg { return historyMsg(m.deps.History.Load()) }, m.tick(), m.endHold(), m.endNotice())
}

// tick moves the spinner on after a while.
func (m Model) tick() tea.Cmd {
	return m.deps.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// endHold ends the hold of the latest refresh's spinner.
func (m Model) endHold() tea.Cmd {
	n := holdEndMsg(m.holds)
	return m.deps.Tick(m.deps.SpinFor, func(time.Time) tea.Msg { return n })
}

// refreshed says how many repos the refresh loaded, once its scan is done
// and its spinner has shown long enough, unless it found a problem, which
// then shows as it is, or no repos, which the list then says. A notice
// showing at that moment keeps it, and this one follows.
func (m Model) refreshed() (Model, tea.Cmd) {
	if m.refreshing || m.holding || len(m.missing) > 0 || len(m.all) == 0 {
		return m, nil
	}
	text := fmt.Sprintf("%d repos loaded", len(m.all))
	if len(m.all) == 1 {
		text = "1 repo loaded"
	}
	if m.notice != "" {
		m.next = text
		return m, nil
	}
	return m.showNotice(text)
}

// quit ends the program, or while an open is running waits for it, so its
// history entry is saved first.
func (m Model) quit() (Model, tea.Cmd) {
	if m.opening {
		m.quitting = true
		return m, nil
	}
	return m, tea.Quit
}

// showNotice shows text at the top right for a moment.
func (m Model) showNotice(text string) (Model, tea.Cmd) {
	m.notice = text
	m.notices++
	return m, m.endNotice()
}

// endNotice ends the moment of the latest notice, if there is one.
func (m Model) endNotice() tea.Cmd {
	if m.notices == 0 {
		return nil
	}
	n := noticeEndMsg(m.notices)
	return m.deps.Tick(m.deps.NoticeFor, func(time.Time) tea.Msg { return n })
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
		m, cmd = m.refreshed()
	case holdEndMsg:
		if int(msg) == m.holds {
			m.holding = false
			m, cmd = m.refreshed()
		}
	case historyMsg:
		// An open recorded before the history arrived is newer than the file
		// that was read, so it stays.
		h := map[string]time.Time{}
		maps.Copy(h, msg)
		maps.Copy(h, m.history)
		m.history = h
		m = m.relist(m.all)
	case openedMsg:
		quit := msg.close || m.quitting
		m.opening, m.quitting = false, false
		h := map[string]time.Time{} // a new map: an older model may still hold the old one
		maps.Copy(h, m.history)
		h[msg.path] = msg.at
		m.history = h
		m = m.relist(m.all)
		notice := "Opened " + safe(filepath.Base(msg.path)) + " in " + msg.target
		if msg.saveErr != nil { // a notice has part of a line: room to name the problem, not its cause
			notice += ", but the history could not be saved"
		}
		m, cmd = m.showNotice(notice)
		if quit && m.failure == nil && !m.add.open { // never quit while an error or the add dialog is showing
			return m, tea.Quit
		}
	case handOverMsg:
		m.handOver = &msg.h
		return m, tea.Quit
	case failedMsg:
		m.opening, m.quitting = false, false
		// The dialog replaces a menu or the help opened while the target ran.
		m.menu, m.help, m.failure = false, false, m.newErrorDialog(msg)
	case fileCheckedMsg:
		// The refusal replaces the fields and what was typed in them; a dialog
		// closed or past its fields since is left alone.
		if a := m.add; msg.err != nil && a.open && a.old == msg.old && a.refusal == "" && !a.confirming && !a.writing {
			m.add = addDialog{open: true, old: a.old, remove: a.remove, refusal: msg.err.Error()}
		}
	case checkedMsg:
		// A check of fields typed over since, or of a dialog closed, is stale.
		if a := m.add; a.open && a.old == msg.old && a.refusal == "" && !a.writing && msg.fields == a.fields && msg.after == a.after {
			m.add.problem = ""
			switch {
			case msg.err != nil && a.remove: // with no fields to put it right in
				m.add.refusal = msg.err.Error()
			case msg.err != nil:
				m.add.problem = msg.err.Error()
			default:
				m.add.confirming, m.add.before, m.add.block = true, msg.before, msg.block
				m, cmd = m.pauseEnter()
			}
		}
	case readyMsg:
		if int(msg) == m.confirms {
			m.add.ready = true
		}
	case changedMsg:
		a := m.add
		switch {
		case msg.err != nil && a.remove:
			m.add = addDialog{open: true, old: a.old, remove: true, refusal: msg.err.Error()}
			return m, nil
		case msg.err != nil: // back to the fields, to put the problem right
			m.add = addDialog{open: true, old: a.old, fields: a.fields, after: a.after, field: a.field, problem: msg.err.Error()}
			return m, nil
		}
		var notice string
		switch {
		case a.remove:
			notice = "Removed " + a.old.Name
		case a.old != nil:
			notice = "Changed " + msg.targets[msg.at].Name
		default:
			notice = "Added " + msg.targets[msg.at].Name
		}
		m.add = addDialog{}
		m.deps.Targets = msg.targets
		m.helpLines, m.helpWidth = m.helpContent()
		// The menu an edit or a removal came from stays, highlighting the
		// target edited or the one in the removed one's place, unless none is
		// left.
		m.choice = max(min(msg.at, len(msg.targets)-1), 0)
		m.menu = m.menu && len(msg.targets) > 0
		m = m.scrollMenu()
		m, cmd = m.showNotice(notice)
	case noticeEndMsg:
		if int(msg) == m.notices {
			m.notice = ""
			if m.next != "" {
				m, cmd = m.showNotice(m.next)
				m.next = ""
			}
		}
	case tickMsg:
		m.ticking = m.refreshing || m.holding // the spinner stops with the refresh and its hold
		if m.ticking {
			m.frame = (m.frame + 1) % len(spinner)
			cmd = m.tick()
		}
	case tea.PasteMsg:
		switch { // as typed text, which only a search and the add dialog's fields take
		case m.failure != nil:
		case m.add.open && !m.add.remove && m.add.refusal == "" && !m.add.confirming && !m.add.writing:
			if m.add.field < len(m.add.fields) { // the choice takes no text
				m.add.fields[m.add.field] += clean(msg.Content)
			}
		case m.searching && !m.menu:
			m = m.search(m.query + clean(msg.Content))
		}
	case tea.WindowSizeMsg:
		hidden := !m.addFits()
		m.width, m.height = msg.Width, msg.Height
		if a := m.add; a.open && a.confirming && !a.writing && hidden && m.addFits() { // the confirmation shows only now
			m, cmd = m.pauseEnter()
		}
		m.menu = m.menu && m.menuRows() > 0
		m = m.scrollMenu()
		m.help = m.help && m.helpRows() > 0
		m.helpTop = min(m.helpTop, len(m.helpLines)-m.helpRows())
	case tea.KeyPressMsg:
		m.notice, m.next = "", ""
		m.follow = m.follow || len(m.repos) > 0
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.failure != nil {
			switch msg.String() {
			case "q":
				return m.quit()
			case "esc":
				m.failure = nil
			}
			return m, nil
		}
		if m.add.open {
			return m.addKey(msg)
		}
		if m.help {
			return m.helpKey(msg)
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
			return m.quit()
		case "j", "down":
			m.selected = min(m.selected+1, max(len(m.repos)-1, 0))
		case "k", "up":
			m.selected = max(m.selected-1, 0)
		case "g":
			m.selected = 0
		case "G":
			m.selected = max(len(m.repos)-1, 0)
		case "ctrl+d":
			m.selected = min(m.selected+max(m.pageHeight()/2, 1), max(len(m.repos)-1, 0))
		case "ctrl+u":
			m.selected = max(m.selected-max(m.pageHeight()/2, 1), 0)
		case "enter", "space":
			m.menu, m.choice, m.menuTop = len(m.repos) > 0 && m.menuRows() > 0, 0, 0
		case "/":
			m.searching, m.before = true, ""
			if len(m.repos) > 0 {
				m.before = m.repos[m.selected].path
			}
			m.repos, m.hits = m.filter() // an empty query matches every repo, in the order shown
		case "?":
			m.help, m.helpTop = m.helpRows() > 0, 0
		case "a":
			m.add, cmd = addDialog{open: true}, m.checkFile()
		case "e":
			m, cmd = m.showNotice("e edits a target from the target menu: press enter first")
		case "d":
			m, cmd = m.showNotice("d removes a target from the target menu: press enter first")
		case "s":
			m.order = (m.order + 1) % len(sorts)
			m = m.relist(m.all)
		case "r":
			if !m.refreshing {
				m.refreshing, m.holding = true, true
				m.holds++
				cmd = tea.Batch(m.refresh(), m.endHold())
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
	m.offset = min(m.offset, m.selected, max(len(m.repos)-m.pageHeight(), 0))
	m.offset = max(m.offset, m.selected-m.pageHeight()+1)
	return m, cmd
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

// open opens the selected repo in t in the background and closes the menu,
// or for a target in place looks up its program, records the open and ends
// the program asking for the hand-over, unless another open is still
// running: then it does nothing, so that one open's result can never hide
// another's.
func (m Model) open(t target.Target) (Model, tea.Cmd) {
	if len(m.repos) == 0 || m.opening {
		return m, nil
	}
	m.menu, m.opening = false, true
	path := m.repos[m.selected].path
	if t.InPlace {
		return m, func() tea.Msg {
			h, err := target.Prepare(t, path)
			if err != nil {
				return failedMsg{repo: filepath.Base(path), target: t.Name, Failure: *err.(*target.Failure)}
			}
			// A history that cannot be saved does not stop the hand-over,
			// whose notice no one would see.
			_ = m.deps.History.Record(path, m.deps.Now().Round(0))
			return handOverMsg{h}
		}
	}
	quit := m.deps.CloseOnOpen
	if t.Close != nil {
		quit = *t.Close
	}
	return m, func() tea.Msg {
		if err := m.deps.Open(t, path); err != nil {
			f := &target.Failure{Reason: err.Error()} // in case the error is not a Failure
			errors.As(err, &f)
			return failedMsg{repo: filepath.Base(path), target: t.Name, Failure: *f}
		}
		// Recorded before the open is reported, so it is on disk even when the
		// tool quits on the report. Round(0) keeps only the wall clock, as the
		// file does: a monotonic reading can stop while the computer sleeps.
		at := m.deps.Now().Round(0)
		return openedMsg{path: path, target: t.Name, at: at, saveErr: m.deps.History.Record(path, at), close: quit}
	}
}
