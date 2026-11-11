package ui_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// sweepTree makes repos with long and wide names under ~/code, ~/dotfiles
// listed directly, and a config with a missing path, a target that opens and
// one that fails; extra settings go before the targets. With the default
// settings, all four columns fit 80 cells exactly.
func sweepTree(e *env, extra string) {
	backend := e.repo("code/work/unlimited/backend")                // the widest folder: 21 cells
	e.git(backend, "checkout", "-q", "-b", "fix/a-long-branch-116") // 21 cells
	e.repo("code/work/mono/日本語のリポジトリ")
	e.repo("code/a-repo-with-a-long-name") // the widest name: 23 cells
	e.repo("dotfiles")
	e.openedBefore(map[string]time.Duration{backend: 2 * time.Hour})
	broken := e.script("broken", "echo 'something went wrong in a rather long line of output'\nexit 1\n")
	e.config(extra + `
paths = ["~/code", "~/dotfiles", "~/a-folder-that-does-not-exist"]

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"

[[targets]]
name = "Broken"
key = "b"
command = "` + broken + ` {path}"
`)
}

func TestNothingIsDrawnWiderOrTallerThanTheTerminalInAnyState(t *testing.T) {
	states := []struct {
		name  string
		extra string // settings for the config
		setup func(e *env) *driver
		// again are keys pressed after each resize, to bring back a menu or the
		// help that a screen too small for it closed.
		again []string
	}{
		{name: "refreshing with a path warning", setup: func(e *env) *driver {
			d := e.start()
			d.hold = true
			d.press("r")
			return d
		}},
		{name: "search", setup: func(e *env) *driver {
			d := e.start()
			d.press("/")
			d.typeText("back")
			return d
		}},
		{name: "search with no matches", setup: func(e *env) *driver {
			d := e.start()
			d.press("/")
			d.typeText("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
			return d
		}},
		{name: "menu", setup: func(e *env) *driver {
			d := e.start()
			d.press("enter")
			return d
		}, again: []string{"esc", "enter"}},
		{name: "dialog", setup: func(e *env) *driver {
			d := e.start()
			d.press("b")
			return d
		}},
		{name: "help", setup: func(e *env) *driver {
			d := e.start()
			d.press("?")
			return d
		}, again: []string{"esc", "?"}},
		{name: "help scrolled to its end", setup: func(e *env) *driver {
			d := e.start()
			d.press("?")
			d.press(slices.Repeat([]string{"j"}, 40)...)
			return d
		}, again: append([]string{"esc", "?"}, slices.Repeat([]string{"j"}, 40)...)},
		{name: "opened", setup: func(e *env) *driver {
			d := e.start()
			d.press("z")
			return d
		}},
		{name: "root-path, columns reversed", extra: "row_style = \"root-path\"\ncolumns = [\"last_opened\", \"branch\", \"name\"]\n", setup: (*env).start},
		{name: "name", extra: "row_style = \"name\"\n", setup: (*env).start},
		{name: "scanning", setup: func(e *env) *driver {
			d := e.launch()
			d.next() // no cache yet
			return d
		}},
	}
	e := newEnv(t)
	sweepTree(e, "")
	if screen := e.start().screen(); !hasExactRow(screen, "backend ~/code/work/unlimited fix/a-long-branch-116 2h ago") {
		t.Fatalf("at 80 cells the sweep's rows do not show all four columns:\n%s", screen)
	}
	for _, color := range []bool{false, true} {
		for _, s := range states {
			t.Run(fmt.Sprintf("%s, colour %v", s.name, color), func(t *testing.T) {
				e := newEnv(t)
				e.color = color
				sweepTree(e, s.extra)
				sweep(t, s.setup(e), color, s.again)
			})
		}
		t.Run(fmt.Sprintf("no repos found, colour %v", color), func(t *testing.T) {
			e := newEnv(t)
			e.color = color
			e.config(`paths = ["~/a-folder-with-a-rather-long-name", "~/another-folder-with-a-long-name"]`)
			sweep(t, e.start(), color, nil)
		})
	}
}

// sweep resizes d to every width up to 100 (with colour off, to widths 0 to
// 3 and every tenth) and a range of heights, pressing again after each resize, and fails the test at the first screen wider or
// taller than the terminal, or with escape codes it should not have.
func sweep(t *testing.T, d *driver, color bool, again []string) {
	t.Helper()
	for w := 0; w <= 100; w++ {
		if !color && w > 3 && w%10 != 0 { // colour off has only to show it draws no escape codes
			continue
		}
		for _, h := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10, 24} {
			d.send(tea.WindowSizeMsg{Width: w, Height: h})
			d.press(again...)
			checkScreen(t, d.screen(), w, h, color)
		}
	}
}

// checkScreen fails the test if screen has more lines than height, a line
// wider than width, or, with colour off, any escape code at all and, with
// colour on, any but the palette's colours and attributes.
func checkScreen(t *testing.T, screen string, width, height int, color bool) {
	t.Helper()
	lines := strings.Split(screen, "\n")
	if screen == "" {
		lines = nil
	}
	if len(lines) > height {
		t.Fatalf("%dx%d: screen has %d lines:\n%s", width, height, len(lines), screen)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("%dx%d: line %d is %d cells wide: %q", width, height, i+1, w, line)
		}
	}
	if !color {
		if strings.Contains(screen, "\x1b") {
			t.Fatalf("%dx%d: escape codes with colour off: %q", width, height, screen)
		}
		return
	}
	for _, seq := range sgr.FindAllString(screen, -1) {
		for _, p := range strings.Split(seq[2:len(seq)-1], ";") {
			if !paletteParam(p) {
				t.Fatalf("%dx%d: %q is not a palette colour or attribute", width, height, seq)
			}
		}
	}
	if rest := sgr.ReplaceAllString(screen, ""); strings.Contains(rest, "\x1b") {
		t.Fatalf("%dx%d: escape codes other than colours and attributes: %q", width, height, rest)
	}
}

// paletteParam reports whether p, a parameter of a colour escape code, is a
// reset, an attribute (bold, dim, underline, reverse) or its end, or one of
// the 16 palette colours or the default colour, never 256 colours or RGB.
func paletteParam(p string) bool {
	switch p {
	case "", "0", "1", "2", "4", "7", "22", "24", "27", "39", "49":
		return true
	}
	var n int
	if _, err := fmt.Sscanf(p, "%d", &n); err != nil || fmt.Sprint(n) != p {
		return false
	}
	return 30 <= n && n <= 37 || 40 <= n && n <= 47 || 90 <= n && n <= 97 || 100 <= n && n <= 107
}

// Every value here is under the 12 cells a column shrinks to, so a column
// drops, for every row, as soon as the rows with it would be wider than the
// terminal; once only the name is left, a name too long is cut.
func TestColumnsDropInAFixedOrderAsTheTerminalNarrowsAndComeBackAsItWidens(t *testing.T) {
	for _, c := range []struct {
		columns string
		// alpha's row and b's row with all four, three, two and one column,
		// then with the name cut
		alpha, b [5]string
	}{
		{`["name", "folder", "branch", "last_opened"]`,
			[5]string{"alpha ~/code main 1h ago", "alpha ~/code main", "alpha ~/code", "alpha", "alp…"},
			[5]string{"b ~/code main -", "b ~/code main", "b ~/code", "b", "b"}},
		{`["last_opened", "branch", "folder", "name"]`,
			[5]string{"1h ago main ~/code alpha", "main ~/code alpha", "~/code alpha", "alpha", "alp…"},
			[5]string{"- main ~/code b", "main ~/code b", "~/code b", "b", "b"}},
	} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.repo("code/b") // short enough for every column at every width tested
		e.openedBefore(map[string]time.Duration{alpha: time.Hour})
		e.config("columns = " + c.columns)
		d := e.start()

		// The marker takes 3 cells and two columns are 2 apart, so the row is
		// 30 cells with all four columns, 22 with three, 16 with two and 8
		// with the name alone.
		for _, s := range []struct{ width, want int }{
			{80, 0}, {30, 0}, {29, 1}, {22, 1}, {21, 2}, {16, 2}, {15, 3}, {8, 3}, {7, 4}, {80, 0},
		} {
			d.send(tea.WindowSizeMsg{Width: s.width, Height: 24})
			screen := d.screen()
			for _, want := range []string{c.alpha[s.want], c.b[s.want]} {
				if !hasExactRow(screen, want) {
					t.Errorf("columns = %s, width %d: want the row %q:\n%s", c.columns, s.width, want, screen)
				}
			}
		}
	}
}

func TestANameTooLongIsCutWithAnEllipsisAndNeverSplitsAWideCharacter(t *testing.T) {
	e := newEnv(t)
	e.repo("code/日本語日本語") // 12 cells
	e.config(`columns = ["name", "branch"]`)
	d := e.start()

	for _, c := range []struct {
		width int
		want  string
	}{
		{15, " > 日本語日本語"},
		{14, " > 日本語日本…"},
		{13, " > 日本語日…"}, // the next character would be cut in half, so it goes whole
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		if got := strings.Split(d.screen(), "\n")[2]; got != c.want {
			t.Errorf("width %d: row is %q, want %q", c.width, got, c.want)
		}
	}
}

func TestANarrowHeaderGivesWayWarningsFirstThenTheSortThenTheSpinner(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/nope"]`)
	d := e.start()
	d.hold = true
	d.press("r") // the refresh runs until released

	// The count takes 9 cells, the warning 18, the sort 20 and the spinner 15.
	for _, c := range []struct {
		width int
		want  string
	}{
		{62, " repos  1  ~/nope not found  sort: last opened   ⠋ refreshing "},
		{50, " repos  1  ~/n…  sort: last opened   ⠋ refreshing "},
		{47, " repos  1  …  sort: last opened   ⠋ refreshing "},
		{46, " repos  1    sort: last opened   ⠋ refreshing "}, // too little room to show a warning was cut
		{44, " repos  1  sort: last opened   ⠋ refreshing "},
		{43, " repos  1" + strings.Repeat(" ", 19) + "  ⠋ refreshing "},
		{24, " repos  1  ⠋ refreshing "},
		{23, " repos  1" + strings.Repeat(" ", 14)},
		{9, " repos  1"},
		{5, " rep…"},
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		if got := strings.Split(d.screen(), "\n")[0]; got != c.want {
			t.Errorf("width %d: header is %q, want %q", c.width, got, c.want)
		}
	}

	d.press("/")
	d.typeText("alpha")
	d.send(tea.WindowSizeMsg{Width: 10, Height: 24})
	if got, want := strings.Split(d.screen(), "\n")[0], " /alpha   "; got != want {
		t.Errorf("searching: header is %q, want the query kept over the spinner, %q", got, want)
	}
}

func TestANarrowFooterDropsWholeItemsFromTheEndAndEndsWithAnEllipsis(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	for _, c := range []struct {
		width int
		want  string
	}{
		{69, " enter open | / search | s sort | r refresh | ? help | q quit | z Zed"},
		{68, " enter open | / search | s sort | r refresh | ? help | q quit | …"},
		{65, " enter open | / search | s sort | r refresh | ? help | q quit | …"},
		{64, " enter open | / search | s sort | r refresh | ? help | …"},
		{15, " enter open | …"},
		{14, " …"},
		{2, " …"},
		{1, " "},
		{0, ""},
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		lines := strings.Split(d.screen(), "\n")
		if got := lines[len(lines)-1]; got != c.want {
			t.Errorf("width %d: footer is %q, want %q", c.width, got, c.want)
		}
	}
}

func TestWithColourTheFootersKeysAreBoldAndItsSeparatorsDimmed(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	d := e.start()

	for _, c := range []struct {
		width int
		want  string
	}{
		{80, " [enter] open{ | }[/] search{ | }[s] sort{ | }[r] refresh{ | }[?] help{ | }[q] quit{ | }[z] Zed"},
		{68, " [enter] open{ | }[/] search{ | }[s] sort{ | }[r] refresh{ | }[?] help{ | }[q] quit{ | }…"},
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		lines := strings.Split(d.screen(), "\n")
		if got := styled(lines[len(lines)-1]); got != c.want {
			t.Errorf("width %d: footer reads %q, want %q", c.width, got, c.want)
		}
	}
}

func TestWithColourTheSelectionIsMarkedAndTheRulesAndBordersAreDimmed(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 30, Height: 8})

	lines := strings.Split(d.screen(), "\n")
	rule := " {" + strings.Repeat("─", 28) + "} "
	if got := styled(lines[1]); got != rule {
		t.Errorf("top rule reads %q, want it dimmed: %q", got, rule)
	}
	if got := styled(lines[6]); got != rule {
		t.Errorf("bottom rule reads %q, want it dimmed: %q", got, rule)
	}
	if got, want := styledRows(d), []string{"[>] alpha {~/code} main -", "beta {~/code} main -"}; !slices.Equal(got, want) {
		t.Errorf("rows read %q, want %q", got, want)
	}
	if want := " \x1b[1m>\x1b[22m alpha"; !strings.HasPrefix(lines[2], want) { // bold, in the terminal's own text colour
		t.Errorf("selected row is %q, want it to start with %q", lines[2], want)
	}

	d.press("enter")
	var box []string
	for _, line := range strings.Split(d.screen(), "\n")[2:6] {
		s := styled(line)
		i, j := strings.IndexAny(s, "╭│╰"), strings.LastIndexAny(s, "╮│╯")+len("│")
		if i < 0 {
			continue
		}
		if i > 0 && s[i-1] == '{' { // the box from its first border to its last, with their style
			i--
		}
		if j < len(s) && s[j] == '}' {
			j++
		}
		box = append(box, strings.Join(strings.Fields(s[i:j]), " "))
	}
	if want := []string{"{╭─} Open alpha {─╮}", "{│} [>] z Zed {│}", "{╰──────────────╯}"}; !slices.Equal(box, want) {
		t.Errorf("menu reads %q, want %q", box, want)
	}
}

func TestWithColourAMenuOverDimmedFoldersKeepsEachStyleOnItsOwnSide(t *testing.T) {
	e := newEnv(t)
	e.color = true
	for _, name := range []string{"repo1", "repo2", "repo3"} {
		e.repo("code/a-folder-name-long-enough-to-run-under-the-menu/" + name)
	}
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 8}) // the menu covers the three rows, in the middle of their folders

	d.press("enter")

	edges := regexp.MustCompile(`^ .*[^ ]\}\{[╭│╰].*[╮│╯]\}\{[^ ]+\}  main  -$`)
	lines := strings.Split(d.screen(), "\n")[2:5]
	for i, line := range lines {
		if s := styled(line); !edges.MatchString(s) {
			t.Errorf("line %d reads %q, want the folder's dimming to end before the box and start again after it", i+3, s)
		}
	}
}

// Under root-path, a path too long gives way from its start, so the repo's
// own name stays whole; only a name that does not fit alone is cut at its end.
func TestUnderRootPathAPathTooLongIsCutFromItsStartAndKeepsTheName(t *testing.T) {
	e := newEnv(t)
	e.repo("code/clients/acme/services/backend")
	e.repo("code/clients/acme/services/frontend")
	e.config("row_style = \"root-path\"\ncolumns = [\"name\", \"branch\"]")
	d := e.start()

	for _, c := range []struct {
		width int
		want  string
	}{
		{32, " > clients/acme/services/backend"},
		{21, " > …/services/backend"},
		{10, " > backend"},
		{9, " > backe…"},
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		if got := strings.Split(d.screen(), "\n")[2]; got != c.want {
			t.Errorf("width %d: row is %q, want %q", c.width, got, c.want)
		}
	}

	e.color = true
	d = e.start()
	d.send(tea.WindowSizeMsg{Width: 21, Height: 24})
	d.press("/")
	d.typeText("services back")
	if got, want := styledRows(d), []string{"[>] {…/[s][e][r][v][i][c][e][s]/}[b][a][c][k]end"}; !slices.Equal(got, want) {
		t.Errorf("with colour: rows read %q, want the dimming and the highlights kept: %q", got, want)
	}

	e = newEnv(t)
	e.repo("code/日本/backend")
	e.config(`row_style = "root-path"`)
	d = e.start()
	d.send(tea.WindowSizeMsg{Width: 13, Height: 24}) // the cut falls in the middle of 本, which goes whole
	if got, want := strings.Split(d.screen(), "\n")[2], " > …/backend"; got != want {
		t.Errorf("wide path: row is %q, want %q", got, want)
	}
}

func TestALongBranchIsCutRatherThanDroppingAColumn(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	branch := "feature/aycid-login-with-refresh-tokens-and-a-much-longer-tail" // 62 cells
	e.git(alpha, "checkout", "-q", "-b", branch)
	e.openedBefore(map[string]time.Duration{alpha: time.Hour})
	d := e.start() // 80 wide: the full row would be 88

	if want := "alpha ~/code " + branch[:53] + "… 1h ago"; !hasExactRow(d.screen(), want) {
		t.Errorf("want the row %q:\n%s", want, d.screen())
	}
}

// The folder and branch columns shrink, widest first, to 12 cells before any
// column drops.
func TestTheFolderAndBranchShrinkBeforeTheFirstColumnDrops(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/a-long-folder-name-here/alpha")           // ~/code/a-long-folder-name-here: 30 cells
	e.git(alpha, "checkout", "-q", "-b", "a-long-branch-name-here") // 23 cells
	e.openedBefore(map[string]time.Duration{alpha: time.Hour})
	d := e.start()

	// The full row is 73 cells, and 44 with both columns at 12.
	for _, c := range []struct {
		width int
		want  string
	}{
		{73, "alpha ~/code/a-long-folder-name-here a-long-branch-name-here 1h ago"},
		{66, "alpha ~/code/a-long-folder-n… a-long-branch-name-here 1h ago"}, // the wider one first
		{60, "alpha ~/code/a-long-folde… a-long-branch-name-… 1h ago"},
		{44, "alpha ~/code/a-lo… a-long-bran… 1h ago"},
		{42, "alpha ~/code/a-long-… a-long-branch-…"}, // last opened dropped, so the rest grows back
	} {
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		if screen := d.screen(); !hasExactRow(screen, c.want) {
			t.Errorf("width %d: want the row %q:\n%s", c.width, c.want, screen)
		}
	}
}

func TestASearchLineTooLongKeepsTheEndOfTheQuery(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 12, Height: 24})

	d.press("/")
	d.typeText("abcdefghijklmnopqrstuvwxyz")

	if got, want := strings.Split(d.screen(), "\n")[0], " /…rstuvwxyz"; got != want {
		t.Errorf("header is %q, want the start of the query cut: %q", got, want)
	}
}
