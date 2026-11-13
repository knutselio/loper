package ui_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// waiting is the footer while q waits for an open to finish.
const waiting = "Quitting when the open is done"

// historyFile returns the path of the history file in e's state folder.
func historyFile(e *env) string {
	return filepath.Join(e.home, ".local", "state", "loper", "history.json")
}

func TestQDuringAnOpenQuitsOnceItIsDoneAndOtherKeysWorkMeanwhile(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.repo("code/beta")
	e.config(`
[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"

[[targets]]
name = "Other"
key = "o"
command = "` + e.fakeTarget("other") + ` {path}"
`)
	d := e.start()
	d.hold = true // zed's open stays in its wait until release
	d.press("z")
	if got := footer(d); !strings.HasPrefix(got, "enter open") {
		t.Errorf("during the open, before q, the footer reads %q, want the keys", got)
	}

	d.press("q")
	if d.quit {
		t.Fatal("q quit while the open was running")
	}
	if got := footer(d); got != waiting {
		t.Errorf("footer reads %q, want %q", got, waiting)
	}

	d.press("j")
	if got := d.selected(); !strings.HasPrefix(got, "beta") {
		t.Errorf("j selected %q, want beta", got)
	}
	d.press("?")
	if !strings.Contains(d.screen(), "╭─ Help ─") {
		t.Errorf("? did not show the help:\n%s", d.screen())
	}
	d.press("q", "esc") // q in the help, then close it
	d.press("enter")
	if rows := boxRows(d); !slices.Contains(rows, "o Other") {
		t.Errorf("enter did not show the menu, rows %q:\n%s", rows, d.screen())
	}
	d.press("q", "esc", "o") // q in the menu, then a target key while the open runs
	if got := e.targetArgs("other"); got != nil {
		t.Errorf("other ran with %q while the open was running", got)
	}
	if d.quit {
		t.Fatal("a key while waiting quit")
	}
	if got := footer(d); got != waiting {
		t.Errorf("after other keys the footer reads %q, want %q", got, waiting)
	}

	d.release()

	if !d.quit {
		t.Error("the tool did not quit once the open was done")
	}
	if b, err := os.ReadFile(historyFile(e)); err != nil || !strings.Contains(string(b), alpha) {
		t.Errorf("the history holds %q (%v), want the open of %s", b, err, alpha)
	}
}

func TestQDuringAnOpenThatFailsShowsTheDialogAndStays(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "exit 1\n")
	d := e.start()
	d.hold = true
	d.press("b", "q")
	d.release()

	if d.quit {
		t.Error("the tool quit after a failed open")
	}
	if got := boxRows(d); len(got) != 2 || got[1] != "exit status 1" {
		t.Errorf("dialog reads %q, want the command line and the exit status:\n%s", got, d.screen())
	}
	if _, err := os.Stat(historyFile(e)); !os.IsNotExist(err) {
		t.Errorf("a history file exists (%v), want none", err)
	}
	d.press("esc")
	if got := footer(d); got == waiting {
		t.Errorf("after the dialog the footer still reads %q", got)
	}
}

func TestCtrlCDuringAnOpenQuitsAtOnce(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := e.start()
	d.hold = true
	d.press("z", "q", "ctrl+c") // ctrl-c also after a q that waits

	if !d.quit {
		t.Error("ctrl-c did not quit while the open was running")
	}
}

func TestQDuringAnOpenDoesNotQuitWhileTheAddDialogIsOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := e.start()
	d.hold = true
	d.press("z", "q", "a", "N")
	d.release()

	if d.quit {
		t.Error("the tool quit with the add dialog open")
	}
	d.press("esc")
	if got := footer(d); got == waiting {
		t.Errorf("after the open the footer still reads %q", got)
	}
}

// holdOpen presses key, which starts an open, and keeps back what the open
// returns while later work runs on the spot. It returns a function that
// hands the open's result over.
func holdOpen(d *driver, key string) (done func()) {
	d.hold = true
	d.press(key)
	held := d.held
	d.hold, d.held = false, nil
	return func() {
		for _, msg := range held {
			d.send(msg)
		}
	}
}

func TestQInTheConfigRefusalDuringAnOpenClosesItAndQuitsOnceTheOpenIsDone(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config("targets = [{ name = \"Zed\", key = \"z\", command = \"zed {path}\" }]\n")
	d := e.start()
	done := holdOpen(d, "z")
	d.press("a")
	if got := dialogText(d); !strings.Contains(got, "lists its targets inline") {
		t.Fatalf("a shows %q, want the refusal", got)
	}

	d.press("q")
	if d.quit {
		t.Fatal("q quit while the open was running")
	}
	if got := dialogText(d); got != "" {
		t.Errorf("after q the refusal still shows %q", got)
	}
	if got := footer(d); got != waiting {
		t.Errorf("footer reads %q, want %q", got, waiting)
	}
	done()

	if !d.quit {
		t.Error("the tool did not quit once the open was done")
	}
	if b, err := os.ReadFile(historyFile(e)); err != nil || !strings.Contains(string(b), alpha) {
		t.Errorf("the history holds %q (%v), want the open of %s", b, err, alpha)
	}
}

func TestQInTheNoTargetsMessageDuringAnOpenQuitsOnceTheOpenIsDone(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := e.start()
	done := holdOpen(d, "z")
	d.press("enter", "d") // remove the only target while its open runs
	d.pass()
	d.press("enter")
	d.press("enter")
	if got := boxRows(d); !slices.Equal(got, []string{"No targets yet. Press a to add one."}) {
		t.Fatalf("enter shows %q, want the no-targets message:\n%s", got, d.screen())
	}

	d.press("q")
	if d.quit {
		t.Fatal("q quit while the open was running")
	}
	if got := footer(d); got != waiting {
		t.Errorf("footer reads %q, want %q", got, waiting)
	}
	done()

	if !d.quit {
		t.Error("the tool did not quit once the open was done")
	}
}
