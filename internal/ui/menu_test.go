package ui_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// threeTargets is a config with two keyed targets and one without a key,
// each a fake script named after its key.
func threeTargets(e *env) {
	e.config(`
[[targets]]
name = "Zed"
key = "z"
command = "` + e.fakeTarget("zed") + ` {path}"

[[targets]]
name = "Menu only"
command = "` + e.fakeTarget("menu-only") + ` {path}"

[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("tab") + ` {path}"
`)
}

// boxRows returns the rows of the box drawn over the list, the target menu
// or the error dialog, with their spacing collapsed, or nil when none is
// shown.
func boxRows(d *driver) []string {
	var rows []string
	for _, line := range strings.Split(d.screen(), "\n") {
		first, last := strings.Index(line, "│"), strings.LastIndex(line, "│")
		if first >= 0 && last > first {
			rows = append(rows, strings.Join(strings.Fields(line[first+len("│"):last]), " "))
		}
	}
	return rows
}

func TestEnterShowsAMenuNamingEveryTargetAndItsKeyAndRunsNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	threeTargets(e)
	d := e.start()

	d.press("enter")

	if got, want := boxRows(d), []string{"> z Zed", "Menu only", "t Terminal tab"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want %q:\n%s", got, want, d.screen())
	}
	for _, name := range []string{"zed", "menu-only", "tab"} {
		if got := e.targetArgs(name); got != nil {
			t.Errorf("%s called with %q by opening the menu", name, got)
		}
	}
}

func TestJKAndTheArrowsMoveTheHighlightAndEnterRunsItOnTheSelectedRepo(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	threeTargets(e)
	d := e.start()
	d.press("j", "enter")

	steps := []struct {
		key  string
		want string // the highlighted row
	}{
		{"k", "> z Zed"}, // stops at the top
		{"j", "> Menu only"},
		{"down", "> t Terminal tab"},
		{"j", "> t Terminal tab"}, // stops at the bottom
		{"k", "> Menu only"},
		{"up", "> z Zed"},
		{"down", "> Menu only"},
	}
	for _, s := range steps {
		d.press(s.key)
		if got := boxRows(d); !slices.Contains(got, s.want) {
			t.Fatalf("after %s: menu reads %q, want the row %q highlighted", s.key, got, s.want)
		}
	}
	d.press("enter")

	if got, want := e.targetArgs("menu-only"), []string{beta}; !slices.Equal(got, want) {
		t.Errorf("menu-only called with %q, want %q", got, want)
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q, want only the highlighted target run", got)
	}
}

func TestATargetsKeyInsideTheMenuRunsThatTarget(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	threeTargets(e)
	d := e.start()

	d.press("enter", "t")

	if got, want := e.targetArgs("tab"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q, want only the target whose key was pressed", got)
	}
	if got := boxRows(d); got != nil {
		t.Errorf("menu still shown after running a target: %q", got)
	}
	if got, want := footer(d), "Opened alpha in Terminal tab"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestEscClosesTheMenuAndRunsNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	threeTargets(e)
	d := e.start()

	d.press("enter", "esc")

	if got := boxRows(d); got != nil {
		t.Errorf("menu still shown after esc: %q", got)
	}
	for _, name := range []string{"zed", "menu-only", "tab"} {
		if got := e.targetArgs(name); got != nil {
			t.Errorf("%s called with %q after esc", name, got)
		}
	}
}

func TestQAndCtrlCQuitFromInsideTheMenu(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()

		d.press("enter", key)

		if !d.quit {
			t.Errorf("%s in the menu did not quit", key)
		}
		if got := e.targetArgs("zed"); got != nil {
			t.Errorf("%s in the menu ran zed with %q", key, got)
		}
	}
}

func TestListKeysDoNotMoveTheSelectionWhileTheMenuIsOpen(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		e.repo("code/" + name)
	}
	d := e.start()

	d.press("enter", "j", "down", "G", "ctrl+d", "esc")

	if got := d.selected(); !startsWith(got, "alpha ~/code main") {
		t.Errorf("selected %q after keys in the menu, want alpha still selected", got)
	}
}

func TestAfterRunningATargetTheMenuIsClosedAndTheFooterConfirmsIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	threeTargets(e)
	d := e.start()

	d.press("enter", "j", "enter")

	if got := boxRows(d); got != nil {
		t.Errorf("menu still shown after running a target: %q", got)
	}
	if got, want := footer(d), "Opened alpha in Menu only"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestTheMenuSitsOverTheListWhichStaysVisibleAroundIt(t *testing.T) {
	e := newEnv(t)
	for i := range 20 {
		e.repo(fmt.Sprintf("code/repo%02d", i))
	}
	e.repo("code/a/folder/deep/enough/to/push/the/branch/past/the/menu/alpha")
	threeTargets(e)
	d := e.start()

	d.press("enter")
	screen := d.screen()
	lines := strings.Split(screen, "\n")

	if len(lines) != 24 {
		t.Errorf("screen has %d lines with the menu open, want 24", len(lines))
	}
	for _, row := range []string{"repos 21", "enter open z Zed t Terminal tab s sort q quit"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q with the menu open:\n%s", row, screen)
		}
	}
	if !strings.Contains(screen, "╭─ Open alpha ─") {
		t.Errorf("menu is not titled with the selected repo:\n%s", screen)
	}
	covered := 0
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line %d is %d cells wide, wider than the screen: %q", i+1, w, line)
		}
		first, last := strings.Index(line, "│"), strings.LastIndex(line, "│")
		if first < 0 {
			continue
		}
		covered++
		left, right := strings.Fields(line[:first]), strings.Fields(line[last+len("│"):])
		if len(left) == 0 || !strings.HasPrefix(left[0], "repo") || len(right) < 2 || strings.Join(right[len(right)-2:], " ") != "main -" {
			t.Errorf("line %d does not show its list row either side of the menu: %q", i+1, line)
		}
	}
	if covered != 3 {
		t.Errorf("the menu covers %d list rows, want 3:\n%s", covered, screen)
	}
}

// checkBox fails the test unless the menu on screen has both borders, a
// highlighted row, and no line of it wider than width.
func checkBox(t *testing.T, d *driver, width int) {
	t.Helper()
	var tops, bottoms int
	for i, line := range strings.Split(d.screen(), "\n") {
		tops += strings.Count(line, "╭")
		bottoms += strings.Count(line, "╰")
		if strings.ContainsAny(line, "╭│╰") && ansi.StringWidth(line) > width {
			t.Errorf("line %d is %d cells wide, wider than the screen: %q", i+1, ansi.StringWidth(line), line)
		}
	}
	if tops != 1 || bottoms != 1 {
		t.Errorf("menu has %d top and %d bottom borders, want one of each:\n%s", tops, bottoms, d.screen())
	}
	if !slices.ContainsFunc(boxRows(d), func(r string) bool { return strings.HasPrefix(r, "> ") }) {
		t.Errorf("no highlighted row is drawn:\n%s", d.screen())
	}
}

// The list itself is not cut to a narrow screen yet, so this checks only the
// lines the menu is drawn on.
func TestTheMenuFitsANarrowScreen(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/a-repository-with-a-long-name")
	threeTargets(e)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 20, Height: 8}) // 4 list lines: room for 2 targets

	steps := []struct {
		key  string
		want []string // the menu rows
	}{
		{"enter", []string{"> z Zed", "Menu only"}},
		{"j", []string{"z Zed", "> Menu only"}},
		{"j", []string{"Menu only", "> t Terminal ta…"}},
	}
	for _, s := range steps {
		d.press(s.key)
		if got := boxRows(d); !slices.Equal(got, s.want) {
			t.Errorf("after %s: menu reads %q, want %q:\n%s", s.key, got, s.want, d.screen())
		}
		checkBox(t, d, 20)
	}
	d.press("enter")
	if got, want := e.targetArgs("tab"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestAMenuTallerThanTheListScrollsToKeepTheHighlightInView(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	config := ""
	for i := 1; i <= 6; i++ {
		config += fmt.Sprintf("[[targets]]\nname = \"Target %d\"\ncommand = \"%s {path}\"\n", i, e.fakeTarget(fmt.Sprint("target", i)))
	}
	e.config(config)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 40, Height: 8}) // 4 list lines: room for 2 targets

	steps := []struct {
		keys []string
		want []string // the menu rows
	}{
		{[]string{"enter"}, []string{"> Target 1", "Target 2"}},
		{[]string{"j", "j", "j", "j", "j"}, []string{"Target 5", "> Target 6"}},
		{[]string{"k"}, []string{"> Target 5", "Target 6"}},
		{[]string{"k"}, []string{"> Target 4", "Target 5"}},
	}
	for _, s := range steps {
		d.press(s.keys...)
		if got := boxRows(d); !slices.Equal(got, s.want) {
			t.Errorf("after %v: menu reads %q, want %q:\n%s", s.keys, got, s.want, d.screen())
		}
		checkBox(t, d, 40)
	}
	d.press("j", "j", "enter")
	if got, want := e.targetArgs("target6"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("target6 called with %q, want %q", got, want)
	}
}

func TestTheMenuDoesNotOpenOrStayOpenWithoutRoomForIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()

	d.send(tea.WindowSizeMsg{Width: 40, Height: 4}) // no list lines
	d.press("enter")
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := boxRows(d); got != nil {
		t.Errorf("enter opened the menu on a screen with no room for it: %q", got)
	}

	d.press("enter")
	d.send(tea.WindowSizeMsg{Width: 40, Height: 4})
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := boxRows(d); got != nil {
		t.Errorf("the menu stayed open through a screen with no room for it: %q", got)
	}
	d.press("j")
	if got := d.selected(); !startsWith(got, "beta ~/code main") {
		t.Errorf("selected %q, want j to move the list once the menu is closed", got)
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q", got)
	}
}

func TestWideCharactersUnderTheMenuEdgesKeepTheRestOfTheirRowInPlace(t *testing.T) {
	e := newEnv(t)
	e.repo("code/0")                 // first, so the menu's title is short
	wide := strings.Repeat("日本", 14) // 56 cells, reaching past both edges of the menu
	for i := range 20 {
		// Every other name starts its wide characters one cell later, so
		// whichever cell an edge of the menu falls on, some row has a wide
		// character across it.
		e.repo(fmt.Sprintf("code/1%02d%s%s", i, strings.Repeat("x", i%2), wide))
	}
	d := e.start()

	d.press("enter")
	lines := strings.Split(d.screen(), "\n")

	folderCell := func(line string) int { return ansi.StringWidth(line[:strings.Index(line, "~/code")]) }
	want := folderCell(lines[2]) // the row for 0, which the menu does not cover
	covered := 0
	for i, line := range lines {
		if !strings.ContainsAny(line, "╭│╰") {
			continue
		}
		covered++
		if got := folderCell(line); got != want {
			t.Errorf("line %d has its folder at cell %d, want %d: %q", i+1, got, want, line)
		}
	}
	if covered == 0 {
		t.Fatalf("the menu covers no rows:\n%s", d.screen())
	}
}
