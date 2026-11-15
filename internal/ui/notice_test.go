package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// keysFooter is the footer of a config with only the target Zed.
const keysFooter = "enter open • z Zed • ? help • q quit"

// startQuiet is start with the launch spinner's last tick delivered, so the
// only ticks still to come are those of notices.
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
	if got, want := topRight(d), "Opened alpha in Zed"; got != want {
		t.Fatalf("after opening: top right reads %q, want %q", got, want)
	}
	if got := footer(d); got != keysFooter {
		t.Errorf("after opening: footer reads %q, want the keys %q", got, keysFooter)
	}
	if len(d.ticks) != 1 {
		t.Fatalf("after opening: %d ticks pending, want 1", len(d.ticks))
	}
	if got := d.ticks[0].after; got != e.notice {
		t.Errorf("the confirmation shows for %v, want %v", got, e.notice)
	}

	d.tick()
	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("after the moment: top right reads %q, want %q", got, want)
	}
}

func TestTheConfirmationOfAnAddedTargetClearsByItselfAfterAMoment(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedTarget)
	d := startQuiet(e)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 24}) // room for every item

	d.fill("Tab", "t", "tab {path}")
	d.confirm()
	if got, want := topRight(d), "Added Tab"; got != want {
		t.Fatalf("after adding: top right reads %q, want %q", got, want)
	}
	if got, want := footer(d), "enter open • z Zed • t Tab • ? help • q quit"; got != want {
		t.Errorf("after adding: footer reads %q, want %q", got, want)
	}

	d.tick()
	if got, want := topRight(d), "1 repo"; got != want {
		t.Errorf("after the moment: top right reads %q, want %q", got, want)
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
	if got, want := topRight(d), "Opened beta in Zed"; got != want {
		t.Errorf("after the first moment: top right reads %q, want %q", got, want)
	}

	d.tick()
	if got, want := topRight(d), "2 repos"; got != want {
		t.Errorf("after the second moment: top right reads %q, want %q", got, want)
	}
}

func TestANoticeHidesAProblemForItsMomentAndTheFooterKeepsTheKeys(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/typo"]` + zedTarget)
	d := startQuiet(e)
	if got, want := topRight(d), "! ~/typo not found"; got != want {
		t.Fatalf("before the open: top right reads %q, want %q", got, want)
	}

	d.press("z")
	if got, want := topRight(d), "Opened alpha in Zed"; got != want {
		t.Errorf("after opening: top right reads %q, want %q", got, want)
	}
	if got := footer(d); got != keysFooter {
		t.Errorf("after opening: footer reads %q, want the keys %q", got, keysFooter)
	}

	d.tick()
	if got, want := topRight(d), "! ~/typo not found"; got != want {
		t.Errorf("after the moment: top right reads %q, want the problem back, %q", got, want)
	}
	if got := footer(d); got != keysFooter {
		t.Errorf("after the moment: footer reads %q, want the keys %q", got, keysFooter)
	}
}

func TestANoticeShowsOverTheSpinnerAndTheSpinnerOverAProblem(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/typo"]` + zedTarget)
	d := e.start()
	d.hold = true
	d.press("r") // the refresh runs until released
	if got, want := topRight(d), "⠋ refreshing"; got != want {
		t.Errorf("while refreshing: top right reads %q, want %q", got, want)
	}

	d.hold = false
	d.press("z")
	if got, want := topRight(d), "Opened alpha in Zed"; got != want {
		t.Errorf("after opening while refreshing: top right reads %q, want %q", got, want)
	}
	d.press("j") // the next key ends the notice
	if got, want := topRight(d), "⠋ refreshing"; got != want {
		t.Errorf("after the next key: top right reads %q, want %q", got, want)
	}
}

func TestWithColourTheSpinnerAndRefreshingAreDimmed(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	d := e.start()
	d.hold = true
	d.press("r") // the refresh runs until released

	if got, want := styled(strings.Split(d.screen(), "\n")[0]), " {⠋ refreshing} "; !strings.HasSuffix(got, want) {
		t.Errorf("top row reads %q, want it to end with %q", got, want)
	}
}

func TestTheTopRowAndTheFooterLeaveTheLastCellBlankAtEveryWidth(t *testing.T) {
	for _, s := range []struct {
		name string
		keys []string
	}{
		{"a problem", nil},
		{"a notice", []string{"z"}},
		{"a long query", []string{"/", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n"}},
	} {
		e := newEnv(t)
		sweepTree(e, "")
		d := e.start()
		d.press(s.keys...)
		for w := 0; w <= 120; w++ {
			d.send(tea.WindowSizeMsg{Width: w, Height: 24})
			lines := strings.Split(d.screen(), "\n")
			for _, line := range []string{lines[0], lines[len(lines)-1]} {
				if plain := ansi.Strip(line); ansi.StringWidth(plain) >= w && !strings.HasSuffix(plain, " ") && w > 0 {
					t.Errorf("%s, width %d: %q reaches the last cell", s.name, w, plain)
				}
			}
		}
	}
}
