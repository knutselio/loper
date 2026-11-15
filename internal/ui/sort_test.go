package ui_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// openedBefore writes the history file with each repo opened that long
// before the clock's time.
func (e *env) openedBefore(ago map[string]time.Duration) {
	e.t.Helper()
	h := map[string]time.Time{}
	for path, d := range ago {
		h[path] = e.now.Add(-d)
	}
	b, err := json.Marshal(h)
	if err != nil {
		e.t.Fatal(err)
	}
	e.writeFile(filepath.Join(e.home, ".local", "state", "loper", "history.json"), string(b))
}

// order returns the name and folder of each row of the list, top to bottom.
func order(d *driver) []string {
	lines := strings.Split(d.screen(), "\n")
	var rows []string
	for _, line := range lines[3 : len(lines)-2] { // under the headings
		if words := strings.Fields(strings.TrimPrefix(line, " > ")); len(words) >= 2 {
			rows = append(rows, words[0]+" "+words[1])
		}
	}
	return rows
}

func TestByDefaultOpenedReposComeFirstMostRecentFirstThenTheRestByName(t *testing.T) {
	for _, lastFirst := range []bool{false, true} { // the scan or the history arriving first
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.repo("code/beta")
		e.repo("code/delta")
		gamma := e.repo("code/gamma")
		e.openedBefore(map[string]time.Duration{gamma: time.Hour, alpha: 3 * time.Hour})

		d := e.launch()
		if lastFirst {
			d.releaseLastFirst()
		} else {
			d.release()
		}

		want := []string{"gamma ~/code", "alpha ~/code", "beta ~/code", "delta ~/code"}
		if got := order(d); !slices.Equal(got, want) {
			t.Errorf("history first: %v: rows %q, want %q:\n%s", lastFirst, got, want, d.screen())
		}
		if got := d.selected(); !startsWith(got, "gamma ~/code") {
			t.Errorf("history first: %v: selected %q, want the top row", lastFirst, got)
		}
	}
}

func TestTwoNeverOpenedReposWithTheSameNameAreOrderedByFolder(t *testing.T) {
	e := newEnv(t)
	e.repo("B/api")
	e.repo("a/api")
	e.config(`paths = ["~/B", "~/a"]`) // folders compare ignoring case, as names do

	d := e.start()

	if got, want := order(d), []string{"api ~/a", "api ~/B"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

func TestNamesAreOrderedIgnoringCaseSoBackendAndBackendSitTogether(t *testing.T) {
	e := newEnv(t)
	e.repo("code/Zeta")
	e.repo("code/b/backend")
	e.repo("code/alpha")
	e.repo("code/a/Backend")

	d := e.start()

	want := []string{"alpha ~/code", "Backend ~/code/a", "backend ~/code/b", "Zeta ~/code"}
	if got := order(d); !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

// header returns the first line of the screen with its spacing collapsed.
func header(d *driver) string {
	return strings.Join(strings.Fields(strings.SplitN(d.screen(), "\n", 2)[0]), " ")
}

// headings returns the headings line, the third of the screen, with its
// spacing collapsed.
func headings(d *driver) string {
	return strings.Join(strings.Fields(strings.Split(d.screen(), "\n")[2]), " ")
}

// recentAndAlphabetical makes three repos whose order by last opened is
// gamma, alpha, beta and by name alpha, beta, gamma; beta was never opened.
func recentAndAlphabetical(e *env) {
	alpha := e.repo("code/alpha")
	e.repo("code/beta")
	gamma := e.repo("code/gamma")
	e.openedBefore(map[string]time.Duration{gamma: time.Hour, alpha: 2 * time.Hour})
}

var (
	byLastOpened = []string{"gamma ~/code", "alpha ~/code", "beta ~/code"}
	byName       = []string{"alpha ~/code", "beta ~/code", "gamma ~/code"}
)

func TestSCyclesThroughTheFourSortsAndTheArrowIsOnTheSortedHeading(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	d := e.start()

	steps := []struct {
		order    []string
		headings string
	}{
		{byLastOpened, "NAME FOLDER BRANCH OPENED ↓"},
		{byName, "NAME ↑ FOLDER BRANCH OPENED"},
		{[]string{"gamma ~/code", "beta ~/code", "alpha ~/code"}, "NAME ↓ FOLDER BRANCH OPENED"},
		{[]string{"alpha ~/code", "gamma ~/code", "beta ~/code"}, "NAME FOLDER BRANCH OPENED ↑"}, // beta, never opened, stays last
		{byLastOpened, "NAME FOLDER BRANCH OPENED ↓"},
	}
	for i, s := range steps {
		if i > 0 {
			d.press("s")
		}
		if got := order(d); !slices.Equal(got, s.order) {
			t.Errorf("after %d presses of s: rows %q, want %q", i, got, s.order)
		}
		if got := headings(d); got != s.headings {
			t.Errorf("after %d presses of s: headings read %q, want %q", i, got, s.headings)
		}
	}
}

func TestWithColourTheSortedHeadingIsAtFullStrengthAndTheOthersDimmed(t *testing.T) {
	e := newEnv(t)
	e.color = true
	recentAndAlphabetical(e)
	d := e.start()

	got := strings.Join(strings.Fields(styled(strings.Split(d.screen(), "\n")[2])), " ")
	if want := "{NAME} {FOLDER} {BRANCH} OPENED ↓"; got != want {
		t.Errorf("headings read %q, want %q", got, want)
	}
}

func TestNoArrowShowsWhenTheSortedColumnIsNotShown(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	e.config(`columns = ["name", "branch"]`)
	d := e.start()

	if got, want := headings(d), "NAME BRANCH"; got != want {
		t.Errorf("columns setting, sorted by last opened: headings read %q, want %q", got, want)
	}
	d.press("s")
	if got, want := headings(d), "NAME ↑ BRANCH"; got != want {
		t.Errorf("columns setting, sorted by name: headings read %q, want %q", got, want)
	}

	e = newEnv(t)
	recentAndAlphabetical(e)
	d = e.start()
	d.send(tea.WindowSizeMsg{Width: 30, Height: 24}) // too narrow for the last_opened column
	if got, want := headings(d), "NAME FOLDER BRANCH"; got != want {
		t.Errorf("narrow, sorted by last opened: headings read %q, want %q", got, want)
	}
}

func TestTheColumnsStayPutAsSGoesThroughTheSorts(t *testing.T) {
	e := newEnv(t)
	ab, cd := e.repo("code/ab"), e.repo("code/cd") // names shorter than NAME
	e.openedBefore(map[string]time.Duration{ab: time.Hour, cd: 2 * time.Hour})
	d := e.start()

	// at returns the cell where each heading starts and where alpha's row has
	// its folder and branch.
	at := func() []int {
		lines := strings.Split(d.screen(), "\n")
		cell := func(line, s string) int { return ansi.StringWidth(line[:strings.Index(line, s)]) } // the arrow is one cell, three bytes
		return []int{cell(lines[2], "FOLDER"), cell(lines[2], "BRANCH"), cell(lines[3], "~/code"), cell(lines[3], "main")}
	}
	want := at()
	for i := 1; i <= 4; i++ {
		d.press("s")
		if got := at(); !slices.Equal(got, want) {
			t.Errorf("after %d presses of s: FOLDER, BRANCH and the row's folder and branch are at %v, want %v:\n%s", i, got, want, d.screen())
		}
	}
}

func TestSortNameInTheConfigStartsInAlphabeticalOrder(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	e.config(`sort = "name"`)

	d := e.start()

	if got := order(d); !slices.Equal(got, byName) {
		t.Errorf("rows %q, want %q", got, byName)
	}
	if got, want := headings(d), "NAME ↑ FOLDER BRANCH OPENED"; got != want {
		t.Errorf("headings read %q, want %q", got, want)
	}
}

func TestASortSwitchedWithSIsNotKeptForTheNextLaunch(t *testing.T) {
	for _, configured := range []struct {
		config string
		order  []string
	}{{"", byLastOpened}, {`sort = "name"`, byName}} {
		e := newEnv(t)
		recentAndAlphabetical(e)
		e.config(configured.config)
		e.start().press("s")

		if got := order(e.start()); !slices.Equal(got, configured.order) {
			t.Errorf("config %q: relaunched with rows %q, want %q", configured.config, got, configured.order)
		}
	}
}

func TestSKeepsTheSelectedRepoSelected(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	d := e.start()
	d.press("j") // alpha, second by last opened and first by name

	for _, key := range []string{"s", "s", "s", "s"} {
		d.press(key)
		if got := d.selected(); !startsWith(got, "alpha ~/code") {
			t.Errorf("after %s: selected %q, want alpha", key, got)
		}
	}
}

func TestOpeningARepoMovesItToTheTopAndKeepsItSelected(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	e.config(zedTarget)
	d := e.start()

	d.press("G", "z") // beta, never opened

	want := []string{"beta ~/code", "gamma ~/code", "alpha ~/code"}
	if got := order(d); !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "beta ~/code main just now") {
		t.Errorf("selected %q, want beta, just opened", got)
	}
}

func TestARepoMovedToTheTopByAnOpenKeepsTheSelectionOnTheRepoChosenSince(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	e.config(zedTarget)
	d := e.start()
	d.press("G")

	d.hold = true
	d.press("z", "k") // open beta, then select alpha while it opens
	d.release()

	if got, want := order(d), []string{"beta ~/code", "gamma ~/code", "alpha ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "alpha ~/code") {
		t.Errorf("selected %q, want alpha, the repo selected while beta opened", got)
	}
}

func TestOpeningARepoUnderTheNameSortLeavesTheOrderAsItIs(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	e.config(zedTarget)
	d := e.start()

	d.press("s", "g", "j", "z") // beta

	if got := order(d); !slices.Equal(got, byName) {
		t.Errorf("rows %q, want %q", got, byName)
	}
	if got := d.selected(); !startsWith(got, "beta ~/code main just now") {
		t.Errorf("selected %q, want beta, just opened", got)
	}
	d.press("s", "s", "s")
	if got, want := order(d), []string{"beta ~/code", "gamma ~/code", "alpha ~/code"}; !slices.Equal(got, want) {
		t.Errorf("back under last opened: rows %q, want %q", got, want)
	}
}

func TestReposOpenedAtTheSameTimeAreOrderedByName(t *testing.T) {
	e := newEnv(t)
	gamma := e.repo("a/gamma") // scanned first, and first by folder or path
	alpha := e.repo("z/alpha")
	e.repo("z/beta")
	e.config(`paths = ["~/a", "~/z"]`)
	e.openedBefore(map[string]time.Duration{gamma: time.Hour, alpha: time.Hour})

	d := e.start()

	if got, want := order(d), []string{"alpha ~/z", "gamma ~/a", "beta ~/z"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}
