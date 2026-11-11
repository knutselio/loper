package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTheHeaderShowsTheRepoCount(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.repo("code/gamma")

	lines := strings.Split(e.start().screen(), "\n")

	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "repos 3" {
		t.Errorf("header reads %q, want %q", lines[0], "repos 3")
	}
}

func TestTheListSitsBetweenRulesWithTheFooterOnTheLastLine(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")

	lines := strings.Split(e.start().screen(), "\n") // the screen is 80x24

	if len(lines) != 24 {
		t.Fatalf("screen has %d lines, want 24:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	rule := " " + strings.Repeat("─", 78) + " " // a one-cell margin either side
	if lines[1] != rule {
		t.Errorf("line 2 is %q, want a rule inside the margin", lines[1])
	}
	if lines[22] != rule {
		t.Errorf("line 23 is %q, want a rule inside the margin", lines[22])
	}
	if !strings.HasPrefix(lines[23], " enter open") || strings.Join(strings.Fields(lines[23]), " ") != "enter open z Zed q quit" {
		t.Errorf("footer reads %q, want %q after a one-cell margin", lines[23], "enter open z Zed q quit")
	}
	for i, want := range map[int]string{0: " repos", 2: " > alpha", 3: "   beta"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d is %q, want it to start with %q", i+1, lines[i], want)
		}
	}
	if !hasRow(lines[2], "alpha ~/code main") || !hasRow(lines[3], "beta ~/code main") {
		t.Errorf("the list does not start under the rule:\n%s", strings.Join(lines, "\n"))
	}
}

func TestGJumpsToTheTopAndCapitalGToTheBottom(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		e.repo("code/" + name)
	}
	d := e.start()

	d.press("G")
	if got := d.selected(); got != "gamma ~/code main" {
		t.Errorf("after G: selected %q, want the last row", got)
	}
	d.press("g")
	if got := d.selected(); got != "alpha ~/code main" {
		t.Errorf("after g: selected %q, want the first row", got)
	}
}

func TestCtrlDAndCtrlUMoveHalfAPageAndStopAtTheEnds(t *testing.T) {
	e := newEnv(t)
	for i := range 6 {
		e.repo(fmt.Sprintf("code/repo%d", i))
	}
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10}) // 6 list rows: half a page is 3

	steps := []struct {
		key  string
		want string
	}{
		{"ctrl+d", "repo3"},
		{"ctrl+d", "repo5"},
		{"ctrl+u", "repo2"},
		{"ctrl+u", "repo0"},
	}
	for _, s := range steps {
		d.press(s.key)
		if got, want := d.selected(), s.want+" ~/code main"; got != want {
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
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10}) // room for 6 rows

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
		if len(lines) != 10 || !strings.HasPrefix(lines[9], " enter open") {
			t.Fatalf("after %v: the list overflows the screen:\n%s", s.keys, d.screen())
		}
		if got, want := d.selected(), s.want+" ~/code main"; got != want {
			t.Fatalf("after %v: selected %q, want %q", s.keys, got, want)
		}
		if !hasRow(lines[2], s.first+" ~/code main") {
			t.Fatalf("after %v: top row is %q, want %s", s.keys, lines[2], s.first)
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

	if lines := strings.Split(d.screen(), "\n"); !hasRow(lines[2], "repo0 ~/code main") {
		t.Errorf("top row is %q, want repo0 once all repos fit:\n%s", lines[2], d.screen())
	}
	if got := d.selected(); got != "repo9 ~/code main" {
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
	if got := d.selected(); got != "beta ~/code main" {
		t.Errorf("after ctrl+d: selected %q, want beta", got)
	}
	d.press("ctrl+u")
	if got := d.selected(); got != "alpha ~/code main" {
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

	d.send(tea.WindowSizeMsg{Width: 80, Height: 8}) // room for 4 rows

	if got := d.selected(); got != "repo9 ~/code main" {
		t.Errorf("selected %q, want repo9 still visible", got)
	}
}
