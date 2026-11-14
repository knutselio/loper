package ui_test

import (
	"errors"
	"fmt"
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

func TestADialogBiggerThanTheScreenKeepsItsBordersAndShowsTheEndCut(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	// Twenty long lines, with a tab and colour codes that must not reach the
	// screen as they are.
	brokenTarget(e, "for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do printf '\\033[31mline\\t%s of a long error message\\033[0m\\n' $i; done\nexit 1\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 36, Height: 10}) // 6 list lines: room for 4 rows

	d.press("b")

	screen := d.screen()
	if strings.ContainsAny(screen, "\x1b\t") {
		t.Errorf("the output's escape codes or tabs reached the screen: %q", screen)
	}
	got := boxRows(d)
	if len(got) != 4 || !strings.HasPrefix(got[0], "$ ") || !strings.HasSuffix(got[0], "…") ||
		got[1] != "… 19 earlier lines left out" ||
		!strings.HasPrefix(got[2], "line 20 of a long") || !strings.HasSuffix(got[2], "…") || got[3] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line, how many lines were left out, the last line of output, both cut, and the exit status:\n%s", got, screen)
	}
	if strings.Count(screen, "╭") != 1 || strings.Count(screen, "╰") != 1 {
		t.Errorf("dialog does not have one top and one bottom border:\n%s", screen)
	}
	for i, line := range strings.Split(screen, "\n") {
		if w := ansi.StringWidth(line); w > 36 {
			t.Errorf("line %d is %d cells wide, wider than the screen: %q", i+1, w, line)
		}
	}
}

func TestADialogShowsTheCommandHowItEndedAndTheLastLinesOfOutput(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	script := brokenTarget(e, "seq -f 'progress %g' 30\necho FINAL_REASON\nexit 3\n")
	d := e.start() // 80×24: 20 list lines, room for 18 rows

	d.press("b")

	command := "$ " + script + " --flag '" + alpha + "'"
	want := []string{"… 16 earlier lines left out"}
	for i := 17; i <= 30; i++ {
		want = append(want, "progress "+strconv.Itoa(i))
	}
	want = append(want, "FINAL_REASON", "exit status 3")
	got := boxRows(d)
	if len(got) == 0 || !(got[0] == command || strings.HasSuffix(got[0], "…") && strings.HasPrefix(command, strings.TrimSuffix(got[0], "…"))) {
		t.Errorf("dialog starts with %q, want the command line %q:\n%s", got, command, d.screen())
	} else if !slices.Equal(got[1:], want) {
		t.Errorf("dialog reads %q after the command line, want %q:\n%s", got[1:], want, d.screen())
	}

	e.noColor = true // the styles on, to see the line that says so dimmed
	d = e.start()
	d.press("b")
	if screen := d.screen(); !strings.Contains(screen, "\x1b[2m … 16 earlier lines left out \x1b[22m") || strings.Contains(screen, "\x1b[2m progress") {
		t.Errorf("only the line that says lines were left out should be dimmed: %q", screen)
	}
}

func TestAtTheSmallestHeightTheDialogDrawsItShowsTheCommandAndHowItEnded(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "seq 30\necho FINAL_REASON\nexit 3\n")
	d := e.start()
	d.press("b")

	h := 0
	for ; h < 24; h++ {
		d.send(tea.WindowSizeMsg{Width: 80, Height: h})
		if boxRows(d) != nil {
			break
		}
	}

	if got := boxRows(d); len(got) != 2 || !strings.HasPrefix(got[0], "$ ") || got[1] != "exit status 3" {
		t.Errorf("at height %d the dialog reads %q, want the command line and the exit status:\n%s", h, got, d.screen())
	}
	d.send(tea.WindowSizeMsg{Width: 80, Height: h + 1})
	if got := boxRows(d); len(got) != 3 || got[1] != "… 31 earlier lines left out" || got[2] != "exit status 3" {
		t.Errorf("at height %d the dialog reads %q, want how many lines were left out above the exit status:\n%s", h+1, got, d.screen())
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
	// The output is longer than the dialog, so its end shows above the exit status.
	if got := boxRows(d); len(got) < 4 || !strings.HasPrefix(got[1], "… ") || got[len(got)-2] != "y" || got[len(got)-1] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line, the end of its output and the exit status:\n%s", got, d.screen())
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

	// 5957 lines are kept, from 14044 to 20000; those that do not fit are
	// counted as left out, and the lines before them make it more.
	got := boxRows(d)
	if len(got) < 4 || got[len(got)-2] != "line 20000" || got[len(got)-1] != "exit status 1" ||
		got[1] != fmt.Sprintf("… more than %d earlier lines left out", 5957-(len(got)-3)) {
		t.Errorf("dialog reads %q, want the end of the output from line 14044 on:\n%s", got, d.screen())
	}
}

func TestOutputThatExactlyFillsTheDialogShowsWholeAndOneLineMoreIsCounted(t *testing.T) {
	for _, c := range []struct {
		lines int
		want  []string // the rows between the command line and the exit status
	}{
		{16, nil}, // 80×24: 18 rows, all for the command, 16 lines and the ending
		{17, []string{"… 2 earlier lines left out"}}, // one more: the count line and the last 15
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		brokenTarget(e, "seq "+strconv.Itoa(c.lines)+"\nexit 1\n")
		d := e.start()

		d.press("b")

		want := c.want
		for i := c.lines - (16 - len(c.want)) + 1; i <= c.lines; i++ {
			want = append(want, strconv.Itoa(i))
		}
		want = append(want, "exit status 1")
		if got := boxRows(d); len(got) == 0 || !slices.Equal(got[1:], want) {
			t.Errorf("%d lines: dialog reads %q after the command line, want %q:\n%s", c.lines, got, want, d.screen())
		}
	}
}

func TestTrailingBlankLinesOfOutputDoNotShow(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "echo only\necho\necho\nexit 1\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("b")

	if got := boxRows(d); len(got) != 3 || got[1] != "only" || got[2] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line, the one line of output and the exit status:\n%s", got, d.screen())
	}
}

func TestTheDialogIsAsWideAsTheLinesItShows(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	// An early line far wider than the command line, then more lines than fit.
	script := brokenTarget(e, "printf '%0390d\\n' 0\nseq 30\nexit 1\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("b")

	command := " $ " + script + " --flag '" + alpha + "' "
	for _, line := range strings.Split(d.screen(), "\n") {
		if i, j := strings.Index(line, "╭"), strings.Index(line, "╮"); i >= 0 && j > i {
			if got, want := ansi.StringWidth(line[i:j+len("╮")]), ansi.StringWidth(command)+2; got != want {
				t.Errorf("the dialog is %d cells wide, want %d, as wide as the command line:\n%s", got, want, d.screen())
			}
		}
	}
}

func TestALongStartErrorKeepsItsCauseInANarrowWindow(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	missing := filepath.Join(e.root, "a-folder-with-a-rather-long-name-for-a-program", "missing")
	e.config("[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"'" + missing + "' {path}\"\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 40, Height: 24})

	d.press("b")

	got := boxRows(d)
	if len(got) != 2 || !strings.HasPrefix(got[0], "$ ") || !strings.HasSuffix(got[0], "…") ||
		!strings.HasPrefix(got[1], "…") || !strings.HasSuffix(got[1], "missing: no such file or directory") {
		t.Errorf("dialog reads %q, want the command line cut at its end and the reason cut at its start:\n%s", got, d.screen())
	}
}

func TestAPathInTheReasonIsEscapedAsInTheCommandLine(t *testing.T) {
	e := newEnv(t)
	odd := e.repo("code/a\x01b")
	e.writeFile(filepath.Join(odd, "notes"), "")
	e.config("[[targets]]\nname = \"Go there\"\nkey = \"c\"\ncommand = \"cd {path}/notes\"\nin_place = true\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("c")

	got := boxRows(d)
	if len(got) != 2 || !strings.Contains(got[0], `/code/a\x01b/notes`) || !strings.HasSuffix(got[1], `/code/a\x01b/notes is not a folder`) {
		t.Errorf("dialog reads %q, want the path escaped the same way in the command line and the reason:\n%s", got, d.screen())
	}
}

func TestOutputCutBeforeItsLast64KBIsCountedAsMore(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	// Seven lines of 20000 bytes: the last 64 KB start inside the fourth, so
	// three whole lines are kept, and all of them fit.
	brokenTarget(e, "for i in 1 2 3 4 5 6 7; do printf \"%019999d\\n\" $i; done\nexit 1\n")
	d := e.start()

	d.press("b")

	if got := boxRows(d); len(got) != 6 || got[1] != "… earlier output left out" || got[len(got)-1] != "exit status 1" {
		t.Errorf("dialog reads %q, want a line saying earlier output was left out above the three lines kept:\n%s", got, d.screen())
	}
}
