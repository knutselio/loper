package ui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTheHeaderShowsTheRepoCountOnTheLeftAndTheSortOnTheRight(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.repo("code/gamma")

	lines := strings.Split(e.start().screen(), "\n") // the screen is 80 wide

	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "loper · 3 repos sort: last opened" {
		t.Errorf("header reads %q, want %q", lines[0], "loper · 3 repos sort: last opened")
	}
	if !strings.HasPrefix(lines[0], " loper · 3 repos ") || !strings.HasSuffix(lines[0], " sort: last opened ") || ansi.StringWidth(lines[0]) != 80 {
		t.Errorf("header %q does not have the count on the left and the sort on the right, inside the margin", lines[0])
	}
}

func TestTheTitleCountsOneRepoAndNoReposAndIsBoldWithColour(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	if got, want := header(e.start()), "loper · 1 repo sort: last opened"; got != want {
		t.Errorf("one repo: header reads %q, want %q", got, want)
	}

	e = newEnv(t)
	if got, want := header(e.start()), "loper · 0 repos ~/code not found sort: last opened"; got != want {
		t.Errorf("no repos: header reads %q, want %q", got, want)
	}

	e.color = true
	if got, want := strings.Split(e.start().screen(), "\n")[0], " \x1b[1mloper\x1b[22m · 0 repos"; !strings.HasPrefix(got, want) {
		t.Errorf("with colour: header is %q, want it to start with %q", got, want)
	}
}

// The only full-screen snapshot in the suite: it guards the layout of
// "Layout" in SPEC.md, a header and a footer each under or over a rule, with
// the headings and the list between them.
func TestTheScreenIsTheHeaderTheListAndTheFooterBetweenRules(t *testing.T) {
	e := newEnv(t)
	backend := e.repo("code/work/unlimited/backend")
	e.git(backend, "checkout", "-q", "-b", "fix/bug-1116")
	aycapi := e.repo("code/work/mono/aycapi")
	exa := e.repo("code/work/mono/exa-men-v2")
	e.git(exa, "checkout", "-q", "-b", "feature/aycid-login")
	e.repo("dotfiles")
	e.openedBefore(map[string]time.Duration{backend: 2 * time.Hour, aycapi: 26 * time.Hour, exa: 3 * 24 * time.Hour})
	e.config(`paths = ["~/code", "~/dotfiles"]

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"

[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("rex") + ` {path}"
`)
	d := e.start()
	d.hold = true
	d.press("r") // the refresh runs until released
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10})

	want := strings.Join([]string{
		" loper · 4 repos                               sort: last opened   ⠋ refreshing ",
		" ────────────────────────────────────────────────────────────────────────────── ",
		"   NAME        FOLDER                 BRANCH               OPENED",
		" > backend     ~/code/work/unlimited  fix/bug-1116         2h ago",
		"   aycapi      ~/code/work/mono       main                 yesterday",
		"   exa-men-v2  ~/code/work/mono       feature/aycid-login  3d ago",
		"   dotfiles    ~                      main                 -",
		"",
		" ────────────────────────────────────────────────────────────────────────────── ",
		" enter open | / search | s sort | r refresh | ? help | q quit | z Zed | …",
	}, "\n")
	if got := d.screen(); got != want {
		t.Errorf("screen reads\n%s\nwant\n%s", got, want)
	}
}

func TestGJumpsToTheTopAndCapitalGToTheBottom(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		e.repo("code/" + name)
	}
	d := e.start()

	d.press("G")
	if got := d.selected(); !startsWith(got, "gamma ~/code main") {
		t.Errorf("after G: selected %q, want the last row", got)
	}
	d.press("g")
	if got := d.selected(); !startsWith(got, "alpha ~/code main") {
		t.Errorf("after g: selected %q, want the first row", got)
	}
}

func TestCtrlDAndCtrlUMoveHalfAPageAndStopAtTheEnds(t *testing.T) {
	e := newEnv(t)
	for i := range 6 {
		e.repo(fmt.Sprintf("code/repo%d", i))
	}
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10}) // 5 list rows under the headings: half a page is 2

	steps := []struct {
		key  string
		want string
	}{
		{"ctrl+d", "repo2"},
		{"ctrl+d", "repo4"},
		{"ctrl+d", "repo5"},
		{"ctrl+u", "repo3"},
		{"ctrl+u", "repo1"},
		{"ctrl+u", "repo0"},
	}
	for _, s := range steps {
		d.press(s.key)
		if got, want := d.selected(), s.want+" ~/code main"; !startsWith(got, want) {
			t.Fatalf("after %s: selected %q, want %q", s.key, got, want)
		}
	}
}

func TestMovingPastTheLastVisibleRowScrollsTheList(t *testing.T) {
	e := newEnv(t)
	for i := range 10 {
		e.repo(fmt.Sprintf("code/repo%d", i))
	}
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 11}) // room for 6 rows under the headings

	steps := []struct {
		keys  []string
		want  string // the selected row
		first string // the top row of the list
	}{
		{[]string{"j", "j", "j", "j", "j"}, "repo5", "repo0"},
		{[]string{"j"}, "repo6", "repo1"},
		{[]string{"G"}, "repo9", "repo4"},
		{[]string{"k", "k", "k", "k", "k"}, "repo4", "repo4"},
		{[]string{"k"}, "repo3", "repo3"},
		{[]string{"ctrl+d"}, "repo6", "repo3"},
		{[]string{"g"}, "repo0", "repo0"},
	}
	for _, s := range steps {
		d.press(s.keys...)
		lines := strings.Split(d.screen(), "\n")
		if len(lines) != 11 || !strings.HasPrefix(lines[10], " enter open") {
			t.Fatalf("after %v: the list overflows the screen:\n%s", s.keys, d.screen())
		}
		if got, want := d.selected(), s.want+" ~/code main"; !startsWith(got, want) {
			t.Fatalf("after %v: selected %q, want %q", s.keys, got, want)
		}
		if !hasRow(lines[3], s.first+" ~/code main") {
			t.Fatalf("after %v: top row is %q, want %s", s.keys, lines[3], s.first)
		}
	}
}

func TestGrowingTheWindowAfterScrollingShowsEveryRepoThatFits(t *testing.T) {
	e := newEnv(t)
	for i := range 10 {
		e.repo(fmt.Sprintf("code/repo%d", i))
	}
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10})
	d.press("G")

	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	if lines := strings.Split(d.screen(), "\n"); !hasRow(lines[3], "repo0 ~/code main") {
		t.Errorf("top row is %q, want repo0 once all repos fit:\n%s", lines[3], d.screen())
	}
	if got := d.selected(); !startsWith(got, "repo9 ~/code main") {
		t.Errorf("selected %q, want repo9 still selected", got)
	}
}

func TestCtrlDAndCtrlUMoveAtLeastOneRowOnAOneRowList(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 5}) // room for 1 row

	d.press("ctrl+d")
	if got := d.selected(); !startsWith(got, "beta ~/code main") {
		t.Errorf("after ctrl+d: selected %q, want beta", got)
	}
	d.press("ctrl+u")
	if got := d.selected(); !startsWith(got, "alpha ~/code main") {
		t.Errorf("after ctrl+u: selected %q, want alpha", got)
	}
}

func TestATerminalTooShortForTheFrameGetsNoMoreLinesThanItHas(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	for _, height := range []int{1, 2, 3} {
		d.send(tea.WindowSizeMsg{Width: 80, Height: height})
		if lines := strings.Split(d.screen(), "\n"); len(lines) > height {
			t.Errorf("height %d: screen has %d lines:\n%s", height, len(lines), d.screen())
		}
	}
}

func TestShrinkingTheWindowKeepsTheSelectedRowVisible(t *testing.T) {
	e := newEnv(t)
	for i := range 10 {
		e.repo(fmt.Sprintf("code/repo%d", i))
	}
	d := e.start()
	d.press("G")

	d.send(tea.WindowSizeMsg{Width: 80, Height: 9}) // room for 4 rows under the headings

	if got := d.selected(); !startsWith(got, "repo9 ~/code main") {
		t.Errorf("selected %q, want repo9 still visible", got)
	}
}
