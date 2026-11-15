package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestALaunchRefreshThatEndsAtOnceShowsTheSpinnerForItsLeastTimeThenSaysHowManyLoaded(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.start()
	e.repo("code/beta")

	d := e.start() // the refresh has found beta at once

	if !hasRow(d.screen(), "beta ~/code main") {
		t.Errorf("want beta listed as soon as the refresh is done:\n%s", d.screen())
	}
	if got, want := topRight(d), "⠋ refreshing"; got != want {
		t.Errorf("before %v: top right reads %q, want %q", e.spin, got, want)
	}
	d.after(e.spin)
	if got, want := topRight(d), "2 repos loaded"; got != want {
		t.Errorf("after %v: top right reads %q, want %q", e.spin, got, want)
	}
	d.after(e.notice)
	if got, want := topRight(d), "2 repos"; got != want {
		t.Errorf("after the notice's moment: top right reads %q, want %q", got, want)
	}
}

func TestRShowsTheSpinnerForItsLeastTimeThenSaysHowManyLoaded(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := startQuiet(e)

	d.press("r") // the refresh ends at once

	if got, want := topRight(d), "refreshing"; !strings.HasSuffix(got, want) {
		t.Errorf("before %v: top right reads %q, want the spinner", e.spin, got)
	}
	d.after(e.spin)
	if got, want := topRight(d), "1 repo loaded"; got != want {
		t.Errorf("after %v: top right reads %q, want %q", e.spin, got, want)
	}
	d.press("j") // the next key ends the notice, as any notice
	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("after a key: top right reads %q, want %q", got, want)
	}
}

func TestARefreshSlowerThanTheLeastTimeSaysHowManyLoadedWhenItEnds(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := startQuiet(e)
	e.repo("code/beta")

	d.hold = true
	d.press("r")
	d.after(e.spin)
	if got, want := topRight(d), "refreshing"; !strings.HasSuffix(got, want) {
		t.Errorf("after %v, still scanning: top right reads %q, want the spinner", e.spin, got)
	}
	d.release()

	if got, want := topRight(d), "2 repos loaded"; got != want {
		t.Errorf("after the scan: top right reads %q, want %q", got, want)
	}
}

func TestARefreshThatFindsAProblemShowsTheProblemAndNoNotice(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/typo"]`)
	d := e.start()

	d.after(e.spin)

	if got, want := topRight(d), "! ~/typo not found"; got != want {
		t.Errorf("top right reads %q, want %q", got, want)
	}
}

func TestAFirstRunSaysItCreatedTheConfigFileThenHowManyLoadedAndASecondRunDoesNot(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")

	d := e.start()
	const created = "Created ~/.config/loper/config.toml with a cd target on o"
	if got := topRight(d); got != created {
		t.Errorf("first run: top right reads %q, want %q", got, created)
	}
	d.after(e.spin)
	if got := topRight(d); got != created {
		t.Errorf("first run, after the spinner: top right reads %q, want %q still", got, created)
	}
	d.after(e.notice)
	if got, want := topRight(d), "1 repo loaded"; got != want {
		t.Errorf("first run, after its notice: top right reads %q, want %q", got, want)
	}
	d.after(e.notice)
	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("first run, after both notices: top right reads %q, want %q", got, want)
	}

	d = e.start()
	d.after(e.spin)
	if got, want := topRight(d), "1 repo loaded"; got != want {
		t.Errorf("second run: top right reads %q, want %q and no first-run notice", got, want)
	}
}

func TestTheFirstRunNoticeDrawsTheConfigFileEscaped(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.home, `x\`+red))
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 100, Height: 24}) // room for the whole notice

	if want := `Created ~/x\\` + redShown + "/loper/config.toml with a cd target on o"; !shown(d, want) {
		t.Errorf("want %q and no escape code:\n%q", want, d.screen())
	}
}

func TestEAndDOnTheListSayWhereTheyWorkAndDoNothingElse(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := startQuiet(e)

	for _, c := range []struct{ key, want string }{
		{"e", "e edits a target from the target menu: press enter first"},
		{"d", "d removes a target from the target menu: press enter first"},
	} {
		d.press(c.key)
		if got := topRight(d); got != c.want {
			t.Errorf("%s: top right reads %q, want %q", c.key, got, c.want)
		}
		if got := boxRows(d); got != nil {
			t.Errorf("%s: a box shows %q, want none", c.key, got)
		}
		if got, _ := fileText(t, configPath()); got != zedTarget {
			t.Errorf("%s: config is %q, want it unchanged", c.key, got)
		}
	}
	d.after(e.notice) // the e notice's moment ends and leaves d's
	if got, want := topRight(d), "d removes a target from the target menu: press enter first"; got != want {
		t.Errorf("after e's moment: top right reads %q, want %q", got, want)
	}
	d.after(e.notice)
	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("after d's moment: top right reads %q, want %q", got, want)
	}

	for _, c := range []struct{ key, title string }{{"e", "╭─ Edit a target ─"}, {"d", "╭─ Remove a target ─"}} {
		d.press("esc", "esc", "enter", c.key) // in the target menu they act, without the list's notice
		if screen := d.screen(); !strings.Contains(screen, c.title) || strings.Contains(topRight(d), "target menu") {
			t.Errorf("%s in the menu: want %q and no notice of the list:\n%s", c.key, c.title, screen)
		}
	}
}

func TestTargetsOnCapitalEAndDStillOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.fakeTarget("edit")
	e.fakeTarget("dup")
	e.config("[[targets]]\nname = \"Edit\"\nkey = \"E\"\ncommand = \"edit {path}\"\n\n[[targets]]\nname = \"Dup\"\nkey = \"D\"\ncommand = \"dup {path}\"\n")
	d := startQuiet(e)

	d.press("E")
	if got, want := topRight(d), "Opened alpha in Edit"; got != want {
		t.Errorf("E: top right reads %q, want %q", got, want)
	}
	d.press("D")
	if got, want := topRight(d), "Opened alpha in Dup"; got != want {
		t.Errorf("D: top right reads %q, want %q", got, want)
	}
	if e.targetArgs("edit") == nil || e.targetArgs("dup") == nil {
		t.Error("a target on E or D did not run")
	}
}

func TestTheSpinnerMovesWhileItIsHeld(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start() // the refresh is done; its spinner is held

	d.after(frame)

	if got, want := topRight(d), "⠙ refreshing"; got != want {
		t.Errorf("after a frame: top right reads %q, want %q", got, want)
	}
}

func TestRDuringTheHoldStartsAHoldOfItsOwn(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start() // the launch refresh is done; its spinner is held

	d.press("r")
	d.after(e.spin) // the launch's hold ends, not r's
	if got := topRight(d); !strings.HasSuffix(got, " refreshing") {
		t.Errorf("after the launch's hold: top right reads %q, want the spinner", got)
	}
	d.after(e.spin)
	if got, want := topRight(d), "1 repo loaded"; got != want {
		t.Errorf("after r's hold: top right reads %q, want %q", got, want)
	}
}

func TestAKeyDropsTheNoticeThatWaitsForTheFirstRunOne(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")
	d := e.start()
	d.after(e.spin) // "1 repo loaded" now waits for the first-run notice

	d.press("j")
	d.after(e.notice) // the first-run notice's moment ends

	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("top right reads %q, want %q and no waiting notice", got, want)
	}
}

func TestTheLoadedNoticeCountsEveryRepoDuringASearch(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	startQuiet(e)
	d := relaunch(e) // the refresh's result is on its way

	d.press("/")
	d.typeText("alp")
	d.release()
	d.after(e.spin)

	if got, want := topRight(d), "2 repos loaded"; got != want {
		t.Errorf("top right reads %q, want %q", got, want)
	}
}

// emptyCode makes ~/code a folder without repos.
func emptyCode(e *env) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Join(e.home, "code"), 0o755); err != nil {
		e.t.Fatal(err)
	}
}

func TestWithNoReposScanningStaysForTheHoldThenNoReposFoundAndNoNotice(t *testing.T) {
	e := newEnv(t)
	emptyCode(e)
	d := e.start() // the launch refresh found nothing at once

	for _, when := range []string{"launch", "r"} {
		if screen := d.screen(); !strings.Contains(screen, "scanning…") || strings.Contains(screen, "No repos found") {
			t.Errorf("%s, during the hold: want scanning… and no No repos found:\n%s", when, screen)
		}
		d.after(e.spin)
		if screen := d.screen(); strings.Contains(screen, "scanning…") || !strings.Contains(screen, "No repos found in ~/code") {
			t.Errorf("%s, after the hold: want No repos found:\n%s", when, screen)
		}
		if got, want := topRight(d), "0 repos"; got != want {
			t.Errorf("%s, after the hold: top right reads %q, want %q and no notice", when, got, want)
		}
		d.press("r")
	}
}
