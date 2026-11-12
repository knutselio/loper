package ui_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestSpaceOnTheListOpensTheTargetMenuForTheSelectedRepo(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	threeTargets(e)
	d := e.start()

	d.press("j", "space")

	if got, want := boxRows(d), []string{"> z Zed", "Menu only", "t Terminal tab"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want %q:\n%s", got, want, d.screen())
	}
	if !strings.Contains(d.screen(), "╭─ Open beta ─") {
		t.Errorf("menu is not for the selected repo:\n%s", d.screen())
	}
	for _, name := range []string{"zed", "menu-only", "tab"} {
		if got := e.targetArgs(name); got != nil {
			t.Errorf("%s called with %q by opening the menu", name, got)
		}
	}
}

func TestSpaceOnTheListWithNoTargetsShowsTheNoTargetsMessage(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.press("space")

	if got, want := boxRows(d), []string{noTargets}; !slices.Equal(got, want) {
		t.Errorf("space shows %q, want %q:\n%s", got, want, d.screen())
	}
}

func TestSpaceInTheMenuOpensTheHighlightedTargetAsEnterDoes(t *testing.T) {
	for _, c := range []struct {
		close string // the second target's close line, "" for none
		quits bool
	}{
		{"", false},
		{"close = true\n", true},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		beta := e.repo("code/beta")
		e.config("[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n\n[[targets]]\nname = \"Menu only\"\ncommand = \"" +
			e.fakeTarget("menu-only") + " {path}\"\n" + c.close)
		d := e.start()

		d.press("j", "space", "j", "space")

		if got, want := e.targetArgs("menu-only"), []string{beta}; !slices.Equal(got, want) {
			t.Errorf("close %q: menu-only called with %q, want %q", c.close, got, want)
		}
		if got := e.targetArgs("zed"); got != nil {
			t.Errorf("close %q: zed called with %q, want only the highlighted target run", c.close, got)
		}
		if d.quit != c.quits {
			t.Errorf("close %q: quit %v, want %v", c.close, d.quit, c.quits)
		}
		if !c.quits {
			if got, want := footer(d), "Opened beta in Menu only"; got != want {
				t.Errorf("close %q: footer reads %q, want %q", c.close, got, want)
			}
			if got := boxRows(d); got != nil {
				t.Errorf("close %q: menu still shown after running a target: %q", c.close, got)
			}
		}
		if _, ok := savedHistory(t, os.Getenv("XDG_STATE_HOME"))[beta]; !ok {
			t.Errorf("close %q: the open of beta is not recorded", c.close)
		}
	}
}

func TestSpaceInTheDialogsTextFieldsIsText(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.press("a")
	d.typeText("My")
	d.press("space")
	d.typeText("Tab")
	d.press("tab")
	d.typeText("t")
	d.press("tab")
	d.typeText("tab")
	d.press("space")
	d.typeText("{path}")

	if got, want := boxRows(d), []string{"Name My Tab", "Key t", "> Command tab {path}"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("dialog reads %q, want %q", got, want)
	}
}

func TestSpaceOnAWriteOrRemoveConfirmationWritesNothing(t *testing.T) {
	for _, c := range []struct {
		name  string
		keys  func(d *driver)
		title string
	}{
		{"add", func(d *driver) { d.fill("Vim", "v", "vim {path}"); d.press("enter", "enter") }, "╭─ Add a target ─"},
		{"edit", func(d *driver) { d.press("enter", "e"); d.retype("Zedd"); d.toLast(); d.press("enter") }, "╭─ Edit a target ─"},
		{"remove", func(d *driver) { d.press("enter", "d") }, "╭─ Remove a target ─"},
	} {
		e := newEnv(t)
		d, _ := editStart(e, editConfig)
		c.keys(d)
		d.pass()

		d.press("space")

		if got, _ := fileText(t, configPath()); got != editConfig {
			t.Errorf("%s: config is %q, want it unchanged", c.name, got)
		}
		if !strings.Contains(d.screen(), c.title) {
			t.Errorf("%s: the confirmation is gone after space:\n%s", c.name, d.screen())
		}
	}
}

func TestSpaceDoesNothingInTheNoTargetsMessageTheErrorDialogAndTheHelp(t *testing.T) {
	for _, c := range []struct {
		name   string
		config string
		keys   []string
	}{
		{"no-targets message", "", []string{"enter"}},
		{"error dialog", "[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"false {path}\"\n", []string{"b"}},
		{"help", zedTarget, []string{"?"}},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(c.config)
		d := e.start()
		d.press(c.keys...)
		before := d.screen()

		d.press("space")

		if got := d.screen(); got != before {
			t.Errorf("%s: space changed the screen to\n%s\nfrom\n%s", c.name, got, before)
		}
		if got := e.targetArgs("zed"); got != nil {
			t.Errorf("%s: space ran zed with %q", c.name, got)
		}
	}
}
