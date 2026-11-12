package ui_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/ui"
)

// lazygitTarget is a config's target Lazygit, key l, which takes over the
// window with the fake lazygit.
const lazygitTarget = "\n[[targets]]\nname = \"Lazygit\"\nkey = \"l\"\ncommand = \"lazygit -p {path}\"\nin_place = true\n"

// handOver returns what the program asks the entry point to run in its
// place once it has ended: the program, its words and its folder.
func handOver(d *driver) (program string, words []string, dir string, ok bool) {
	h, ok := d.model.(ui.Model).HandOver()
	return h.Program, h.Args, h.Dir, ok
}

func TestAnInPlaceTargetsKeyEndsTheUIAndHandsOverTheCommandInTheRepo(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.fakeTarget("lazygit")
	e.config(lazygitTarget)
	d := e.start()

	d.press("l")

	if !d.quit {
		t.Errorf("the UI did not end:\n%s", d.screen())
	}
	program, words, dir, ok := handOver(d)
	if !ok {
		t.Fatal("the UI asks for no hand-over")
	}
	if want := filepath.Join(e.root, "bin", "lazygit"); program != want {
		t.Errorf("hands over the program %q, want %q", program, want)
	}
	if want := []string{"lazygit", "-p", alpha}; !slices.Equal(words, want) {
		t.Errorf("hands over the words %q, want %q", words, want)
	}
	if dir != alpha {
		t.Errorf("hands over in %q, want %q", dir, alpha)
	}
	if got := e.targetArgs("lazygit"); got != nil {
		t.Errorf("lazygit ran inside the tool with %q; it should only be handed over", got)
	}
}

func TestAnInPlaceTargetChosenFromTheMenuHandsOverTheSame(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.fakeTarget("lazygit")
	e.config(zedTarget + lazygitTarget)
	d := e.start()

	d.press("j", "enter", "j", "enter")

	if !d.quit {
		t.Errorf("the UI did not end:\n%s", d.screen())
	}
	if _, words, dir, ok := handOver(d); !ok || !slices.Equal(words, []string{"lazygit", "-p", beta}) || dir != beta {
		t.Errorf("hands over %q in %q (%v), want lazygit -p %s in it", words, dir, ok, beta)
	}
}

func TestAnInPlaceOpenIsRecordedSoTheRepoIsFirstOnTheNextStart(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.fakeTarget("lazygit")
	e.config(lazygitTarget)
	d := e.start()

	d.press("j", "l")
	if !d.quit {
		t.Fatalf("the UI did not end:\n%s", d.screen())
	}

	if got := e.start().selected(); !startsWith(got, "beta") {
		t.Errorf("on the next start the top row is %q, want beta, opened last", got)
	}
}

func TestAnInPlaceTargetWhoseProgramDoesNotExistShowsTheDialogAndRecordsNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.config("\n[[targets]]\nname = \"Missing\"\nkey = \"m\"\ncommand = \"no-such-program {path}\"\nin_place = true\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("j", "m")

	want := []string{"$ no-such-program '" + beta + "'", `exec: "no-such-program": executable file not found in $PATH`}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
	if d.quit {
		t.Errorf("the UI ended")
	}
	if _, _, _, ok := handOver(d); ok {
		t.Errorf("the UI asks for a hand-over")
	}
	if got := e.start().selected(); !startsWith(got, "alpha") {
		t.Errorf("on the next start the top row is %q, want alpha, as nothing was recorded", got)
	}
}

func TestAnInPlaceTargetsRelativeProgramIsFoundInTheRepo(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	program := filepath.Join(alpha, "tools", "lg")
	e.writeFile(program, "#!/bin/sh\n")
	if err := os.Chmod(program, 0o755); err != nil {
		t.Fatal(err)
	}
	e.config("\n[[targets]]\nname = \"Local\"\nkey = \"o\"\ncommand = \"./tools/lg {path}\"\nin_place = true\n")
	d := e.start()

	d.press("o")

	if got, words, dir, ok := handOver(d); !ok || got != program || !slices.Equal(words, []string{"./tools/lg", alpha}) || dir != alpha {
		t.Errorf("hands over %q as %q in %q (%v), want %q as ./tools/lg in %q:\n%s", got, words, dir, ok, program, alpha, d.screen())
	}
}

func TestAddingWithTakeOverWritesInPlaceAndNoCloseAndTheTargetHandsOverAtOnce(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.fakeTarget("lazygit")
	d := e.start()
	d.fill("Lazygit", "l", "lazygit -p {path}")
	d.press("tab", "left")
	if got, want := boxRows(d), "> After open ‹ take over this window ›"; !slices.Contains(got, want) {
		t.Fatalf("dialog reads %q, want the row %q", got, want)
	}

	d.confirmChoice()

	if got, _ := fileText(t, configPath()); got != "[[targets]]\nname = 'Lazygit'\nkey = 'l'\ncommand = 'lazygit -p {path}'\nin_place = true\n" {
		t.Errorf("config is %q, want the block with in_place = true and no close", got)
	}
	d.press("l")
	if _, words, dir, ok := handOver(d); !d.quit || !ok || !slices.Equal(words, []string{"lazygit", "-p", alpha}) || dir != alpha {
		t.Errorf("l hands over %q in %q (%v, ended %v), want lazygit -p %s in it", words, dir, ok, d.quit, alpha)
	}
}

// lazygitBlock is a config file with only an in-place target, Lazygit.
const lazygitBlock = "[[targets]]\nname = \"Lazygit\"\nkey = \"l\"\ncommand = \"lazygit -p {path}\"\nin_place = true\n"

func TestAnEditOfAnInPlaceTargetStartsOnTakeOverAndKeepsTheLine(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, lazygitBlock)

	d.press("enter", "e")
	if got, want := boxRows(d), "After open ‹ take over this window ›"; !slices.Contains(got, want) {
		t.Fatalf("dialog reads %q, want the row %q", got, want)
	}
	d.retype("Lazygit two")
	d.toLast()
	d.press("enter")

	want := []string{
		"Replace these lines in ~/.config/loper/config.toml:",
		"[[targets]]", `name = "Lazygit"`, `key = "l"`, `command = "lazygit -p {path}"`, "in_place = true",
		"",
		"with these (comments inside the block are not kept):",
		"[[targets]]", "name = 'Lazygit two'", "key = 'l'", "command = 'lazygit -p {path}'", "in_place = true",
		"",
		"enter write esc back",
	}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("confirmation reads %q, want %q", got, want)
	}
}

func TestAnEditOfAnInPlaceTargetThatChangesNothingWritesNothing(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, lazygitBlock)

	d.press("enter", "e")
	d.toLast()
	d.press("enter")

	if got, want := boxRows(d), []string{"> l Lazygit"}; !slices.Equal(got, want) {
		t.Errorf("box reads %q, want the menu back, %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != lazygitBlock {
		t.Errorf("config is %q, want it byte for byte as it was", got)
	}
}

func TestChoosingStayOpenForAnInPlaceTargetRemovesTheLineAndWritesCloseFalse(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, lazygitBlock)

	d.press("enter", "e")
	d.toLast()
	d.press("right", "right")
	d.confirmChoice()

	if got, _ := fileText(t, configPath()); got != "[[targets]]\nname = 'Lazygit'\nkey = 'l'\ncommand = 'lazygit -p {path}'\nclose = false\n" {
		t.Errorf("config is %q, want close = false and no in_place line", got)
	}
	d.press("l")
	if d.quit {
		t.Errorf("l ended the UI; the target should now stay open")
	}
	if _, _, _, ok := handOver(d); ok {
		t.Errorf("l asks for a hand-over; the target is no longer in place")
	}
}

func TestTwoBlocksThatDifferOnlyInInPlaceAreEachEditedAndRemoved(t *testing.T) {
	const first = "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\n"
	const second = "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\nin_place = true\n"
	const body = "# one\n" + first + "\n# two\n" + second + "# end\n"
	for _, c := range []struct {
		at     int
		remove bool
		want   string
	}{
		{0, false, "# one\n[[targets]]\nname = 'X'\ncommand = 'same {path}'\n\n# two\n" + second + "# end\n"},
		{1, false, "# one\n" + first + "\n# two\n[[targets]]\nname = 'X'\ncommand = 'same {path}'\nin_place = true\n# end\n"},
		{0, true, "# one\n# two\n" + second + "# end\n"},
		{1, true, "# one\n" + first + "\n# two\n# end\n"},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(body)
		d := e.start()
		d.press("enter")
		d.press(slices.Repeat([]string{"j"}, c.at)...)

		if c.remove {
			d.press("d")
		} else {
			d.press("e")
			d.retype("X")
			d.toLast()
			d.press("enter")
		}
		if !strings.Contains(dialogText(d), "these lines") {
			t.Errorf("target %d, remove %v: no confirmation:\n%s", c.at, c.remove, d.screen())
			continue
		}
		d.pass()
		d.press("enter")

		if got, _ := fileText(t, configPath()); got != c.want {
			t.Errorf("target %d, remove %v: config is\n%q\nwant\n%q\n%s", c.at, c.remove, got, c.want, d.screen())
		}
	}
}

func TestAnInPlaceTargetsKeyDoesNothingWhileAnotherOpenIsInItsWait(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.fakeTarget("lazygit")
	e.config(zedTarget + lazygitTarget)
	d := e.start()

	d.hold = true // zed's open stays in its wait until release
	d.press("z", "j", "l")
	d.release()

	if _, _, _, ok := handOver(d); ok || d.quit {
		t.Errorf("l during zed's open asks for a hand-over (%v) or ended the UI (%v)", ok, d.quit)
	}
	if got, want := footer(d), "Opened alpha in Zed"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	if screen := e.start().screen(); !hasExactRow(screen, "beta ~/code main -") {
		t.Errorf("beta was recorded; want only zed's open of alpha:\n%s", screen)
	}
}
