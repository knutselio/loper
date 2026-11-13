package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// red is a colour sequence in a file name, and redShown how it is drawn.
const red, redShown = "\x1b[31mred", `\x1b[31mred`

// shown reports whether the screen of d, or its dialog with the spaces left
// out as a wrapped path breaks across rows, shows want and no escape code.
func shown(d *driver, want string) bool {
	screen := d.screen()
	return !strings.Contains(screen, "\x1b") &&
		(strings.Contains(screen, want) || strings.Contains(strings.ReplaceAll(dialogText(d), " ", ""), strings.ReplaceAll(want, " ", "")))
}

// redConfig names a config file in a folder in the home whose name holds a
// colour sequence, writes body to it and returns the file and how it is drawn.
func redConfig(e *env, body string) (file, drawn string) {
	file = filepath.Join(e.home, "x"+red, "c.toml")
	e.configFile = file
	e.writeFile(file, body)
	return file, "~/x" + redShown + "/c.toml"
}

func TestTheConfigFileIsDrawnEscapedWhereNoReposAreFound(t *testing.T) {
	e := newEnv(t)
	_, drawn := redConfig(e, "paths = []\n")
	d := e.start()

	if want := "Change paths in " + drawn; !shown(d, want) {
		t.Errorf("want %q and no escape code:\n%q", want, d.screen())
	}
}

func TestTheConfigFileIsDrawnEscapedInTheAddDialog(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	_, drawn := redConfig(e, zedConfig)
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")

	if want := "Append to " + drawn; !shown(d, want) {
		t.Errorf("want %q and no escape code: %q", want, dialogText(d))
	}
}

func TestALinkToNothingIsDrawnEscapedInTheRefusal(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	file, drawn := redConfig(e, zedConfig)
	d := e.start()
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gone"+red, file); err != nil {
		t.Fatal(err)
	}

	d.press("a")

	if want := drawn + " is a link to gone" + redShown + ", which does not exist"; !shown(d, want) {
		t.Errorf("want %q and no escape code: %q", want, dialogText(d))
	}
}

func TestALoaderProblemIsDrawnEscapedWithItsQuotedValuesEscapedOnce(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	file, drawn := redConfig(e, zedConfig)
	d := e.start()

	d.press("a")
	e.writeFile(file, "\"a\\\\b\" = 1\n"+zedConfig) // a setting named a\b
	d.typeText("Tab")
	d.press("tab", "t", "tab")
	d.typeText("tab {path}")
	d.press("enter", "enter")

	if want := drawn + `:1: unknown setting "a\\b"`; !shown(d, want) {
		t.Errorf("want %q and no escape code: %q", want, dialogText(d))
	}
}

func TestAFirstRunNoticeIsDrawnEscaped(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.picker = true
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gone"+red, configPath()); err != nil {
		t.Fatal(err)
	}
	d := e.start()

	if want := "is a link to gone" + redShown; !shown(d, want) {
		t.Errorf("want %q and no escape code:\n%q", want, d.screen())
	}
}
