package ui_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestQuestionMarkShowsEveryKeyOfTheSpecWithItsAction(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 60}) // room for the whole help

	d.press("?")

	rows := boxRows(d)
	for _, want := range []string{
		// The table in "Keybindings" in SPEC.md, row by row.
		"j / k, ↓ / ↑ Move the selection",
		"g / G Jump to the top / bottom",
		"ctrl-d / ctrl-u Move half a page down / up",
		"enter Open the target menu for the selected repo",
		"a target's key Open the selected repo in that target",
		"/ Start the search",
		"s Switch the sort",
		"r Force a full refresh",
		"a Add a target to the config file",
		"? Show the help overlay",
		"q, ctrl-c Quit",
		"esc Close a menu or dialog, or clear the search",
	} {
		if !slices.Contains(rows, want) {
			t.Errorf("help has no row %q:\n%s", want, d.screen())
		}
	}
	if !strings.Contains(d.screen(), "╭─ Help ─") {
		t.Errorf("help is not titled:\n%s", d.screen())
	}
}

func TestTheHelpListsTheMenuAndSearchKeysAndTheTargetsWithTheirKeys(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	threeTargets(e)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 60})

	d.press("?")

	rows := boxRows(d)
	for _, want := range [][]string{
		{"In the target menu", "j / k, ↓ / ↑ Move the highlight", "enter Run the highlighted target", "a target's key Run that target", "esc Close the menu"},
		{"While searching", "↓ / ↑, ctrl-n / ctrl-p Move through the matches", "enter Open the target menu for the selected match", "esc Clear the search"},
		{"Targets", "z Zed", "Menu only (through the menu)", "t Terminal tab"},
	} {
		if i := slices.Index(rows, want[0]); i < 0 || !slices.Equal(rows[i:min(i+len(want), len(rows))], want) {
			t.Errorf("help has no rows %q:\n%s", want, d.screen())
		}
	}
}

func TestEscOrQuestionMarkAgainClosesTheHelpAndTheListIsBack(t *testing.T) {
	for _, key := range []string{"esc", "?"} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.repo("code/beta")
		d := e.start()
		d.press("j")
		before := d.screen()

		d.press("?", key)

		if got := d.screen(); got != before {
			t.Errorf("after ? and %s: screen reads\n%s\nwant\n%s", key, got, before)
		}
	}
}

func TestWhileTheHelpIsOpenListKeysAndTargetKeysDoNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	threeTargets(e)
	d := e.start()
	d.hold = true // a refresh would show in the header until released

	d.press("?")
	d.press("k", "up", "g", "ctrl+u", "j", "down", "G", "ctrl+d", "z", "t", "s", "r", "/", "enter") // the moves end down the list
	d.press("esc")

	if got := d.selected(); !startsWith(got, "alpha ~/code main") {
		t.Errorf("selected %q, want alpha still selected", got)
	}
	if got, want := header(d), "loper · 2 repos sort: last opened"; got != want {
		t.Errorf("header reads %q, want no search, no sort switch and no refresh: %q", got, want)
	}
	if got := boxRows(d); got != nil {
		t.Errorf("a box is open after closing the help: %q", got)
	}
	for _, name := range []string{"zed", "tab"} {
		if got := e.targetArgs(name); got != nil {
			t.Errorf("%s called with %q while the help was open", name, got)
		}
	}
}

func TestQAndCtrlCQuitFromTheHelp(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()

		d.press("?", key)

		if !d.quit {
			t.Errorf("%s did not quit from the help", key)
		}
	}
}

func TestQuestionMarkShowsNoHelpInTheMenuTheDialogOrASearch(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Broken"
key = "b"
command = "false {path}"
`)
	d := e.start()

	d.press("enter")
	menu := boxRows(d)
	d.press("?")
	if got := boxRows(d); !slices.Equal(got, menu) {
		t.Errorf("? in the menu: box reads %q, want the menu %q", got, menu)
	}

	d.press("esc", "b")
	dialog := boxRows(d)
	d.press("?")
	if got := boxRows(d); !slices.Equal(got, dialog) {
		t.Errorf("? in the dialog: box reads %q, want the dialog %q", got, dialog)
	}

	d.press("esc", "/", "?")
	if got := boxRows(d); got != nil {
		t.Errorf("? while searching: a box is open: %q", got)
	}
	if got, want := header(d), "/?"; got != want {
		t.Errorf("? while searching: header reads %q, want %q", got, want)
	}
}

func TestAHelpTallerThanTheScreenScrollsWithJKAndTheArrowsInsideItsBorders(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10}) // 6 list lines: room for 4 rows of help

	steps := []struct {
		keys  []string
		first string // the top row of the help shown
		last  string // its bottom row
	}{
		{[]string{"?"}, "j / k, ↓ / ↑ Move the selection", "enter Open the target menu for the selected repo"},
		{[]string{"j"}, "g / G Jump to the top / bottom", "a target's key Open the selected repo in that target"},
		{[]string{"down", "k"}, "g / G Jump to the top / bottom", "a target's key Open the selected repo in that target"},
		{[]string{"up", "k"}, "j / k, ↓ / ↑ Move the selection", "enter Open the target menu for the selected repo"},
		{slices.Repeat([]string{"j"}, 40), "esc, ? Close the help", "z Zed"},
		{[]string{"k"}, "j / k, ↓ / ↑ Scroll", "Targets"},
		{[]string{"esc", "?"}, "j / k, ↓ / ↑ Move the selection", "enter Open the target menu for the selected repo"},
	}
	for _, s := range steps {
		d.press(s.keys...)
		rows := boxRows(d)
		if len(rows) != 4 || rows[0] != s.first || rows[3] != s.last {
			t.Fatalf("after %v: help reads %q, want it from %q to %q:\n%s", s.keys, rows, s.first, s.last, d.screen())
		}
		screen := d.screen()
		if strings.Count(screen, "╭─ Help ─") != 1 || strings.Count(screen, "╰") != 1 {
			t.Fatalf("after %v: want both borders of the help:\n%s", s.keys, screen)
		}
	}

	d.send(tea.WindowSizeMsg{Width: 80, Height: 60})
	if rows := boxRows(d); len(rows) == 0 || rows[0] != "j / k, ↓ / ↑ Move the selection" || rows[len(rows)-1] != "z Zed" {
		t.Errorf("once the screen is tall enough: help reads %q, want all of it", rows)
	}
}

func TestAFailureArrivingWhileTheHelpIsOpenReplacesIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Broken"
key = "b"
command = "false {path}"
`)
	d := e.start()
	d.hold = true

	d.press("b", "?") // the open is still running when the help opens
	d.release()

	if rows := boxRows(d); !strings.Contains(d.screen(), "Could not open alpha in Broken") {
		t.Errorf("want the error dialog, got the box %q:\n%s", rows, d.screen())
	}
	d.press("esc")
	if got := boxRows(d); got != nil {
		t.Errorf("after esc: a box is still open: %q", got)
	}
}

func TestTheHelpDoesNotOpenOrStayOpenWithoutRoomForIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()

	d.send(tea.WindowSizeMsg{Width: 80, Height: 6}) // 2 list lines: only the borders would fit
	d.press("?")
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := boxRows(d); got != nil {
		t.Errorf("? opened the help on a screen with no room for it: %q", got)
	}

	d.press("?")
	d.send(tea.WindowSizeMsg{Width: 80, Height: 6})
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := boxRows(d); got != nil {
		t.Errorf("the help stayed open through a screen with no room for it: %q", got)
	}
	d.press("j")
	if got := d.selected(); !startsWith(got, "beta ~/code main") {
		t.Errorf("selected %q, want j to move the list once the help is closed", got)
	}
}

func TestWithColourTheHelpsKeysAreBold(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 60})

	d.press("?")

	var rows []string
	for _, line := range strings.Split(d.screen(), "\n") {
		rows = append(rows, strings.Join(strings.Fields(styled(line)), " "))
	}
	for _, want := range []string{"{│} [q, ctrl-c] Quit {│}", "{│} [z] Zed {│}"} {
		if !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, want) }) {
			t.Errorf("help has no row %q:\n%s", want, strings.Join(rows, "\n"))
		}
	}
}
