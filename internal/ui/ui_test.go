package ui_test

import (
	"testing"
)

func TestListsReposUnderCodeWithNameAndShortenedFolder(t *testing.T) {
	e := newEnv(t)
	e.repo("code/aycapi")
	e.repo("code/work/backend")

	screen := e.start().screen()

	for _, row := range []string{"aycapi ~/code main", "backend ~/code/work main"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q:\n%s", row, screen)
		}
	}
}

func TestFirstRowIsSelectedAndKeysMoveTheSelectionWithinTheList(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.repo("code/gamma")
	d := e.start()

	steps := []struct {
		keys []string
		want string
	}{
		{nil, "alpha ~/code main"},
		{[]string{"j"}, "beta ~/code main"},
		{[]string{"down"}, "gamma ~/code main"},
		{[]string{"j", "down"}, "gamma ~/code main"},
		{[]string{"k"}, "beta ~/code main"},
		{[]string{"up"}, "alpha ~/code main"},
		{[]string{"k", "up"}, "alpha ~/code main"},
	}
	for _, s := range steps {
		d.press(s.keys...)
		if got := d.selected(); got != s.want {
			t.Fatalf("after %v: selected %q, want %q", s.keys, got, s.want)
		}
	}
}

func TestEnterOpensTheSelectedRepoAndStaysOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	d := e.start()

	d.press("j", "enter")

	if got := e.targetArgs(); len(got) != 1 || got[0] != beta {
		t.Errorf("target called with %q, want [%q]", got, beta)
	}
	if d.quit {
		t.Error("the tool quit after opening a repo")
	}
	if got := d.selected(); got != "beta ~/code main" {
		t.Errorf("selected %q after opening, want the list still on beta", got)
	}
}

func TestKeysOnAnEmptyListDoNothing(t *testing.T) {
	e := newEnv(t)
	d := e.start() // no repos: the list is empty

	d.press("j", "k", "enter")

	if got := e.targetArgs(); got != nil {
		t.Errorf("target called with %q on an empty list", got)
	}
}

func TestQAndCtrlCQuit(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()

		d.press(key)

		if !d.quit {
			t.Errorf("%s did not quit", key)
		}
	}
}
