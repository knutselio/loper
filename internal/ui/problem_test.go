package ui_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const tooSmall = "! Window too small for the dialog"

// redInBar reports whether a line of the screen draws text in red inside the
// reverse-video bar.
func redInBar(screen string) bool {
	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	for _, line := range strings.Split(screen, "\n") {
		var reverse, red bool
		at := 0
		for _, m := range sgr.FindAllStringSubmatchIndex(line, -1) {
			if reverse && red && strings.TrimSpace(line[at:m[0]]) != "" {
				return true
			}
			for _, p := range strings.Split(line[m[2]:m[3]], ";") {
				switch p {
				case "", "0":
					reverse, red = false, false
				case "7":
					reverse = true
				case "27":
					reverse = false
				case "31":
					red = true
				case "39":
					red = false
				}
			}
			at = m[1]
		}
		if reverse && red && strings.TrimSpace(line[at:]) != "" {
			return true
		}
	}
	return false
}

func TestTheDialogsProblemIsRedWithColourAndKeepsItsMarkWithout(t *testing.T) {
	const reason = `target command "tab" has no {path}`
	for _, c := range []struct {
		color bool
		want  string
	}{
		{true, "\x1b[31m! " + reason + "\x1b[39m"},
		{false, "! " + reason},
	} {
		e := newEnv(t)
		e.color, e.noColor = c.color, !c.color
		e.repo("code/alpha")
		d := e.start()
		d.fill("Tab", "t", "tab")
		d.press("enter", "enter")

		screen := d.screen()
		if !strings.Contains(screen, c.want) {
			t.Errorf("colour %v: want the problem %q:\n%q", c.color, c.want, screen)
		}
		if got := strings.Contains(screen, "\x1b[31m") || strings.Contains(screen, "\x1b[39m"); got != c.color {
			t.Errorf("colour %v: red drawn is %v:\n%q", c.color, got, screen)
		}
		if redInBar(screen) {
			t.Errorf("colour %v: red inside the selection bar:\n%q", c.color, screen)
		}
	}
}

func TestAProblemTooLongForTheDialogWrapsRedWithOneMark(t *testing.T) {
	const reason = `target command "tab" has no {path}`
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 30, Height: 24})
	d.fill("Tab", "t", "tab")
	d.press("enter", "enter")

	var text []string
	marks := 0
	for _, line := range strings.Split(d.screen(), "\n") {
		if !strings.Contains(line, "\x1b[31m") {
			continue
		}
		row := line[strings.Index(line, "\x1b[31m"):]
		end := strings.Index(row, "\x1b[39m")
		if end < 0 {
			t.Fatalf("a red row is not closed in the dialog: %q", line)
		}
		row = strings.TrimPrefix(row[:end], "\x1b[31m")
		if strings.HasPrefix(row, "! ") {
			marks++
		} else if !strings.HasPrefix(row, "  ") {
			t.Errorf("a wrapped row of the problem is not indented under its text: %q", row)
		}
		text = append(text, strings.TrimSpace(strings.TrimPrefix(row, "! ")))
	}
	if got := strings.Join(text, " "); len(text) < 2 || marks != 1 || got != reason {
		t.Errorf("problem reads %q in %d rows with %d marks, want %q over several rows with one mark", got, len(text), marks, reason)
	}
}

func TestATooSmallWindowIsAProblemTopRightAndEveryRowStaysARepo(t *testing.T) {
	for _, c := range []struct {
		color bool
		want  string
	}{
		{true, " \x1b[31m" + tooSmall + "\x1b[39m "},
		{false, " " + tooSmall + " "},
	} {
		e := newEnv(t)
		e.color, e.noColor = c.color, !c.color
		var repos []string
		for i := range 8 {
			name := fmt.Sprintf("repo%d", i)
			e.repo("code/" + name)
			repos = append(repos, name+" ~/code main -")
		}
		d := e.start()
		d.press("j", "j") // the selection on the middle row
		d.press("a")

		for _, height := range []int{5, 8, 10} {
			d.send(tea.WindowSizeMsg{Width: 80, Height: height})
			screen := d.screen()
			if top := strings.SplitN(screen, "\n", 2)[0]; !strings.HasSuffix(top, c.want) {
				t.Errorf("colour %v at 80x%d: top row is %q, want it to end with %q", c.color, height, top, c.want)
			}
			for _, row := range rows(d) {
				if plain := strings.TrimPrefix(strings.Join(strings.Fields(ansi.Strip(row)), " "), "> "); !slices.Contains(repos, plain) {
					t.Errorf("colour %v at 80x%d: the row %q is not a repo:\n%s", c.color, height, row, screen)
				}
			}
			if redInBar(screen) {
				t.Errorf("colour %v at 80x%d: red inside the selection bar:\n%q", c.color, height, screen)
			}
		}

		d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
		if got := ansi.Strip(topRight(d)); got == tooSmall || !strings.Contains(d.screen(), "Add a target") {
			t.Errorf("colour %v grown: top right reads %q, want the dialog and no problem", c.color, got)
		}
		d.send(tea.WindowSizeMsg{Width: 80, Height: 8})
		d.press("esc")
		if got := ansi.Strip(topRight(d)); got != "8 repos" {
			t.Errorf("colour %v closed: top right reads %q, want the count", c.color, got)
		}
	}
}

func TestTheSearchCountDoesNotHideATooSmallWindow(t *testing.T) {
	d, _ := editStart(newEnv(t), "[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n")
	d.press("/")
	d.typeText("al")
	d.press("enter", "e") // the edit dialog, over the menu, while searching

	d.send(tea.WindowSizeMsg{Width: 80, Height: 6})

	if got := topRight(d); got != tooSmall {
		t.Errorf("top right reads %q, want %q", got, tooSmall)
	}
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := topRight(d); got != "1 of 1" {
		t.Errorf("grown: top right reads %q, want the search count", got)
	}
}

func TestANoticeAndTheRefreshSpinnerGoBeforeATooSmallWindow(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	startQuiet(e)

	d := relaunch(e) // the refresh is held back, so its spinner shows
	d.press("a")
	d.send(tea.WindowSizeMsg{Width: 80, Height: 6})
	if got, want := topRight(d), "⠋ refreshing"; got != want {
		t.Errorf("while refreshing: top right reads %q, want %q", got, want)
	}
	d.release()
	if got := topRight(d); got != tooSmall {
		t.Errorf("after the refresh: top right reads %q, want %q", got, tooSmall)
	}

	d.press("esc")
	d.hold = true
	d.press("z", "a") // the open's result arrives with the dialog open
	d.release()
	if got, want := topRight(d), "Opened alpha in Zed"; got != want {
		t.Errorf("with a notice: top right reads %q, want %q", got, want)
	}
	for range 100 { // the notice's tick, among any others pending
		if !d.tick() {
			break
		}
	}
	if got := topRight(d); got != tooSmall {
		t.Errorf("after the notice: top right reads %q, want %q", got, tooSmall)
	}
}

func TestTheErrorDialogOverATooSmallAddDialogHidesItsProblem(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config("[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"false {path}\"\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 10}) // room for the error dialog, not the add dialog

	d.hold = true
	d.press("b", "a") // the open fails with the dialog open
	if got := topRight(d); got != tooSmall {
		t.Fatalf("before the failure: top right reads %q, want %q", got, tooSmall)
	}
	d.release()

	if screen := d.screen(); strings.Contains(screen, "Window too small") || !strings.Contains(screen, "Could not open") {
		t.Errorf("want the error dialog and no problem about the add dialog:\n%s", screen)
	}
}
