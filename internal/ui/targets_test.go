package ui_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWithNoTargetsTheFooterListsNoneAndZRunsNothing(t *testing.T) {
	for _, body := range []string{"", "targets = []\n", "sort = \"name\"\n"} {
		e := newEnv(t)
		e.repo("code/alpha")
		if body != "" {
			e.config(body)
		}
		d := e.start()

		if !hasExactRow(d.screen(), "alpha ~/code main -") {
			t.Errorf("config %q: want the list as before:\n%s", body, d.screen())
		}
		if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | a add"; got != want {
			t.Errorf("config %q: footer reads %q, want %q", body, got, want)
		}
		d.press("z")

		if got := e.targetArgs("zed"); got != nil {
			t.Errorf("config %q: z ran zed with %q", body, got)
		}
		if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); h != nil {
			t.Errorf("config %q: z recorded %v in the history", body, h)
		}
		d.press("enter")
		if got, want := boxRows(d), []string{noTargets}; !slices.Equal(got, want) {
			t.Errorf("config %q: enter shows %q, want %q", body, got, want)
		}
	}
}

// noTargets is what enter shows in the menu's place when there are no targets.
const noTargets = "No targets yet. Press a to add one."

func TestEnterWithNoTargetsShowsAMessageInsteadOfTheMenu(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()

	d.press("enter")
	if got, want := boxRows(d), []string{noTargets}; !slices.Equal(got, want) {
		t.Fatalf("enter shows %q, want %q:\n%s", got, want, d.screen())
	}
	if strings.Contains(d.screen(), "Open alpha") {
		t.Errorf("enter opened a menu:\n%s", d.screen())
	}

	d.press("j", "k", "down", "enter", "z", "?", "s", "/")
	if got, want := boxRows(d), []string{noTargets}; !slices.Equal(got, want) {
		t.Errorf("after other keys: box reads %q, want the message %q:\n%s", got, want, d.screen())
	}
	if got, want := header(d), "loper · 2 repos sort: last opened"; got != want {
		t.Errorf("header reads %q, want no search and no sort switch: %q", got, want)
	}

	d.press("esc")
	if got := boxRows(d); got != nil {
		t.Errorf("after esc: box reads %q, want none", got)
	}
	if got := d.selected(); !startsWith(got, "alpha ~/code") {
		t.Errorf("selected %q, want alpha still selected", got)
	}

	d.press("enter", "a")
	if screen := d.screen(); !strings.Contains(screen, "╭─ Add a target ─") || strings.Contains(screen, noTargets) {
		t.Errorf("a from the message does not show the add dialog alone:\n%s", screen)
	}
	d.press("esc")
	if got := boxRows(d); got != nil {
		t.Errorf("after esc in the dialog: box reads %q, want none", got)
	}

	d.press("/", "b", "enter", "a")
	if screen := d.screen(); !strings.Contains(screen, "╭─ Add a target ─") || strings.Contains(screen, noTargets) {
		t.Errorf("a from the message during a search does not show the add dialog alone:\n%s", screen)
	}
}

func TestWithNoTargetsTheHelpSaysHowToAddOne(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 60}) // room for the whole help

	d.press("?")

	if rows := boxRows(d); len(rows) < 2 || !slices.Equal(rows[len(rows)-2:], []string{"Targets", "None yet. Press a to add one."}) {
		t.Errorf("help ends with %q, want the Targets heading and how to add one:\n%s", rows, d.screen())
	}
}

func TestQAndCtrlCQuitFromTheNoTargetsMessage(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()

		d.press("enter", key)

		if !d.quit {
			t.Errorf("%s did not quit from the message", key)
		}
	}
}

func TestTheFirstTargetAddedMakesEnterOpenTheMenuWithIt(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.fakeTarget("tab")
	d := e.start()

	d.press("enter")
	d.fill("Tab", "t", "tab {path}") // a, from the message
	d.press("enter", "enter")

	d.press("enter")
	if got, want := boxRows(d), []string{"> t Tab"}; !slices.Equal(got, want) {
		t.Fatalf("menu reads %q, want %q:\n%s", got, want, d.screen())
	}
	d.press("enter")
	if got, want := e.targetArgs("tab"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestAConfiguredTargetsKeyRunsItsCommandWithTheRepoPath(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("tab") + ` window new -c {path}"
`)
	d := e.start()

	d.press("t")

	if got, want := e.targetArgs("tab"), []string{"window", "new", "-c", alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestNameIsReplacedByTheReposName(t *testing.T) {
	e := newEnv(t)
	e.repo("code/aycapi")
	e.config(`
[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("tab") + ` --title {name} -c {path}"
`)
	d := e.start()

	d.press("t")

	if got := e.targetArgs("tab"); len(got) != 4 || got[1] != "aycapi" {
		t.Errorf("tab called with %q, want the title aycapi", got)
	}
}

func TestAPathWithSpacesArrivesAsOneArgumentWithoutQuotingInTheConfig(t *testing.T) {
	e := newEnv(t)
	repo := e.repo("code/my work/the repo")
	e.config(`
[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("tab") + ` {path}"
`)
	d := e.start()

	d.press("t")

	if got, want := e.targetArgs("tab"), []string{repo}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestQuotedWordsArriveAsSingleArgumentsAndAPlaceholderInsideAWordIsSubstitutedInPlace(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Shell"
key = "x"
command = """` + e.fakeTarget("shell") + ` -c 'cd "$1" && ls' "two words" it\\'s --dir={path} {name}.log"""
`)
	d := e.start()

	d.press("x")

	want := []string{"-c", `cd "$1" && ls`, "two words", "it's", "--dir=" + alpha, "alpha.log"}
	if got := e.targetArgs("shell"); !slices.Equal(got, want) {
		t.Errorf("shell called with %q, want %q", got, want)
	}
}

func TestATargetRunsWithTheRepoAsItsWorkingFolder(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(zedTarget)
	d := e.start()

	d.press("z")

	got, err := filepath.EvalSymlinks(e.targetDir("zed"))
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(alpha); got != want {
		t.Errorf("zed ran in %q, want %q", got, want)
	}
}

func TestATargetRunsWithTheToolsOwnEnvironment(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	t.Setenv("LOPER_TEST_GREETING", "hello from the tool")
	e.config(`
[[targets]]
name = "Env"
key = "e"
command = "` + e.script("env", `printf '%s\n' "$LOPER_TEST_GREETING" > "$LOPER_TEST_RECORDS"/env`) + ` {path}"
`)
	d := e.start()

	d.press("e")

	if got, want := e.record("env"), "hello from the tool\n"; got != want {
		t.Errorf("the target saw LOPER_TEST_GREETING=%q, want %q", got, want)
	}
}

// footer returns the last line of the screen with its spacing collapsed.
func footer(d *driver) string {
	lines := strings.Split(d.screen(), "\n")
	return strings.Join(strings.Fields(lines[len(lines)-1]), " ")
}

func TestTheFooterListsTheToolsOwnKeysThenTheKeyedTargets(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"

[[targets]]
name = "Menu only"
command = "zed {path}"

[[targets]]
name = "Terminal tab"
key = "t"
command = "zed {path}"
`)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 100, Height: 24}) // room for every item

	lines := strings.Split(d.screen(), "\n")
	if got, want := lines[len(lines)-1], " enter open | / search | s sort | r refresh | ? help | q quit | z Zed | t Terminal tab | a add"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestASuccessfulOpenIsConfirmedInTheFooterUntilTheNextKey(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/aycapi")
	e.config(zedTarget)
	d := e.start()

	d.press("j", "z")
	if got, want := footer(d), "Opened aycapi in Zed"; got != want {
		t.Errorf("after opening: footer reads %q, want %q", got, want)
	}

	d.press("k")
	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | z Zed | a add"; got != want {
		t.Errorf("after the next key: footer reads %q, want %q", got, want)
	}
}

func TestAFailedOpenIsNotConfirmed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Broken"
key = "b"
command = "false {path}"
`)
	d := e.start()

	d.press("b")

	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | b Broken | a add"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestBackslashesInACommandFollowTheShellsRules(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	// In TOML, \\ is one backslash and \n a newline.
	e.config(`
[[targets]]
name = "Shell"
key = "x"
command = "` + e.fakeTarget("shell") + ` \"\\$ \\` + "`" + ` \\\" \\\\ \\x \\\n\" one\\\ntwo {path}"
`)
	d := e.start()

	d.press("x")

	want := []string{"$ ` \" \\ \\x ", "onetwo", alpha}
	if got := e.targetArgs("shell"); !slices.Equal(got, want) {
		t.Errorf("shell called with %q, want %q", got, want)
	}
}
