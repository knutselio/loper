package ui_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// cdBlock is the config file a first run creates: the target cd, key o.
const cdBlock = "[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}\"\nin_place = true\n"

// cdFooter is the footer with the cd target.
const cdFooter = "enter open | / search | s sort | r refresh | ? help | q quit | o cd | a add"

func TestAFirstRunCreatesTheConfigFileAndTheFooterShowsTheCdTarget(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")

	d := e.start()

	if got := footer(d); got != cdFooter {
		t.Errorf("footer reads %q, want %q", got, cdFooter)
	}
	if got, _ := fileText(t, configPath()); got != cdBlock {
		t.Errorf("config is %q, want %q", got, cdBlock)
	}
}

func TestTheCdTargetHandsOverToTheShellInTheRepoAndRecordsTheOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.config(cdBlock)
	shell := filepath.Join(e.root, "bin", "fake shell")
	t.Setenv("SHELL", shell)
	d := e.start()

	d.press("j", "o")

	if !d.quit {
		t.Errorf("the UI did not end:\n%s", d.screen())
	}
	if program, words, dir, ok := handOver(d); !ok || program != shell || !slices.Equal(words, []string{shell}) || dir != beta {
		t.Errorf("hands over %q as %q in %q (%v), want %q in %q", program, words, dir, ok, shell, beta)
	}
	if got := e.start().selected(); !startsWith(got, "beta") {
		t.Errorf("on the next start the top row is %q, want beta, opened last", got)
	}
}

func TestTheCdTargetHandsOverToBinShWhenShellIsNotSet(t *testing.T) {
	for _, unset := range []bool{true, false} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.config(cdBlock)
		t.Setenv("SHELL", "")
		if unset {
			os.Unsetenv("SHELL")
		}
		d := e.start()

		d.press("o")

		if program, words, dir, ok := handOver(d); !d.quit || !ok || program != "/bin/sh" || !slices.Equal(words, []string{"/bin/sh"}) || dir != alpha {
			t.Errorf("SHELL unset %v: hands over %q as %q in %q (%v, ended %v), want /bin/sh in %q", unset, program, words, dir, ok, d.quit, alpha)
		}
	}
}

func TestTheCdTargetOnAFolderThatIsGoneShowsTheDialogAndRecordsNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.config(cdBlock)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})
	if err := os.RemoveAll(beta); err != nil {
		t.Fatal(err)
	}

	d.press("j", "o")

	want := []string{"$ cd '" + beta + "'", "stat " + beta + ": no such file or directory"}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
	if _, _, _, ok := handOver(d); ok || d.quit {
		t.Errorf("asks for a hand-over (%v) or ended the UI (%v)", ok, d.quit)
	}
	if h := savedHistory(t, os.Getenv("XDG_STATE_HOME")); h != nil {
		t.Errorf("the history holds %v, want nothing recorded", h)
	}
}

func TestATargetWhoseProgramIsNotCdHandsOverAsAnyOther(t *testing.T) {
	if _, err := os.Stat("/usr/bin/cd"); err != nil {
		t.Skip("no /usr/bin/cd here")
	}
	for _, c := range []struct {
		command, program string
		words            func(alpha string) []string
	}{
		{"cdx {path}", "", func(alpha string) []string { return []string{"cdx", alpha} }},
		{"/usr/bin/cd {path}", "/usr/bin/cd", func(alpha string) []string { return []string{"/usr/bin/cd", alpha} }},
	} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.fakeTarget("cdx")
		if c.program == "" {
			c.program = filepath.Join(e.root, "bin", "cdx")
		}
		e.config("[[targets]]\nname = \"X\"\nkey = \"o\"\ncommand = \"" + c.command + "\"\nin_place = true\n")
		d := e.start()

		d.press("o")

		if program, words, dir, ok := handOver(d); !ok || program != c.program || !slices.Equal(words, c.words(alpha)) || dir != alpha {
			t.Errorf("%s: hands over %q as %q in %q (%v), want %q as %q", c.command, program, words, dir, ok, c.program, c.words(alpha))
		}
	}
}

func TestTheAddDialogRefusesACdCommandForTheLoadersReasons(t *testing.T) {
	for _, c := range []struct {
		command string
		inPlace bool
		reason  string
	}{
		{"cd {path}", false, `target "Go" runs cd, which needs in_place = true`},
		{"cd", true, `target command "cd" needs one folder after cd`},
		{"cd {path} extra", true, `target command "cd {path} extra" needs one folder after cd`},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()

		d.fill("Go", "c", c.command)
		d.press("tab")
		if c.inPlace {
			d.press("left")
		}
		d.press("enter")

		if text := dialogText(d); !strings.Contains(text, c.reason) {
			t.Errorf("%s: dialog reads %q, want the reason %q", c.command, text, c.reason)
		}
		if got, _ := fileText(t, configPath()); got != "" {
			t.Errorf("%s: config is %q, want nothing written", c.command, got)
		}
	}
}

func TestTheCdTargetIsEditedAndRemovedLikeAnyOtherAndStaysRemoved(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")
	d := e.start()

	d.press("enter", "e")
	d.retype("Here")
	d.press("tab")
	d.retype("h")
	d.press("enter", "enter")
	d.confirmChoice()

	if got, _ := fileText(t, configPath()); got != "[[targets]]\nname = 'Here'\nkey = 'h'\ncommand = 'cd {path}'\nin_place = true\n" {
		t.Errorf("after the edit the config is %q, want the block named Here with key h", got)
	}
	if got, want := footer(d), "Changed Here"; got != want {
		t.Errorf("after the edit the footer reads %q, want %q", got, want)
	}
	d.press("esc", "enter", "d")
	d.pass()
	d.press("enter")
	if got, _ := fileText(t, configPath()); got != "" {
		t.Errorf("after the removal the config is %q, want it empty", got)
	}

	d = e.start()
	if got, want := footer(d), "enter open | / search | s sort | r refresh | ? help | q quit | a add"; got != want {
		t.Errorf("on the next run the footer reads %q, want no target, %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != "" {
		t.Errorf("on the next run the config is %q, want it still empty", got)
	}
}

func TestAConfigFileThatCannotBeCreatedShowsWhyAndTheCdTarget(t *testing.T) {
	e := newEnv(t)
	e.picker = true
	e.repo("code/alpha")
	folder := filepath.Dir(configPath())
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(folder, 0o700) })

	d := e.start()

	if got, want := footer(d), "The config file could not be written: permission denied"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	for d.tick() {
	}
	if got := footer(d); got != cdFooter {
		t.Errorf("once the notice has gone the footer reads %q, want %q", got, cdFooter)
	}
	if _, ok := fileText(t, configPath()); ok {
		t.Errorf("a config file was written")
	}
}

func TestTheCdTargetHandsOverInTheFolderItsArgumentNames(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	sub := filepath.Join(alpha, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e.config("[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}/sub\"\nin_place = true\n")
	d := e.start()

	d.press("o")

	if _, _, dir, ok := handOver(d); !ok || dir != sub {
		t.Errorf("hands over in %q (%v), want %q", dir, ok, sub)
	}
}

func TestTheCdTargetOnAFileShowsTheDialogAndRecordsNothing(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	file := filepath.Join(alpha, "notes")
	e.writeFile(file, "")
	e.config("[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}/notes\"\nin_place = true\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("o")

	want := []string{"$ cd '" + file + "'", file + " is not a folder"}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
	if _, _, _, ok := handOver(d); ok || d.quit {
		t.Errorf("asks for a hand-over (%v) or ended the UI (%v)", ok, d.quit)
	}
	if h := savedHistory(t, os.Getenv("XDG_STATE_HOME")); h != nil {
		t.Errorf("the history holds %v, want nothing recorded", h)
	}
}
