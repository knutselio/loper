package ui_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// boxWidths returns the width of each box on screen, its borders included.
func boxWidths(d *driver) []int {
	var widths []int
	for _, line := range strings.Split(d.screen(), "\n") {
		if i, j := strings.Index(line, "╭"), strings.Index(line, "╮"); i >= 0 && j > i {
			widths = append(widths, ansi.StringWidth(line[i:j+len("╮")]))
		}
	}
	return widths
}

func TestTheSentenceAboveTheLinesIsBoldAndTheLinesAreNot(t *testing.T) {
	file := "~/.config/loper/config.toml"
	for _, c := range []struct {
		what      string
		do        func(d *driver)
		sentences []string
	}{
		{"add", func(d *driver) { d.fill("Vim", "v", "vim {path}"); d.press("enter", "enter") },
			[]string{"Append to " + file}},
		{"edit", func(d *driver) { d.press("enter", "e"); d.retype("Zedd"); d.toLast(); d.press("enter") },
			[]string{"Replace these lines in " + file + ":", "with these (comments inside the block are not kept):"}},
		{"remove", func(d *driver) { d.press("enter", "d") },
			[]string{"Remove these lines from " + file + ":"}},
	} {
		e := newEnv(t)
		e.noColor = true // bold stays without colour
		d, _ := editStart(e, editConfig)

		c.do(d)

		screen := d.screen()
		for _, s := range c.sentences {
			if !strings.Contains(screen, "\x1b[1m"+s+"\x1b[22m") {
				t.Errorf("%s: %q is not bold:\n%q", c.what, s, screen)
			}
		}
		// Every other row is not bold, but for the keys of the hint.
		for _, line := range strings.Split(screen, "\n") {
			first, last := strings.Index(line, "│"), strings.LastIndex(line, "│")
			if first < 0 || last <= first {
				continue
			}
			row := line[first:last]
			if !slices.ContainsFunc(c.sentences, func(s string) bool { return strings.Contains(row, s) }) &&
				!strings.Contains(row, "enter\x1b[22m") && strings.Contains(row, "\x1b[1m") {
				t.Errorf("%s: a row other than the sentence is bold: %q", c.what, row)
			}
		}
	}
}

func TestTheErrorDialogShowsTheHomeFolderAsTildeAndRunsTheRealPath(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	// Beside the home, its rest, with and without a space: two folders that
	// start with the home folder's text but are others.
	beside, quote, spaced := e.home+"2/x", e.home+"'x/y", e.home+"/my code/x"
	e.script("broken", "printf '%s\\n' \"$@\" > \"$LOPER_TEST_RECORDS\"/broken.args\nexit 1\n")
	e.config(`[[targets]]
name = "Broken"
key = "b"
command = """broken '` + e.home + `' {path} '` + beside + `' "` + quote + `" '` + spaced + `'"""
`)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("b")

	// A backslash shows as \\, as everywhere in the command line.
	command := "$ broken ~ ~/code/alpha '" + beside + "' '" + strings.ReplaceAll(quote, "'", `'\\''`) + "' ~/'my code/x'"
	if got, want := boxRows(d), []string{command, "exit status 1"}; !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
	if got, want := e.targetArgs("broken"), []string{e.home, alpha, beside, quote, spaced}; !slices.Equal(got, want) {
		t.Errorf("the target ran with %q, want the real paths %q", got, want)
	}
}

func TestAHomeWithAQuoteInItsNameShowsAsTildeInTheErrorDialog(t *testing.T) {
	e := newEnv(t)
	e.home = filepath.Join(e.root, "it's home")
	t.Setenv("HOME", e.home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(e.home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(e.home, ".local", "state"))
	e.repo("code/alpha")
	e.script("broken", "exit 1\n")
	e.config("[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"broken {path}\"\n")
	d := e.start()

	d.press("b")

	if got, want := boxRows(d), []string{"$ broken ~/code/alpha", "exit status 1"}; !slices.Equal(got, want) {
		t.Errorf("dialog reads %q, want %q:\n%s", got, want, d.screen())
	}
}

func TestNoDialogButTheErrorDialogIsWiderThan100Columns(t *testing.T) {
	long := strings.Repeat("long ", 30) + "name"
	for _, c := range []struct {
		what string
		keys []string
		// at80 and at200 are how wide it is at 80 columns, as before the
		// limit, and at 200.
		at80, at200 int
	}{
		{"menu", []string{"enter"}, 80, 100},
		{"help", []string{"?"}, 80, 100},
		{"error dialog", []string{"b"}, 80, 200}, // as wide as its lines, up to the screen
		{"add dialog", []string{"enter", "esc", "a"}, 62, 62},
		{"edit dialog", []string{"enter", "e"}, 62, 62},
		{"remove dialog", []string{"enter", "d"}, 62, 62},
	} {
		for _, width := range []int{80, 200} {
			e := newEnv(t)
			e.repo("code/alpha")
			script := e.script("broken", "printf '%0300d\\n' 0\nexit 1\n")
			e.config("[[targets]]\nname = \"" + long + "\"\nkey = \"b\"\ncommand = \"" + script + " {path}\"\n")
			d := startQuiet(e)
			d.send(tea.WindowSizeMsg{Width: width, Height: 40})

			d.press(c.keys...)

			want := c.at80
			if width == 200 {
				want = c.at200
			}
			if got := boxWidths(d); !slices.Equal(got, []int{want}) {
				t.Errorf("%s at %d columns: the box is %v wide, want %d:\n%s", c.what, width, got, want, d.screen())
			}
		}
	}
}
