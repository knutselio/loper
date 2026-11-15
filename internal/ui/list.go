package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/matcher"
)

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
		rows[i] = matcher.Row{Name: m.name(r), Folder: safe(m.shorten(filepath.Dir(r.path)))}
	}
	hits := matcher.Find(m.query, rows)
	repos := make([]repo, len(hits))
	for i, h := range hits {
		repos[i] = m.all[h.Row]
	}
	return repos, hits
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
