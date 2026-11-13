// Package ui is the Bubble Tea program: the repo list and its keys.
package ui

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/matcher"
	"github.com/knutselio/loper/internal/reltime"
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
	// Tick is tea.Tick, which drives the refresh spinner and ends footer
	// notices. Tests deliver its ticks themselves, so neither needs real time.
	Tick func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd
	// Notice is shown in the footer at launch, as a notice, or is "".
	Notice string
	// NoticeFor is how long a footer notice shows before the keys return.
	NoticeFor time.Duration
	// EnterPause is how long after the dialog's confirmation appears an
	// enter is ignored, so a quick second enter cannot write unseen.
	EnterPause time.Duration
	// CloseOnOpen quits the program after a successful open.
	CloseOnOpen bool
	// SortByName starts the list sorted by name rather than by last opened.
	SortByName bool
	// CacheMaxAge is how old the cache may be before a launch scans again
	// rather than re-reading the cached repos; 0 means every launch scans.
	CacheMaxAge time.Duration
	// Styles turns the styles on: the bold title and selection marker, the
	// bar of the selection, dimmed headings, folders, rules and borders,
	// underlined matches, and bold keys in the footer, the help and the menu.
	// Without it the output has no escape codes at all.
	Styles bool
	// Color, with Styles, draws the keys in the accent colour. It is off
	// when NO_COLOR is set and non-empty, and then no colour is drawn.
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
	notice   string       // shown in place of the footer keys for a moment, or until the next key press
	notices  int          // how many notices have shown, so the end of an earlier one's moment leaves a later one
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
	byName   bool         // sorted by name rather than by last opened
	// helpLines and helpWidth are the help's rows and the width of the
	// widest, worked out once in New rather than on every render.
	helpLines []string
	helpWidth int
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

// repo is one row of the list.
type repo struct {
	path   string
	root   string // the configured path it was found under
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
	close        bool // the tool quits on it, as the target or close_on_open says
}

// handOverMsg asks for the tool to end and h to run in its place.
type handOverMsg struct{ h target.HandOver }

// failedMsg reports that a repo did not open in a target.
type failedMsg struct {
	repo, target string
	target.Failure
}

// addDialog is the dialog that adds a target to the config file, or edits or
// removes one: its fields, then the confirmation of the lines it writes. A
// removal has no fields.
type addDialog struct {
	open bool
	// old is the target edited or removed, as the menu listed it, or nil
	// while adding one. Each dialog has its own, so a result for an earlier
	// dialog is told apart by it.
	old        *target.Target
	remove     bool
	refusal    string    // why the config file cannot take the change, shown instead of the fields
	fields     [3]string // the name, the key and the command, as typed
	after      int       // the After open choice after them, an index into choices
	field      int       // the field being typed in, or len(fields) for the choice
	confirming bool      // the confirmation shows
	before     string    // old's lines in the file, while the confirmation shows them
	block      string    // the lines to write, while the confirmation shows them
	ready      bool      // the confirmation's first moment, in which enter is ignored, has passed
	writing    bool      // the change is being written, so keys wait for the result
	problem    string    // why the target was refused or not written
}

// change is the change to the config file that the dialog makes.
func (a addDialog) change() target.Change {
	var quit *bool // default and take over: no close line
	inPlace := choices[a.after] == "take over this window"
	if a.after > 0 && !inPlace {
		c := choices[a.after] == "close loper"
		quit = &c
	}
	return target.Change{Old: a.old, Lines: a.before, Remove: a.remove, Name: a.fields[0], Key: a.fields[1], Command: a.fields[2], Close: quit, InPlace: inPlace}
}

// choiceOf returns the index in choices of what quit and inPlace, a
// target's close and in_place, say.
func choiceOf(quit *bool, inPlace bool) int {
	switch {
	case inPlace:
		return slices.Index(choices, "take over this window")
	case quit == nil:
		return 0
	case *quit:
		return slices.Index(choices, "close loper")
	}
	return slices.Index(choices, "stay open")
}

// title is the dialog's title.
func (a addDialog) title() string {
	switch {
	case a.remove:
		return " Remove a target "
	case a.old != nil:
		return " Edit a target "
	}
	return " Add a target "
}

// fileCheckedMsg carries why the config file cannot take the change of the
// dialog for old, or nil when it can.
type fileCheckedMsg struct {
	old *target.Target
	err error
}

// readyMsg ends the first moment of the confirmation it counts.
type readyMsg int

// checkedMsg carries, for the dialog for old with fields and after, the
// lines its change replaces and those it writes, or why the change is refused.
type checkedMsg struct {
	old           *target.Target
	fields        [3]string
	after         int
	before, block string
	err           error
}

// changedMsg carries the targets once the dialog's change was written, and
// the index of the target added or changed, or of where the removed one was,
// or why it was not written.
type changedMsg struct {
	targets []target.Target
	at      int
	err     error
}

// tickMsg moves the spinner on a frame.
type tickMsg struct{}

// noticeEndMsg ends the moment of the footer notice it counts.
type noticeEndMsg int

// examples are what the add dialog shows, dimmed, in its empty fields: the
// name, the key and the command of a target.
var examples = [3]string{"Zed", "z", "zed {path}"}

// choices are the add dialog's choices of what happens after an open: what
// close_on_open says, stay open, close loper, or run in its place.
var choices = []string{"default", "stay open", "close loper", "take over this window"}

// headings are the column headings, by column.
var headings = map[string]string{"name": "NAME", "folder": "FOLDER", "branch": "BRANCH", "last_opened": "OPENED"}

// spinner is the frames of the refresh spinner.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// The styles, from the terminal's own palette and attributes only, so the
// tool follows the terminal's theme. matchOn and matchOff start and end the
// highlight of a matched letter, dimOn and dimOff a dimmed part (a folder,
// the path before a name, the rules and the borders of a box), boldOn and
// boldOff the title and the marker of the selected row or target, and keyOn
// and keyOff the keys of the footer, the help and the target menu, in the
// accent colour. barOn and barOff draw the selected row or target as a bar,
// in reverse video; the styles inside the bar end only their own attribute,
// so none of them ends the bar.
var (
	matchOn  = ansi.Style{}.Underline(true).String()
	matchOff = ansi.Style{}.NoUnderline().String()
	dimOn    = ansi.Style{}.Faint().String()
	dimOff   = ansi.Style{}.Normal().String()
	boldOn   = ansi.Style{}.Bold().String()
	boldOff  = ansi.Style{}.Normal().String()
	keyOn    = ansi.Style{}.Bold().ForegroundColor(ansi.Cyan).String()
	keyOff   = ansi.Style{}.Normal().DefaultForegroundColor().String()
	barOn    = ansi.Style{}.Reverse(true).String()
	barOff   = ansi.Style{}.NoReverse().String()
)

func New(deps Deps) Model {
	m := Model{deps: deps, byName: deps.SortByName, refreshing: true, ticking: true}
	for _, c := range deps.Columns {
		if c != "folder" || deps.RowStyle == "name-folder" { // the other styles have no folder column
			m.columns = append(m.columns, c)
		}
	}
	m.helpLines, m.helpWidth = m.helpContent()
	if deps.Notice != "" {
		m.notice, m.notices = deps.Notice, 1
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
	}, func() tea.Msg { return historyMsg(m.deps.History.Load()) }, m.tick(), m.endNotice())
}

// tick moves the spinner on after a while.
func (m Model) tick() tea.Cmd {
	return m.deps.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
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

// showNotice shows text in the footer for a moment.
func (m Model) showNotice(text string) (Model, tea.Cmd) {
	m.notice = text
	m.notices++
	return m, m.endNotice()
}

// endNotice ends the moment of the latest footer notice, if there is one.
func (m Model) endNotice() tea.Cmd {
	if m.notices == 0 {
		return nil
	}
	n := noticeEndMsg(m.notices)
	return m.deps.Tick(m.deps.NoticeFor, func(time.Time) tea.Msg { return n })
}

// refresh is the full refresh: it scans the configured paths, reads each
// repo's branch and rewrites the cache.
func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		missing := m.deps.Missing(m.deps.Paths)
		var found []repo
		for _, r := range m.deps.Scan(m.deps.Paths) {
			found = append(found, repo{path: r.Path, root: r.Root})
		}
		return scannedMsg{repos: m.save(found, m.deps.Now()), missing: missing}
	}
}

// reread is the light refresh: it drops the cached repos that are gone and
// reads the branch of the rest, without scanning; it checks only that the
// configured paths exist. The cache keeps the time of its scan, so it still
// expires.
func (m Model) reread(c cachedMsg) tea.Cmd {
	return func() tea.Msg {
		var found []repo
		for _, r := range c.repos {
			if m.deps.Exists(r.path) {
				found = append(found, r)
			}
		}
		return scannedMsg{repos: m.save(found, c.scannedAt), missing: m.deps.Missing(m.deps.Paths)}
	}
}

// save reads the branch of each of found and rewrites the cache with them,
// as scanned at scannedAt from the configured paths.
func (m Model) save(found []repo, scannedAt time.Time) []repo {
	list := cache.List{ScannedAt: scannedAt, Paths: m.deps.Paths}
	var repos []repo
	for _, r := range found {
		r.branch = m.deps.Branch(r.path)
		repos = append(repos, r)
		list.Repos = append(list.Repos, cache.Repo{Path: r.path, Root: r.root, Branch: r.branch})
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
		quit := msg.close || m.quitting
		m.opening, m.quitting = false, false
		h := map[string]time.Time{} // a new map: an older model may still hold the old one
		maps.Copy(h, m.history)
		h[msg.path] = msg.at
		m.history = h
		m = m.relist(m.all)
		notice := "Opened " + safe(filepath.Base(msg.path)) + " in " + msg.target
		if msg.saveErr != nil { // the footer has one line: room to name the problem, not its cause
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
		m.menu, m.help, m.failure = false, false, newErrorDialog(msg)
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
		}
	case tickMsg:
		m.ticking = m.refreshing // the spinner stops with the refresh
		if m.refreshing {
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
		m.notice = ""
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
	m.offset = min(m.offset, m.selected, max(len(m.repos)-m.pageHeight(), 0))
	m.offset = max(m.offset, m.selected-m.pageHeight()+1)
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
		rows[i] = matcher.Row{Name: m.name(r), Folder: safe(m.shorten(filepath.Dir(r.path))), Branch: safe(r.branch)}
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
	if len(m.deps.Targets) == 0 { // the menu says there are none, and a adds one
		switch msg.String() {
		case "q":
			return m.quit()
		case "a":
			m.menu, m.add = false, addDialog{open: true}
			return m, m.checkFile()
		case "esc":
			m.menu = false
		}
		return m, nil
	}
	switch msg.String() {
	case "q":
		return m.quit()
	case "j", "down":
		m.choice = min(m.choice+1, len(m.deps.Targets)-1)
	case "k", "up":
		m.choice = max(m.choice-1, 0)
	case "enter", "space":
		return m.open(m.deps.Targets[m.choice])
	case "e":
		t := m.deps.Targets[m.choice] // a copy of its own, which tells this dialog apart
		m.add = addDialog{open: true, old: &t, fields: [3]string{t.Name, t.Key, t.Command}, after: choiceOf(t.Close, t.InPlace)}
		return m, m.checkFile()
	case "d":
		t := m.deps.Targets[m.choice]
		m.add = addDialog{open: true, old: &t, remove: true}
		return m, m.check()
	case "esc":
		m.menu = false
	default:
		if t, ok := m.keyed(msg); ok {
			return m.open(t)
		}
	}
	return m, nil
}

// addKey handles a key press while the dialog is open. In the fields,
// printable characters are text, so list keys and target keys are too.
func (m Model) addKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	a := &m.add
	switch {
	case a.writing:
	case a.refusal != "":
		switch msg.String() {
		case "q":
			if m.opening { // closed, so the quit goes ahead once the open is done
				m.add = addDialog{}
			}
			return m.quit()
		case "esc":
			m.add = addDialog{}
		}
	case a.confirming:
		switch msg.String() {
		case "enter":
			if a.ready && m.addFits() { // never what was not there to read
				a.writing = true
				return m, m.write()
			}
		case "esc":
			if a.remove {
				m.add = addDialog{}
				break
			}
			a.confirming, a.before, a.block = false, "", ""
		}
	case a.remove: // its lines are still being read
		if msg.String() == "esc" {
			m.add = addDialog{}
		}
	default:
		last := len(a.fields) // the choice, after the text fields
		switch msg.String() {
		case "tab", "down":
			a.field = min(a.field+1, last)
		case "shift+tab", "up":
			a.field = max(a.field-1, 0)
		case "enter":
			if a.field < last {
				a.field++
				break
			}
			if o := a.old; o != nil && strings.TrimSpace(a.fields[0]) == o.Name && a.fields[1] == o.Key && a.fields[2] == o.Command && a.after == choiceOf(o.Close, o.InPlace) {
				m.add = addDialog{} // an edit that changes nothing writes nothing
				break
			}
			return m, m.check()
		case "esc":
			m.add = addDialog{}
		case "backspace":
			if a.field < last {
				if f := []rune(a.fields[a.field]); len(f) > 0 {
					a.fields[a.field] = string(f[:len(f)-1])
				}
			}
		default:
			if a.field < last {
				a.fields[a.field] += msg.Text
				break
			}
			switch msg.String() { // the choice takes no text
			case "right", "space":
				a.after = (a.after + 1) % len(choices)
			case "left":
				a.after = (a.after + len(choices) - 1) % len(choices)
			}
		}
	}
	return m, nil
}

// pauseEnter starts the moment, after the confirmation shows, in which enter
// is ignored.
func (m Model) pauseEnter() (Model, tea.Cmd) {
	m.add.ready = false
	m.confirms++
	n := readyMsg(m.confirms)
	return m, m.deps.Tick(m.deps.EnterPause, func(time.Time) tea.Msg { return n })
}

// checkFile checks in the background whether the config file can take the
// dialog's change, as its fields open: a target added, or the target edited
// as it is.
func (m Model) checkFile() tea.Cmd {
	a := m.add
	return func() tea.Msg {
		if a.old == nil {
			return fileCheckedMsg{err: m.deps.CheckFile(m.deps.ConfigFile, m.deps.Home)}
		}
		_, _, err := m.deps.CheckTarget(m.deps.ConfigFile, m.deps.Home, a.change())
		return fileCheckedMsg{old: a.old, err: err}
	}
}

// check checks the dialog's change against the config file in the
// background.
func (m Model) check() tea.Cmd {
	a := m.add
	return func() tea.Msg {
		before, block, err := m.deps.CheckTarget(m.deps.ConfigFile, m.deps.Home, a.change())
		return checkedMsg{old: a.old, fields: a.fields, after: a.after, before: before, block: block, err: err}
	}
}

// write makes the dialog's change to the config file in the background.
func (m Model) write() tea.Cmd {
	c := m.add.change()
	return func() tea.Msg {
		targets, at, err := m.deps.WriteTarget(m.deps.ConfigFile, m.deps.Home, c)
		return changedMsg{targets: targets, at: at, err: err}
	}
}

// menuRows is the number of targets the menu has room to show between its
// borders, or 0 when the list is too short for the menu. With no targets
// the menu is the one row that says so.
func (m Model) menuRows() int {
	return max(min(max(len(m.deps.Targets), 1), m.listHeight()-2), 0)
}

// scrollMenu moves the menu's first shown target so the highlighted one is
// in view.
func (m Model) scrollMenu() Model {
	rows := max(m.menuRows(), 1)
	m.menuTop = min(m.menuTop, m.choice, len(m.deps.Targets)-rows)
	m.menuTop = max(m.menuTop, m.choice-rows+1)
	return m
}

// helpKey handles a key press while the help is open: it scrolls, closes
// or quits, and every other key does nothing.
func (m Model) helpKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.quit()
	case "j", "down":
		m.helpTop = min(m.helpTop+1, len(m.helpLines)-m.helpRows())
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "esc", "?":
		m.help = false
	}
	return m, nil
}

// helpRows is the number of rows of the help there is room for between its
// borders, or 0 when the list is too short for the help.
func (m Model) helpRows() int {
	return max(min(len(m.helpLines), m.listHeight()-2), 0)
}

// helpContent returns the rows of the help, the keys from KeyList and those
// of the help itself, and the width of the widest.
func (m Model) helpContent() (rows []string, width int) {
	sections := append(KeyList(), Section{Title: "In this help", Bindings: []Binding{
		{Keys: "j / k, ↓ / ↑", Action: "Scroll"},
		{Keys: "esc, ?", Action: "Close the help"},
	}})
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.Bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.Keys))
		}
	}
	for i, s := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		if s.Title != "" {
			rows = append(rows, " "+s.Title+" ")
		}
		for _, b := range s.Bindings {
			rows = append(rows, " "+pad(m.key(b.Keys), keyWidth)+"  "+b.Action+" ")
		}
	}
	rows = append(rows, "", " Targets ")
	if len(m.deps.Targets) == 0 {
		rows = append(rows, " None yet. Press "+m.key("a")+" to add one. ")
	}
	for _, t := range m.deps.Targets {
		name := t.Name
		if t.Key == "" {
			name += " (through the menu)"
		}
		rows = append(rows, " "+pad(m.key(t.Key), keyWidth)+"  "+name+" ")
	}
	for _, r := range rows {
		width = max(width, ansi.StringWidth(r))
	}
	return rows, width
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
	now := m.deps.Now()
	// The width of each column's widest value, over all repos and the
	// heading, so the columns stay put while searching.
	full := make([]int, len(m.columns))
	for i, c := range m.columns {
		full[i] = ansi.StringWidth(headings[c])
	}
	for _, r := range m.all {
		for i, c := range m.columns {
			full[i] = max(full[i], ansi.StringWidth(m.cell(c, r, matcher.Match{}, now, math.MaxInt)))
		}
	}
	shown, widths := m.fit(full)
	rule := ansi.Truncate(" "+m.dim(strings.Repeat("─", max(m.width-2, 0)))+" ", m.width, "")
	noun := "repos"
	if len(m.repos) == 1 {
		noun = "repo"
	}
	count, warnings := fmt.Sprintf(" %s · %d %s", m.bold("loper"), len(m.repos), noun), ""
	for _, path := range m.missing {
		warnings += "  " + safe(m.shorten(path)) + " not found"
	}
	sort := "  sort: last opened "
	if m.byName {
		sort = "  sort: name "
	}
	if m.searching { // the search line replaces the count, the warnings and the sort
		count, warnings, sort = " /"+m.query, "", ""
	}
	spin := ""
	if m.refreshing && len(m.all) > 0 { // with no list yet, the spinner is in the list
		spin = "  " + spinner[m.frame] + " refreshing "
	}
	// The sort and the spinner keep their place on the right. What does not
	// fit gives way: the warnings first, then the sort, then the spinner; the
	// count, or the search line, is cut last.
	right := sort + spin
	if ansi.StringWidth(count+right) > m.width {
		warnings, right = "", spin
	}
	if ansi.StringWidth(count+right) > m.width {
		right = ""
	}
	room := m.width - ansi.StringWidth(right)
	if over := ansi.StringWidth(count) - room; m.searching && over > 0 && room >= ansi.StringWidth(" /…") {
		count = " /…" + cutStart(m.query, over+1) // the end of the query, where the typing is
	}
	left := ansi.Truncate(count, room, "…")
	if rest := room - ansi.StringWidth(left); rest >= ansi.StringWidth("  …") { // less would not read as a cut warning
		left += ansi.Truncate(warnings, rest, "…")
	}
	lines := []string{pad(left, room) + right, rule}
	var empty []string // what an empty list says instead
	switch {
	case len(m.repos) > 0:
	case m.searching && len(m.all) > 0:
		empty = []string{"No matches"}
	case !m.refreshing:
		paths := make([]string, len(m.deps.Paths))
		for i, p := range m.deps.Paths {
			paths[i] = safe(m.shorten(p))
		}
		empty = []string{"No repos found in " + strings.Join(paths, ", "), "Change paths in " + safe(m.shorten(m.deps.ConfigFile))}
	case m.loaded:
		empty = []string{spinner[m.frame] + " scanning…"}
	}
	// join lines up the shown columns, each cell from cell given its room.
	join := func(cell func(c string, room int) string) string {
		cells := make([]string, len(shown))
		for j, k := range shown {
			room := widths[k]
			if len(shown) == 1 { // the name alone has the whole row
				room = max(m.width-3, 0)
			}
			cells[j] = cell(m.columns[k], room)
			if j < len(cells)-1 { // the last column is not padded, so a row has no trailing spaces
				cells[j] = pad(cells[j], widths[k])
			}
		}
		return strings.Join(cells, "  ")
	}
	if m.pageHeight() < m.listHeight() {
		line := join(func(c string, room int) string { return headings[c] })
		lines = append(lines, ansi.Truncate("   "+m.dim(line), m.width, "…"))
	}
	for i := m.offset; i < m.offset+m.pageHeight(); i++ {
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
			marker = " " + m.bold(">") + " "
		}
		var h matcher.Match
		if m.hits != nil {
			h = m.hits[i]
		}
		cells := join(func(c string, room int) string { return m.cell(c, r, h, now, room) })
		line := ansi.Truncate(marker+cells, m.width, "…")
		if i == m.selected {
			line = m.bar(line, m.width)
		}
		lines = append(lines, line)
	}
	keys := m.footer()
	if m.notice != "" {
		keys = ansi.Truncate(" "+message(m.notice), m.width, "…")
	}
	if m.quitting {
		keys = ansi.Truncate(" Quitting when the open is done", m.width, "…")
	}
	switch {
	case m.failure != nil:
		lines = m.overlayFailure(lines)
	case m.add.open:
		lines = m.overlayAdd(lines)
	case m.menu:
		lines = m.overlayMenu(lines)
	case m.help:
		lines = m.overlay(lines, m.box(" Help ", m.helpLines[m.helpTop:m.helpTop+m.helpRows()], m.helpWidth))
	}
	lines = append(lines, rule, keys)
	lines = lines[:min(len(lines), m.height)] // never taller than the terminal
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// footer is the footer's line of keys: the tool's own keys, then the keyed
// targets, then the keys that come last, separated by " | ". What does not fit gives way whole items at a
// time from the end, and a … then ends the line.
func (m Model) footer() string {
	var items, last []string
	for _, s := range KeyList() {
		for _, b := range s.Bindings {
			switch {
			case b.FooterLabel == "":
			case b.FooterLast:
				last = append(last, m.key(b.FooterKey)+" "+b.FooterLabel)
			default:
				items = append(items, m.key(b.FooterKey)+" "+b.FooterLabel)
			}
		}
	}
	for _, t := range m.deps.Targets {
		if t.Key != "" {
			items = append(items, m.key(t.Key)+" "+t.Name)
		}
	}
	items = append(items, last...)
	for n := len(items); n >= 0; n-- {
		shown := items[:n]
		if n < len(items) {
			shown = append(slices.Clone(shown), "…")
		}
		if line := " " + strings.Join(shown, m.dim(" | ")); ansi.StringWidth(line) <= m.width {
			return line
		}
	}
	return ansi.Truncate(" …", m.width, "")
}

// fit returns the indexes of the columns shown and the width of each column,
// given the width of each column's widest value. While the rows would be
// wider than the terminal, the folder and branch columns shrink, the widest
// first, down to 12 cells; a column narrower than that keeps its width. When
// that is not enough, last_opened is dropped, then branch, then folder,
// whatever order the columns are in, and the columns left shrink again from
// their full width. The name never shrinks here.
func (m Model) fit(full []int) (shown, widths []int) {
	shown = make([]int, len(m.columns))
	for i := range shown {
		shown[i] = i
	}
	for _, drop := range []string{"last_opened", "branch", "folder", ""} { // "" drops nothing: the last pass only shrinks
		widths = slices.Clone(full)
		for {
			width := 3 + 2*(len(shown)-1) // the marker, and the gaps between columns
			widest := -1                  // the folder or branch column to shrink next
			for _, i := range shown {
				width += widths[i]
				if c := m.columns[i]; (c == "folder" || c == "branch") && widths[i] > 12 && (widest < 0 || widths[i] > widths[widest]) {
					widest = i
				}
			}
			if width <= m.width {
				return shown, widths
			}
			if widest < 0 {
				break
			}
			widths[widest]--
		}
		shown = slices.DeleteFunc(shown, func(i int) bool { return m.columns[i] == drop })
	}
	return shown, widths
}

// cell returns the text of column c for r, as of now, with the letters
// matched by h highlighted, cut with … to room cells.
func (m Model) cell(c string, r repo, h matcher.Match, now time.Time, room int) string {
	var s string
	switch c {
	case "name":
		// Under the root-path style, the path before the name is dimmed.
		name := m.name(r)
		cut := strings.LastIndex(name, string(filepath.Separator)) + 1
		k := utf8.RuneCountInString(name[:cut])
		var rest []int // the matched positions in the name itself, counted from its start
		for _, p := range h.Name {
			if p >= k {
				rest = append(rest, p-k)
			}
		}
		path, base := m.highlight(name[:cut], h.Name), m.highlight(name[cut:], rest)
		over := ansi.StringWidth(path+base) - room
		switch {
		case over <= 0:
			return m.dim(path) + base
		case ansi.StringWidth(base) < room: // the path gives way from its start, so the name stays whole
			return m.dim("…"+cutStart(path, over+1)) + base
		}
		return ansi.Truncate(base, room, "…")
	case "folder":
		s = m.dim(m.highlight(safe(m.shorten(filepath.Dir(r.path))), h.Folder))
	case "branch":
		s = m.highlight(safe(r.branch), h.Branch)
	default: // last_opened
		s = "-"
		if at, ok := m.history[r.path]; ok {
			s = reltime.Format(at, now)
		}
	}
	return ansi.Truncate(s, room, "…")
}

// cutStart removes the first n cells of s, and the rest of a wide character
// the cut would split.
func cutStart(s string, n int) string {
	if cut := ansi.TruncateLeft(s, n, ""); ansi.StringWidth(cut) <= ansi.StringWidth(s)-n {
		return cut
	}
	return ansi.TruncateLeft(s, n+1, "")
}

// name is the text of r's name column: its name or, under the root-path
// style, its path from its root, which for a repo listed directly in paths is
// its name, as display text.
func (m Model) name(r repo) string {
	if m.deps.RowStyle == "root-path" {
		// The error is dropped: in a list the tool scanned or cached itself, a
		// repo's path is its root or a folder under it, both absolute. A cache
		// edited by hand could break that; the row then shows "" or a path
		// with "..", and the next refresh puts it right.
		if rel, _ := filepath.Rel(r.root, r.path); rel != "." {
			return safe(rel)
		}
	}
	return safe(filepath.Base(r.path))
}

// dim dims s, unless the styles are off.
func (m Model) dim(s string) string {
	if !m.deps.Styles || s == "" {
		return s
	}
	return dimOn + s + dimOff
}

// bold styles s as the title or the marker of what is selected, unless the
// styles are off.
func (m Model) bold(s string) string {
	if !m.deps.Styles || s == "" {
		return s
	}
	return boldOn + s + boldOff
}

// bar draws s, padded to width cells, as a bar, unless the styles are off.
func (m Model) bar(s string, width int) string {
	if !m.deps.Styles {
		return s
	}
	return barOn + pad(s, width) + barOff
}

// key styles s as a key: bold, and in the accent colour unless colour is off.
func (m Model) key(s string) string {
	if !m.deps.Color || s == "" {
		return m.bold(s)
	}
	return keyOn + s + keyOff
}

// highlight marks the characters of s at the positions pos, which are in
// ascending order and may go past the end of s, unless the styles are off.
func (m Model) highlight(s string, pos []int) string {
	if !m.deps.Styles || len(pos) == 0 {
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
			f := &target.Failure{Output: err.Error()} // in case the error is not a Failure
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

// overlayMenu draws the target menu centred over the list part of lines,
// which stays visible around it, or with no targets a box that says so.
func (m Model) overlayMenu(lines []string) []string {
	if len(m.deps.Targets) == 0 {
		row := " No targets yet. Press " + m.key("a") + " to add one. "
		return m.overlay(lines, m.box("", []string{row}, ansi.StringWidth(row)))
	}
	keyWidth := 0
	for _, t := range m.deps.Targets {
		keyWidth = max(keyWidth, ansi.StringWidth(t.Key))
	}
	title := " Open " + safe(filepath.Base(m.repos[m.selected].path)) + " "
	rows := make([]string, len(m.deps.Targets))
	width := 0
	for i, t := range m.deps.Targets {
		marker, key := "  ", m.key(t.Key)
		if i == m.choice { // in the bar, without the accent
			marker, key = m.bold(">")+" ", m.bold(t.Key)
		}
		rows[i] = " " + marker + pad(key, keyWidth) + "  " + t.Name + " "
		width = max(width, ansi.StringWidth(rows[i]))
	}
	hint := " " + m.key("e") + " edit  " + m.key("d") + " remove "
	width = m.boxWidth(title, max(width, ansi.StringWidth(hint)+2)) // the bar spans the menu
	rows[m.choice] = m.bar(ansi.Truncate(rows[m.choice], width, "…"), width)
	box := m.box(title, rows[m.menuTop:m.menuTop+m.menuRows()], width)
	box[len(box)-1] = m.edge("╰", hint, width, "╯") // the hint in the bottom border, as the title is in the top
	return m.overlay(lines, box)
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

// overlayAdd draws the dialog centred over the list part of lines or,
// when the screen is too small for it, a notice in its place: on the middle
// line of the list, or on the header when there is no list.
func (m Model) overlayAdd(lines []string) []string {
	if !m.addFits() {
		row := 0
		if m.listHeight() > 0 {
			row = 2 + (m.listHeight()-1)/2
		}
		lines[row] = ansi.Truncate(" Window too small for the dialog", m.width, "…")
		return lines
	}
	rows, width := m.addRows()
	return m.overlay(lines, m.box(m.add.title(), rows, width))
}

// addFits reports whether the dialog fits on the screen whole, with
// room in its fields for a value.
func (m Model) addFits() bool {
	rows, width := m.addRows()
	return len(rows) <= m.listHeight()-2 && width >= 17 // 17: the margins, the marker, the label and a cell of value
}

// addRows returns the dialog's rows and its width inside the border: why
// the config file cannot take the change, its fields, with the end of a
// value too long for its field, or the lines it will write, replace or
// remove and the file they are in. Text too long for the dialog wraps.
func (m Model) addRows() ([]string, int) {
	a := m.add
	width := m.boxWidth(a.title(), 60)
	wrap := func(s string) []string {
		var rows []string
		for _, l := range strings.Split(ansi.Wrap(clean(s), max(width-2, 1), ""), "\n") {
			rows = append(rows, " "+l+" ")
		}
		return rows
	}
	lines := func(block string) []string {
		var rows []string
		for _, l := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
			rows = append(rows, wrap(l)...)
		}
		return rows
	}
	file := safe(m.shorten(m.deps.ConfigFile))
	var rows []string
	switch {
	case a.refusal != "":
		rows = append(wrap(message(m.shorten(a.refusal))), "", " "+m.key("esc")+" close ") // a loader message starts with the file
	case a.confirming && a.remove:
		rows = slices.Concat(wrap("Remove these lines from "+file+":"), lines(short(a.before)),
			[]string{"", " " + m.key("enter") + " remove  " + m.key("esc") + " keep "})
	case a.confirming && a.old != nil:
		rows = slices.Concat(wrap("Replace these lines in "+file+":"), lines(short(a.before)), []string{""},
			wrap("with these (comments inside the block are not kept):"), lines(a.block),
			[]string{"", " " + m.key("enter") + " write  " + m.key("esc") + " back "})
	case a.confirming:
		rows = slices.Concat(wrap("Append to "+file), []string{""}, lines(a.block),
			[]string{"", " " + m.key("enter") + " write  " + m.key("esc") + " back "})
	case a.remove: // its lines are still being read
		rows = []string{" " + m.key("esc") + " keep "}
	default:
		after := choices[a.after]
		if a.after == 0 {
			after += " (" + choices[choiceOf(&m.deps.CloseOnOpen, false)] + ")"
		}
		values := append(a.fields[:], "‹ "+after+" ›") // drawn as a choice, changed with ← and →
		for i, label := range []string{"Name", "Key", "Command", "After open"} {
			marker := "  "
			if i == a.field {
				marker = m.bold(">") + " "
			}
			// A command as loaded may hold a line break or a tab.
			value := clean(values[i])
			if over := ansi.StringWidth(value) - (width - 16); over > 0 { // 16: the margins, the marker and the label
				value = "…" + cutStart(value, over+1)
			}
			if value == "" && m.deps.Styles { // undimmed the example would read as typed
				value = m.dim(examples[i])
			}
			rows = append(rows, " "+marker+pad(label, 10)+"  "+value+" ")
		}
		if a.problem != "" {
			rows = append(append(rows, ""), wrap(message(m.shorten(a.problem)))...) // a loader message starts with the file
		}
		rows = append(rows, "", " "+m.key("enter")+" next  "+m.key("tab")+" move  "+m.key("esc")+" cancel ")
	}
	return rows, width
}

// short returns block, lines as they are in the config file, cut to its
// first two lines and its last when it is longer than the block the tool
// writes, so a confirmation fits whenever one for such a block would.
func short(block string) string {
	ls := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	if len(ls) <= 5 { // the block the tool writes, with a close line
		return block
	}
	return strings.Join([]string{ls[0], ls[1], fmt.Sprintf("… %d more lines", len(ls)-3), ls[len(ls)-1]}, "\n")
}

// errorDialog is the error dialog's content, worked out once when the
// failure arrives rather than on every render.
type errorDialog struct {
	title string
	rows  []string // the command line that ran, then its output and why it failed
	width int      // of the widest row
}

func newErrorDialog(msg failedMsg) *errorDialog {
	d := &errorDialog{title: " Could not open " + safe(msg.repo) + " in " + msg.target + " "}
	d.rows = []string{" $ " + safe(msg.Command) + " "}
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

// safe turns outside text, such as a repo's name, folder or branch, into
// display text: control characters and bytes that are not UTF-8 are shown
// escaped, as \n, \r, \t, \x1b or \u009b, so none reaches the terminal. A
// backslash is shown as \\, so two different names never look the same.
// Other printable text is kept.
func safe(s string) string {
	return escape(s, `\\`)
}

// message is safe for a message, such as a loader's, that names files as
// they are but quotes other values as Go does, escaped already: a backslash
// is kept as it is, so a quoted value is not escaped twice.
func message(s string) string {
	return escape(s, `\`)
}

// escape is safe, with a backslash shown as backslash.
func escape(s, backslash string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[0])
		case r == '\\':
			b.WriteString(backslash)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < utf8.RuneSelf && unicode.IsControl(r): // C0 and DEL
			fmt.Fprintf(&b, `\x%02x`, r)
		case unicode.IsControl(r): // C1, told apart from a byte that is not UTF-8
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[:n])
		}
		s = s[n:]
	}
	return b.String()
}

// box draws rows in a rounded border with title in its top edge. width is
// what the rows want inside the border; the box grows to fit the title and
// shrinks to fit the screen, cutting rows that are too long.
func (m Model) box(title string, rows []string, width int) []string {
	width = m.boxWidth(title, width)
	box := []string{m.edge("╭", title, width, "╮")}
	side := m.dim("│")
	for _, r := range rows {
		box = append(box, side+pad(ansi.Truncate(r, width, "…"), width)+side)
	}
	return append(box, m.dim("╰"+strings.Repeat("─", width)+"╯"))
}

// edge draws the top or bottom border of a box width wide inside, from the
// corner left to the corner right, with text in it after one stroke of the
// border, if there is room, and cut to fit.
func (m Model) edge(left, text string, width int, right string) string {
	stroke := min(width, 1)
	text = ansi.Truncate(text, width-stroke, "")
	return m.dim(left+strings.Repeat("─", stroke)) + text + m.dim(strings.Repeat("─", width-stroke-ansi.StringWidth(text))+right)
}

// boxWidth is the width inside the border of a box with title whose rows
// want width: room for a stroke of the border either side of the title, and
// no more than the screen has inside the two side borders.
func (m Model) boxWidth(title string, width int) int {
	return max(min(max(width, ansi.StringWidth(title)+2), m.width-2), 0)
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
		// No style leaks across either edge of the box: Truncate keeps the
		// codes after the cut, so the row's styles are closed before the
		// box, and TruncateLeft keeps those before it, so the rest of the row
		// is styled as it was.
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

// compareNames orders two repos by name as shown, which under the root-path
// style is the path from the root, then by folder as shown. Case is ignored,
// so Backend and backend sit together; the full path breaks any tie, so the
// order never depends on the order the repos arrived in.
func (m Model) compareNames(a, b repo) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(m.name(a)), strings.ToLower(m.name(b))),
		cmp.Compare(strings.ToLower(m.shorten(filepath.Dir(a.path))), strings.ToLower(m.shorten(filepath.Dir(b.path)))),
		cmp.Compare(a.path, b.path),
	)
}

// listHeight is the number of screen lines left for the list between the
// header, the footer and their rules.
func (m Model) listHeight() int {
	return max(m.height-4, 0)
}

// pageHeight is the number of rows of the list on screen at a time: the
// list's height, less the headings line, which shows only when there are
// rows and room for one under it.
func (m Model) pageHeight() int {
	if len(m.repos) > 0 && m.listHeight() > 1 {
		return m.listHeight() - 1
	}
	return m.listHeight()
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
