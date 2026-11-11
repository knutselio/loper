package ui_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWithNoTargetsConfiguredZOpensTheSelectedRepoInZed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	d := e.start()

	d.press("j", "z")

	if got := e.targetArgs("zed"); len(got) != 1 || got[0] != beta {
		t.Errorf("zed called with %q, want [%q]", got, beta)
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

func TestDefiningATargetRemovesTheBuiltInZed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Terminal tab"
key = "t"
command = "` + e.fakeTarget("tab") + ` {path}"
`)
	d := e.start()

	d.press("z")

	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q after a target was configured", got)
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
	if got, want := lines[len(lines)-1], " enter open | / search | s sort | r refresh | ? help | q quit | z Zed | t Terminal tab"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestASuccessfulOpenIsConfirmedInTheFooterUntilTheNextKey(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/aycapi")
	d := e.start()

	d.press("j", "z")
	if got, want := footer(d), "Opened aycapi in Zed"; got != want {
		t.Errorf("after opening: footer reads %q, want %q", got, want)
	}

	d.press("k")
	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | z Zed"; got != want {
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

	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | b Broken"; got != want {
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
