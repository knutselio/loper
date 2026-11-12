package ui_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// typeText presses each character of text in turn.
func (d *driver) typeText(text string) {
	d.t.Helper()
	for _, r := range text {
		d.press(string(r))
	}
}

func TestSlashShowsTheSearchLineAndOnlyMatchingRowsWithTheTopOneSelected(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.repo("code/gamma")
	d := e.start()
	d.press("j", "j") // gamma

	d.press("/")
	if got, want := header(d), "/"; got != want {
		t.Errorf("after /: header reads %q, want %q", got, want)
	}
	if got, want := order(d), []string{"alpha ~/code", "beta ~/code", "gamma ~/code"}; !slices.Equal(got, want) {
		t.Errorf("after /: rows %q, want them all", got)
	}
	if got := d.selected(); !startsWith(got, "gamma") {
		t.Errorf("after /: selected %q, want gamma still, as nothing is typed yet", got)
	}
	d.typeText("mm") // only gamma has two m
	if got, want := header(d), "/mm"; got != want {
		t.Errorf("header reads %q, want %q", got, want)
	}
	if got, want := order(d), []string{"gamma ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}

	d.press("backspace")
	if got, want := header(d), "/m"; got != want {
		t.Errorf("after backspace: header reads %q, want %q", got, want)
	}
	d.press("backspace", "a")
	if got, want := order(d), []string{"alpha ~/code", "beta ~/code", "gamma ~/code"}; !slices.Equal(got, want) {
		t.Errorf("searching a: rows %q, want them all", got)
	}
	if got := d.selected(); !startsWith(got, "alpha") {
		t.Errorf("searching a: selected %q, want the top match", got)
	}
}

func TestARepoIsFoundByPartOfItsNameItsFolderOrItsBranch(t *testing.T) {
	e := newEnv(t)
	e.repo("code/backend")
	e.repo("work/web")
	feature := e.repo("code/site")
	e.git(feature, "checkout", "-q", "-b", "fix/login")
	e.config(`paths = ["~/code", "~/work"]`)

	for _, c := range []struct{ query, want string }{
		{"bkd", "backend ~/code main"},
		{"wrk", "web ~/work main"},
		{"login", "site ~/code fix/login"},
	} {
		d := e.start()
		d.press("/")
		d.typeText(c.query)
		if got := order(d); len(got) != 1 || !hasRow(d.screen(), c.want) {
			t.Errorf("%q: rows %q, want only %q:\n%s", c.query, got, c.want, d.screen())
		}
	}
}

func TestANameMatchRanksAboveAFolderOrBranchMatch(t *testing.T) {
	e := newEnv(t)
	e.repo("api/main") // the folder matches exactly
	web := e.repo("code/web")
	e.git(web, "checkout", "-q", "-b", "api") // the branch matches exactly
	e.repo("code/a-p-i")                      // the name matches, scattered
	e.config(`paths = ["~/api", "~/code"]`)
	d := e.start()

	d.press("/")
	d.typeText("api")

	if got := order(d); len(got) != 3 || got[0] != "a-p-i ~/code" {
		t.Errorf("rows %q, want a-p-i first of three", got)
	}
	if got := d.selected(); !startsWith(got, "a-p-i") {
		t.Errorf("selected %q, want the top match", got)
	}
}

func TestWhileSearchingTargetKeysAndListKeysAreTextAndTriggerNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	e.repo("code/beta") // a refresh would find it

	d.press("/")
	d.typeText("qsrjkgG?z/")
	d.press("space", "ö")

	if got, want := header(d), "/qsrjkgG?z/ ö"; got != want {
		t.Errorf("header reads %q, want %q", got, want)
	}
	if d.quit {
		t.Error("q quit while searching")
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q while searching", got)
	}
	d.press("backspace") // ö is two bytes but one character
	if got, want := header(d), "/qsrjkgG?z/"; got != want {
		t.Errorf("after backspace: header reads %q, want %q", got, want)
	}
	d.press("esc")
	if got, want := header(d), "loper · 1 repo sort: last opened"; got != want {
		t.Errorf("after esc: header reads %q, want %q: s switched the sort or r refreshed", got, want)
	}
}

func TestCtrlCQuitsWhileSearching(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.press("/", "ctrl+c")

	if !d.quit {
		t.Error("ctrl+c did not quit")
	}
}

func TestTheArrowsAndCtrlNAndCtrlPMoveThroughTheMatches(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "delta", "gamma"} {
		e.repo("code/" + name)
	}
	d := e.start()
	d.press("/")
	d.typeText("ta") // beta and delta

	for _, s := range []struct {
		key, want string
	}{{"down", "delta"}, {"down", "delta"}, {"up", "beta"}, {"ctrl+n", "delta"}, {"ctrl+p", "beta"}, {"ctrl+p", "beta"}} {
		d.press(s.key)
		if got := d.selected(); !startsWith(got, s.want) {
			t.Errorf("after %s: selected %q, want %s", s.key, got, s.want)
		}
	}
}

func TestEnterOpensTheMenuForTheSelectedMatchAndTheTargetRunsOnIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	threeTargets(e)
	d := e.start()
	d.press("/")
	d.typeText("bet")

	d.press("enter")
	if got := boxRows(d); len(got) != 3 {
		t.Fatalf("want the target menu, got %q:\n%s", got, d.screen())
	}
	d.press("enter")

	if got := e.targetArgs("zed"); !slices.Equal(got, []string{beta}) {
		t.Errorf("zed called with %q, want %q", got, beta)
	}
}

func TestEscRestoresTheFullListAndTheSelectionFromBeforeTheSearch(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		e.repo("code/" + name)
	}
	d := e.start()
	d.press("j") // beta

	d.press("/")
	d.typeText("gam")
	d.press("esc")

	if got, want := order(d), []string{"alpha ~/code", "beta ~/code", "gamma ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "beta") {
		t.Errorf("selected %q, want beta, selected before the search", got)
	}
	if got, want := header(d), "loper · 3 repos sort: last opened"; got != want {
		t.Errorf("header reads %q, want %q", got, want)
	}
}

func TestEscClosesTheMenuFirstAndASecondEscClearsTheSearch(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()
	d.press("/")
	d.typeText("bet")
	d.press("enter")

	d.press("esc")
	if got := boxRows(d); got != nil {
		t.Errorf("after esc: want the menu closed, got %q", got)
	}
	if got := header(d); got != "/bet" {
		t.Errorf("after esc: header reads %q, want the search still on", got)
	}
	d.press("esc")
	if got := header(d); got != "loper · 2 repos sort: last opened" {
		t.Errorf("after a second esc: header reads %q, want the search cleared", got)
	}
}

func TestAQueryThatMatchesNothingShowsNoMatches(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.press("/")
	d.typeText("xyz")

	if screen := d.screen(); !strings.Contains(screen, "\n   No matches\n") || hasRow(screen, "alpha") {
		t.Errorf("want No matches and no rows:\n%s", screen)
	}
	d.press("enter", "z")
	if got := boxRows(d); got != nil {
		t.Errorf("enter with no matches opened %q", got)
	}
	d.press("esc")
	if screen := d.screen(); !hasRow(screen, "alpha ~/code main") {
		t.Errorf("after esc: want alpha back:\n%s", screen)
	}
}

func TestARefreshWhileSearchingUpdatesTheMatchesAndKeepsTheSelectedMatch(t *testing.T) {
	e := newEnv(t)
	alphaBetaGamma(e)
	e.repo("code/zeta")
	e.repo("code/abet")
	d := relaunch(e)
	d.press("/")
	d.typeText("et") // beta, until the refresh adds abet and zeta

	d.release()

	if got, want := order(d), []string{"abet ~/code", "beta ~/code", "zeta ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "beta") {
		t.Errorf("selected %q, want beta, selected before the refresh", got)
	}
	if got := header(d); got != "/et" {
		t.Errorf("header reads %q, want the search still on", got)
	}
}

func TestEscSelectsTheNearestRepoWhenTheOneSelectedBeforeTheSearchIsGone(t *testing.T) {
	e := newEnv(t)
	repos := alphaBetaGamma(e)
	remove(t, repos["beta"])
	d := relaunch(e)
	d.press("j") // beta, still in the cached list
	d.press("/")
	d.typeText("gam")

	d.release() // the refresh drops beta
	d.press("esc")

	if got, want := order(d), []string{"alpha ~/code", "gamma ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "gamma") {
		t.Errorf("selected %q, want gamma, the row that took beta's place", got)
	}
}

func TestAnOpenWhileSearchingKeepsTheSearchAndEscShowsTheOpenedRepoFirst(t *testing.T) {
	e := newEnv(t)
	e.repo("a/api")
	e.repo("b/api") // an equal match, so only the sort orders the two
	e.repo("c/web")
	e.config(`paths = ["~/a", "~/b", "~/c"]`)
	d := e.start()

	d.press("/")
	d.typeText("api")
	d.press("down", "enter", "z") // api in ~/b

	if got, want := order(d), []string{"api ~/b", "api ~/a"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q: the opened repo first", got, want)
	}
	if got := d.selected(); !startsWith(got, "api ~/b") {
		t.Errorf("selected %q, want the opened repo", got)
	}
	d.press("esc")
	if got, want := order(d), []string{"api ~/b", "api ~/a", "web ~/c"}; !slices.Equal(got, want) {
		t.Errorf("after esc: rows %q, want %q", got, want)
	}
	if got := d.selected(); !startsWith(got, "api ~/a") {
		t.Errorf("after esc: selected %q, want api in ~/a, selected before the search", got)
	}
}

func TestTheSearchLineKeepsTheSpinnerAndLeavesOutThePathWarnings(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/nope"]`)
	d := e.start()
	d.hold = true
	d.press("r")

	d.press("/")
	if got, want := header(d), "/ ⠋ refreshing"; got != want {
		t.Errorf("while refreshing: header reads %q, want %q", got, want)
	}
	d.typeText("xyz")
	if got, want := header(d), "/xyz ⠋ refreshing"; got != want {
		t.Errorf("no matches while refreshing: header reads %q, want %q", got, want)
	}
	if screen := d.screen(); !strings.Contains(screen, "No matches") {
		t.Errorf("no matches while refreshing: want No matches:\n%s", screen)
	}
	d.release()
	if got, want := header(d), "/xyz"; got != want {
		t.Errorf("after the refresh: header reads %q, want %q", got, want)
	}
	d.press("esc")
	if got, want := header(d), "loper · 1 repo ~/nope not found sort: last opened"; got != want {
		t.Errorf("after esc: header reads %q, want %q", got, want)
	}
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestMatchedLettersAreHighlightedInEachColumn(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/mango")
	e.repo("code/web")
	d := e.start()

	d.press("/")
	d.typeText("mn")

	rows := styledRows(d)
	if want := []string{"[>] [m]a[n]go {~/code} [m]ai[n] -", "web {~/code} [m]ai[n] -"}; !slices.Equal(rows, want) {
		t.Errorf("rows read %q, want %q", rows, want)
	}
}

func TestTheColumnsStayPutWhileSearching(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/averyverylongname")
	d := e.start()
	folderCell := func() int {
		line := strings.Split(d.screen(), "\n")[3]
		return ansi.StringWidth(line[:strings.Index(line, "~/code")])
	}
	before := folderCell()

	d.press("/")
	d.typeText("alp") // alpha, the shorter name

	if got := order(d); !slices.Equal(got, []string{"alpha ~/code"}) {
		t.Fatalf("rows %q, want only alpha", got)
	}
	if got := folderCell(); got != before {
		t.Errorf("the folder column starts at cell %d while searching, at %d before", got, before)
	}
}

func TestAPasteWhileSearchingGoesToTheQueryWithoutLineBreaksOrControlCharacters(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		e.repo("code/" + name)
	}
	d := e.start()
	d.press("j", "j", "/") // gamma

	d.paste("be\r\nt\x07")

	if got, want := header(d), "/bet"; got != want {
		t.Errorf("header reads %q, want %q", got, want)
	}
	if got := order(d); !slices.Equal(got, []string{"beta ~/code"}) {
		t.Errorf("rows %q, want only beta", got)
	}
	if got := d.selected(); !startsWith(got, "beta") {
		t.Errorf("selected %q, want the top match", got)
	}
}

func TestAPasteOutsideTheSearchDoesNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()
	before := d.screen()

	d.paste("/bjq")

	if d.quit || d.screen() != before {
		t.Errorf("the paste changed the screen or quit:\n%s", d.screen())
	}
}
