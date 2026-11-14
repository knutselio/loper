package ui_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// brokenTarget configures the target "Broken" on the key b, a script with
// body as its commands, and returns its path quoted as in the command.
func brokenTarget(e *env, body string) string {
	script := e.script("broken", body)
	e.config(`
[[targets]]
name = "Broken"
key = "b"
command = "` + script + ` --flag {path}"

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"
`)
	return script
}

func TestATargetThatExitsNonZeroShowsADialogWithTheCommandLineAndItsOutput(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	script := brokenTarget(e, "echo 'to stdout'\necho 'fatal: to stderr' >&2\nexit 3\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24}) // wide enough for the temporary paths

	d.press("b")

	want := []string{"$ " + script + " --flag '" + alpha + "'", "to stdout", "fatal: to stderr", "exit status 3"}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
	if !strings.Contains(d.screen(), "Could not open alpha in Broken") {
		t.Errorf("dialog does not name the repo and the target:\n%s", d.screen())
	}
}

func TestATargetWhoseProgramDoesNotExistShowsTheDialogWithTheCommandLineAndTheReason(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Missing"
key = "m"
command = "no-such-program {path}"
`)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("m")

	want := []string{"$ no-such-program '" + alpha + "'", `exec: "no-such-program": executable file not found in $PATH`}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
}

func TestEscClosesTheDialogAndTheListWorksAgain(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	brokenTarget(e, "exit 1\n")
	d := e.start()
	d.press("b")

	d.press("j", "z", "enter") // inert while the dialog is open
	if boxRows(d) == nil {
		t.Fatalf("no dialog after a failed open:\n%s", d.screen())
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q while the dialog was open", got)
	}

	d.press("esc")
	if got := boxRows(d); got != nil {
		t.Errorf("dialog still shown after esc: %q", got)
	}
	d.press("j", "z")
	if got := d.selected(); !startsWith(got, "beta ~/code main") {
		t.Errorf("selected %q after esc and j, want beta", got)
	}
	if got, want := e.targetArgs("zed"), []string{beta}; !slices.Equal(got, want) {
		t.Errorf("zed called with %q after the dialog closed, want %q", got, want)
	}
}

func TestQAndCtrlCQuitFromInsideTheDialog(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		e := newEnv(t)
		e.repo("code/alpha")
		brokenTarget(e, "exit 1\n")
		d := e.start()
		d.press("b")
		if boxRows(d) == nil {
			t.Fatalf("no dialog after a failed open:\n%s", d.screen())
		}

		d.press(key)

		if !d.quit {
			t.Errorf("%s in the dialog did not quit", key)
		}
	}
}

func TestADialogBiggerThanTheScreenKeepsItsBordersAndCutsItsContent(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	// Twenty long lines, with a tab and colour codes that must not reach the
	// screen as they are.
	brokenTarget(e, "for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do printf '\\033[31mline\\t%s of a long error message\\033[0m\\n' $i; done\nexit 1\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 30, Height: 8}) // 4 list lines: room for 2 rows

	d.press("b")

	screen := d.screen()
	if strings.ContainsAny(screen, "\x1b\t") {
		t.Errorf("the output's escape codes or tabs reached the screen: %q", screen)
	}
	if got := boxRows(d); len(got) != 2 || !strings.HasPrefix(got[0], "$ ") || !strings.HasSuffix(got[0], "…") || !strings.HasPrefix(got[1], "line 1 of a long") || !strings.HasSuffix(got[1], "…") {
		t.Errorf("dialog reads %q, want the command line and the first line of output, both cut:\n%s", got, screen)
	}
	if strings.Count(screen, "╭") != 1 || strings.Count(screen, "╰") != 1 {
		t.Errorf("dialog does not have one top and one bottom border:\n%s", screen)
	}
	for i, line := range strings.Split(screen, "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Errorf("line %d is %d cells wide, wider than the screen: %q", i+1, w, line)
		}
	}
}

func TestTheDialogSurvivesAScreenTooSmallForItAndShowsAgainWhenThereIsRoom(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "exit 1\n")
	d := e.start()
	d.press("b")

	for w := range 6 {
		for h := range 11 {
			d.send(tea.WindowSizeMsg{Width: w, Height: h})
			_ = d.screen()
		}
	}
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	if got := boxRows(d); len(got) != 2 || got[1] != "exit status 1" {
		t.Errorf("dialog reads %q after the screen grew back:\n%s", got, d.screen())
	}
}

// pid returns the process id a fake target wrote to the record rel, or 0
// if it has not written it yet.
func pid(e *env, rel string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(e.record(rel)))
	return n
}

// eventually fails the test unless cond becomes true within a few seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within 5s", what)
		}
	}
}

// runningTarget configures the target "Server" on the key v, a program that
// keeps running until the test creates the file server.go in the root, then
// writes server.done and exits. It records its process id in server.pid.
// It is told to go when the test ends, so it never outlives the test.
func runningTarget(e *env) {
	r := e.root + "/server"
	e.t.Cleanup(func() { e.writeFile(r+".go", "") })
	// The loop gives up after about ten seconds, in case the test never says go.
	e.config(`
[[targets]]
name = "Server"
key = "v"
command = "` + e.script("server", "echo $$ > '"+r+".pid'\nfor i in $(seq 200); do [ -e '"+r+".go' ] && break; sleep 0.05; done\necho > '"+r+".done'\n") + ` {path}"
`)
}

func TestATargetStillRunningWhenTheWaitEndsCountsAsOpenedAndRunsOnDetached(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	runningTarget(e)
	e.wait = 200 * time.Millisecond
	d := e.start()

	began := time.Now()
	d.press("v")
	took := time.Since(began)

	if took > e.wait+time.Second {
		t.Errorf("the open took %v, longer than the wait of %v", took, e.wait)
	}
	if got, want := footer(d), "Opened alpha in Server"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	if d.quit {
		t.Error("the tool quit after opening a repo")
	}
	eventually(t, "the target recording its process id", func() bool { return pid(e, "server.pid") > 0 })
	p := pid(e, "server.pid")
	if g, err := syscall.Getpgid(p); err != nil || g != p || g == syscall.Getpgrp() {
		t.Errorf("the target is in process group %d (%v), want one of its own, apart from loper's %d", g, err, syscall.Getpgrp())
	}
	e.writeFile(filepath.Join(e.root, "server.go"), "")
	eventually(t, "the target answering after the wait", func() bool { return e.record("server.done") != "" })
	eventually(t, "the finished target being collected rather than left a zombie", func() bool {
		return errors.Is(syscall.Kill(p, 0), syscall.ESRCH)
	})
}

func TestAFailedTargetIsReportedAtOnceEvenIfWhatItStartedKeepsItsOutputOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "sleep 60 &\necho $! > '"+e.root+"/sleep.pid'\necho started\nexit 1\n")
	t.Cleanup(func() {
		// The sleep is still running, so its id is still its own.
		if p := pid(e, "sleep.pid"); p > 0 {
			syscall.Kill(p, syscall.SIGKILL)
		}
	})
	d := e.start() // the wait is several seconds

	began := time.Now()
	d.press("b")

	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("the open took %v, waiting on the program left running", took)
	}
	if got := boxRows(d); len(got) != 3 || got[1] != "started" || got[2] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line, its output and the exit status:\n%s", got, d.screen())
	}
}

func TestAFailedTargetIsReportedAtOnceEvenIfWhatItStartedWritesWithoutEnd(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	// It starts four copies of yes, which write as fast as they can, and
	// fails once they are about to run.
	r := e.root + "/yes"
	brokenTarget(e, "for i in 1 2 3 4; do\n\tsh -c \": > '"+r+".$i'; exec yes\" &\n\techo $! >> '"+r+".pids'\ndone\n"+
		"until [ -e '"+r+".1' ] && [ -e '"+r+".2' ] && [ -e '"+r+".3' ] && [ -e '"+r+".4' ]; do sleep 0.01; done\nexit 1\n")
	t.Cleanup(func() {
		for _, line := range strings.Fields(e.record("yes.pids")) {
			if p, _ := strconv.Atoi(line); p > 0 {
				syscall.Kill(p, syscall.SIGKILL)
			}
		}
	})
	d := e.start() // the wait is several seconds

	began := time.Now()
	d.press("b")

	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("the open took %v, reading what was left running", took)
	}
	// The output is longer than the dialog, so the exit status is cut off.
	if got := boxRows(d); len(got) < 2 || got[1] != "y" {
		t.Errorf("dialog reads %q, want the command line and its output:\n%s", got, d.screen())
	}
}

func TestWithCloseOnOpenTheToolQuitsAfterASuccessfulOpen(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config("close_on_open = true\n" + zedTarget)
	d := e.start()

	d.press("z")

	if got, want := e.targetArgs("zed"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("zed called with %q, want %q", got, want)
	}
	if !d.quit {
		t.Error("the tool did not quit after a successful open")
	}
}

func TestATargetsCloseOverridesCloseOnOpenAfterASuccessfulOpen(t *testing.T) {
	for _, c := range []struct {
		setting, close string // "" leaves the line out
		quits          bool
	}{
		{"false", "true", true},
		{"false", "", false},
		{"true", "false", false},
		{"true", "", true},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		body := "close_on_open = " + c.setting + "\n\n[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n"
		if c.close != "" {
			body += "close = " + c.close + "\n"
		}
		e.config(body)
		d := e.start()

		d.press("z")

		if e.targetArgs("zed") == nil {
			t.Errorf("close_on_open %s, close %q: zed did not run", c.setting, c.close)
		}
		if d.quit != c.quits {
			t.Errorf("close_on_open %s, close %q: quit %v, want %v", c.setting, c.close, d.quit, c.quits)
		}
	}
}

func TestWithCloseOnOpenAFailedOpenShowsTheDialogAndTheToolDoesNotQuit(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
close_on_open = true

[[targets]]
name = "Broken"
key = "b"
command = "false {path}"
`)
	d := e.start()

	d.press("b")

	if d.quit {
		t.Error("the tool quit after a failed open")
	}
	if got := boxRows(d); len(got) != 2 || got[1] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line and the exit status:\n%s", got, d.screen())
	}
}

func TestAFailedOpenDoesNotQuitWhenItsTargetSaysClose(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config("\n[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"false {path}\"\nclose = true\n")
	d := e.start()

	d.press("b")

	if d.quit {
		t.Error("the tool quit after a failed open")
	}
	if got := boxRows(d); len(got) != 2 || got[1] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line and the exit status:\n%s", got, d.screen())
	}
}

func TestWhileAnOpenIsRunningNoOtherTargetRunsAndItsFailureIsNotLost(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(`
close_on_open = true

[[targets]]
name = "Broken"
key = "b"
command = "false {path}"

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"
`)
	d := e.start()

	d.hold = true
	d.press("b")
	d.press("z")                   // a target key
	d.press("enter", "j", "enter") // the menu
	d.release()

	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("zed called with %q while another open was running", got)
	}
	if d.quit {
		t.Error("the tool quit with a failed open")
	}
	if got := boxRows(d); len(got) != 2 || got[1] != "exit status 1" {
		t.Errorf("dialog reads %q, want the failed open's command line and exit status:\n%s", got, d.screen())
	}

	d.press("esc", "z")
	if got, want := e.targetArgs("zed"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("zed called with %q once the open had finished, want %q", got, want)
	}
}

func TestAFailedTargetsOutputIsCutToItsLast64KB(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	// 20000 lines of 11 bytes: the last 64 KB start inside line 14043, so
	// the first whole line kept is 14044.
	brokenTarget(e, "seq -f 'line %05g' 20000\nexit 1\n")
	d := e.start()

	d.press("b")

	if got := boxRows(d); len(got) < 3 || got[1] != "line 14044" || got[2] != "line 14045" {
		t.Errorf("dialog reads %q, want the output from line 14044 on:\n%s", got, d.screen())
	}
}
