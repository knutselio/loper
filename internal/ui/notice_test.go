package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// keysFooter is the footer of a config with only the target Zed.
const keysFooter = "enter open | / search | s sort | r refresh | ? help | q quit | z Zed | a add"

// startQuiet is start with the launch spinner's last tick delivered, so the
// only ticks still to come are those of footer notices.
func startQuiet(e *env) *driver {
	e.t.Helper()
	d := e.start()
	for range 100 {
		if !d.tick() {
			return d
		}
	}
	e.t.Fatal("the launch spinner still ticks after 100 ticks")
	return nil
}

func TestTheConfirmationOfAnOpenClearsByItselfAfterAMoment(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := startQuiet(e)

	d.press("z")
	if got, want := footer(d), "Opened alpha in Zed"; got != want {
		t.Fatalf("after opening: footer reads %q, want %q", got, want)
	}
	if len(d.ticks) != 1 {
		t.Fatalf("after opening: %d ticks pending, want 1", len(d.ticks))
	}
	if got := d.ticks[0].after; got != e.notice {
		t.Errorf("the confirmation shows for %v, want %v", got, e.notice)
	}

	d.tick()
	if got := footer(d); got != keysFooter {
		t.Errorf("after the moment: footer reads %q, want %q", got, keysFooter)
	}
}

func TestTheConfirmationOfAnAddedTargetClearsByItselfAfterAMoment(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := startQuiet(e)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 24}) // room for every item

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")
	if got, want := footer(d), "Added Tab"; got != want {
		t.Fatalf("after adding: footer reads %q, want %q", got, want)
	}

	d.tick()
	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | z Zed | t Tab | a add"; got != want {
		t.Errorf("after the moment: footer reads %q, want %q", got, want)
	}
}

func TestASecondConfirmationStaysForItsOwnMoment(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.config(zedTarget)
	d := startQuiet(e)

	d.press("z", "j", "z")
	d.tick() // the moment of the first confirmation ends
	if got, want := footer(d), "Opened beta in Zed"; got != want {
		t.Errorf("after the first moment: footer reads %q, want %q", got, want)
	}

	d.tick()
	if got := footer(d); got != keysFooter {
		t.Errorf("after the second moment: footer reads %q, want %q", got, keysFooter)
	}
}
