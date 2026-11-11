package ui_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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
	for _, line := range lines[2 : len(lines)-2] {
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

// recentAndAlphabetical makes three repos whose order by last opened is
// gamma, alpha, beta and by name alpha, beta, gamma.
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

func TestSSwitchesToAlphabeticalOrderAndBackAndTheHeaderShowsTheSort(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
	d := e.start()

	steps := []struct {
		keys   []string
		order  []string
		header string
	}{
		{nil, byLastOpened, "repos 3 sort: last opened"},
		{[]string{"s"}, byName, "repos 3 sort: name"},
		{[]string{"s"}, byLastOpened, "repos 3 sort: last opened"},
	}
	for _, s := range steps {
		d.press(s.keys...)
		if got := order(d); !slices.Equal(got, s.order) {
			t.Errorf("after %v: rows %q, want %q", s.keys, got, s.order)
		}
		if got := header(d); got != s.header {
			t.Errorf("after %v: header reads %q, want %q", s.keys, got, s.header)
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
	if got, want := header(d), "repos 3 sort: name"; got != want {
		t.Errorf("header reads %q, want %q", got, want)
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

	for _, key := range []string{"s", "s"} {
		d.press(key)
		if got := d.selected(); !startsWith(got, "alpha ~/code") {
			t.Errorf("after %s: selected %q, want alpha", key, got)
		}
	}
}

func TestOpeningARepoMovesItToTheTopAndKeepsItSelected(t *testing.T) {
	e := newEnv(t)
	recentAndAlphabetical(e)
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
	d := e.start()

	d.press("s", "g", "j", "z") // beta

	if got := order(d); !slices.Equal(got, byName) {
		t.Errorf("rows %q, want %q", got, byName)
	}
	if got := d.selected(); !startsWith(got, "beta ~/code main just now") {
		t.Errorf("selected %q, want beta, just opened", got)
	}
	d.press("s")
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
