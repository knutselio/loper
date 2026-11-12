package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// editConfig is a config with comments around and inside two targets, the
// fake zed and the fake tab.
const editConfig = "# my targets\n[[targets]]\nname = \"Zed\" # the editor\nkey = \"z\"\ncommand = \"zed {path}\"\n\n# a tab\n[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n# below\n"

// editStart makes a repo alpha, writes body as the config with the fake
// targets zed and tab on the PATH, and starts the program. It returns alpha.
func editStart(e *env, body string) (*driver, string) {
	e.t.Helper()
	alpha := e.repo("code/alpha")
	e.fakeTarget("tab")
	e.config(body)
	return e.start(), alpha
}

// retype replaces what is in the dialog's current field with text.
func (d *driver) retype(text string) {
	d.t.Helper()
	d.press(slices.Repeat([]string{"backspace"}, 80)...)
	d.typeText(text)
}

// toLast moves from the dialog's first field to its last, as enter does.
func (d *driver) toLast() {
	d.t.Helper()
	d.press("enter", "enter", "enter")
}

// menuBottom is the bottom border of the box on screen, with its spacing
// collapsed, or "" when there is none.
func menuBottom(d *driver) string {
	for _, line := range strings.Split(d.screen(), "\n") {
		if strings.Contains(line, "╰") {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

func TestEInTheMenuOpensTheDialogFilledInWithTheHighlightedTargetAndEscChangesNothing(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.press("enter", "j", "e")

	if !strings.Contains(d.screen(), "╭─ Edit a target ─") {
		t.Fatalf("e shows no edit dialog:\n%s", d.screen())
	}
	if got, want := boxRows(d), []string{"> Name Tab", "Key t", "Command tab {path}"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("fields read %q, want %q", got, want)
	}
	d.press("esc")
	if got, want := boxRows(d), []string{"z Zed", "> t Tab"}; !slices.Equal(got, want) {
		t.Errorf("after esc: box reads %q, want the menu back as it was, %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}
}

func TestAnEditedNameKeyAndCommandAreWrittenAndWorkAtOnce(t *testing.T) {
	e := newEnv(t)
	d, alpha := editStart(e, editConfig)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 60}) // room for the whole footer and help

	d.press("enter", "j", "e")
	d.retype("Terminal")
	d.press("tab")
	d.retype("x")
	d.press("tab")
	d.retype("tab --new {path}")
	d.confirm()

	want := "# my targets\n[[targets]]\nname = \"Zed\" # the editor\nkey = \"z\"\ncommand = \"zed {path}\"\n\n# a tab\n[[targets]]\nname = 'Terminal'\nkey = 'x'\ncommand = 'tab --new {path}'\n# below\n"
	if got, _ := fileText(t, configPath()); got != want {
		t.Errorf("config is %q, want %q", got, want)
	}
	if got, want := boxRows(d), []string{"z Zed", "> x Terminal"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want %q with the edited target highlighted", got, want)
	}
	if got, want := footer(d), "Changed Terminal"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	d.press("esc", "?")
	if rows := boxRows(d); !slices.Contains(rows, "x Terminal") || slices.Contains(rows, "t Tab") {
		t.Errorf("help does not list x Terminal alone:\n%s", d.screen())
	}
	d.press("esc")
	if got := footer(d); !strings.HasSuffix(got, "| z Zed | x Terminal | a add") {
		t.Errorf("footer reads %q, want it to end with the edited target", got)
	}
	d.press("t")
	if got := e.targetArgs("tab"); got != nil {
		t.Errorf("the old key t still ran tab with %q", got)
	}
	d.press("x")
	if got, want := e.targetArgs("tab"), []string{"--new", alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestAnEditKeepingItsKeyIsNotADuplicateButAnotherTargetsKeyIs(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.press("enter", "j", "e")
	d.retype("Tab two")
	d.toLast()
	d.press("enter")
	if got := dialogText(d); !strings.Contains(got, "with these") {
		t.Fatalf("an edit keeping its key shows %q, want the confirmation", got)
	}

	d.press("esc", "up", "up")
	d.retype("z")
	d.press("enter", "enter", "enter")
	if got, want := dialogText(d), `target key "z" is already used by the target on line 2`; !strings.Contains(got, want) || !strings.Contains(got, "Key z") {
		t.Errorf("dialog reads %q, want the fields and the reason %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}

	d.press("esc", "k", "e", "tab") // Zed, whose key t is the key of the target after it
	d.retype("t")
	d.press("enter", "enter", "enter")
	if got, want := dialogText(d), `target key "t" is already used by the target on line 8`; !strings.Contains(got, want) {
		t.Errorf("dialog reads %q, want the reason %q, naming the other target's line", got, want)
	}
}

func TestTheEditConfirmationShowsTheOldAndTheNewBlockAndEscGoesBackToTheFields(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.press("enter", "e")
	d.retype("Zed two")
	d.toLast()
	d.press("enter")

	want := []string{
		"Replace these lines in ~/.config/loper/config.toml:",
		"[[targets]]", `name = "Zed" # the editor`, `key = "z"`, `command = "zed {path}"`,
		"",
		"with these (comments inside the block are not kept):",
		"[[targets]]", "name = 'Zed two'", "key = 'z'", "command = 'zed {path}'",
		"",
		"enter write esc back",
	}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("confirmation reads %q, want %q", got, want)
	}

	d.press("esc")
	if got, want := boxRows(d), []string{"Name Zed two", "Key z", "Command zed {path}", "> After open ‹ default (stay open) ›"}; len(got) < 4 || !slices.Equal(got[:4], want) {
		t.Errorf("after esc: dialog reads %q, want the fields with their contents %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}
}

func TestDShowsTheLinesToRemoveEscKeepsThemAndEnterRemovesTheTarget(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 60})

	d.press("enter", "d")
	want := []string{
		"Remove these lines from ~/.config/loper/config.toml:",
		"[[targets]]", `name = "Zed" # the editor`, `key = "z"`, `command = "zed {path}"`,
		"",
		"enter remove esc keep",
	}
	if got := boxRows(d); !slices.Equal(got, want) || !strings.Contains(d.screen(), "╭─ Remove a target ─") {
		t.Errorf("d shows %q, want %q:\n%s", got, want, d.screen())
	}
	d.pass()
	d.press("esc")
	if got, want := boxRows(d), []string{"> z Zed", "t Tab"}; !slices.Equal(got, want) {
		t.Errorf("after esc: box reads %q, want the menu %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("after esc: config is %q, want it unchanged", got)
	}

	d.press("d", "enter") // too quick: the confirmation has only just appeared
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("after a quick enter: config is %q, want it unchanged", got)
	}
	d.pass()
	d.press("enter")

	if got, _ := fileText(t, configPath()); got != "# my targets\n# a tab\n[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n# below\n" {
		t.Errorf("config is %q, want the Zed block gone and the rest as it was", got)
	}
	if got, want := boxRows(d), []string{"> t Tab"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want %q", got, want)
	}
	if got, want := footer(d), "Removed Zed"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	d.press("esc", "?")
	if rows := boxRows(d); slices.Contains(rows, "z Zed") {
		t.Errorf("help still lists z Zed:\n%s", d.screen())
	}
	d.press("esc")
	if got := footer(d); strings.Contains(got, "Zed") {
		t.Errorf("footer reads %q, want no Zed", got)
	}
	d.press("z")
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("z still ran zed with %q", got)
	}
}

func TestRemovingTheLastTargetLeavesTheNoTargetsState(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, "paths = [\"~/code\"]\n"+zedTarget)

	d.press("enter", "d")
	d.pass()
	d.press("enter")

	if got, _ := fileText(t, configPath()); got != "paths = [\"~/code\"]\n" {
		t.Errorf("config is %q, want only the setting", got)
	}
	if got := boxRows(d); got != nil {
		t.Errorf("box reads %q, want the menu closed", got)
	}
	if got, want := footer(d), "Removed Zed"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	d.press("enter")
	if got, want := boxRows(d), []string{noTargets}; !slices.Equal(got, want) {
		t.Errorf("enter shows %q, want %q", got, want)
	}
}

func TestAfterARemoveTheHighlightStaysInRange(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	config := ""
	for i := 1; i <= 7; i++ {
		config += fmt.Sprintf("[[targets]]\nname = \"Target %d\"\ncommand = \"target%d {path}\"\n", i, i)
	}
	e.config(config)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 80, Height: 12}) // 8 list lines: room for 6 targets, so the menu scrolls, and the dialog
	d.press("enter")
	d.press(slices.Repeat([]string{"j"}, 6)...)

	d.press("d")
	d.pass()
	d.press("enter")

	if got, want := boxRows(d), []string{"Target 1", "Target 2", "Target 3", "Target 4", "Target 5", "> Target 6"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want the new last target highlighted, %q", got, want)
	}
}

// Every case's file has the targets One, Two and Three, or One and Two, and
// the edit renames one of them X.
func TestAnEditOrRemoveChangesOnlyThatBlocksBytes(t *testing.T) {
	const comments = "# settings\npaths = [\"~/code\"]\n\n# first\n[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"\n\n" +
		"# second\n[[targets]]\n# inside\nname = \"Two\"\ncommand = \"two {path}\" # trailing\n\n" +
		"# third\n[[targets]]\ncommand = \"three {path}\"\nkey = \"3\"\nname = \"Three\"\n# below\n"
	const crlf = "paths = [\"~/code\"]\r\n\r\n[[targets]]\r\nname = \"One\"\r\nkey = \"1\"\r\ncommand = \"one {path}\"\r\n\r\n[[targets]]\r\nname = \"Two\"\r\ncommand = \"two {path}\"\r\n"
	const crlfNoFinal = "[[targets]]\r\nname = \"One\"\r\nkey = \"1\"\r\ncommand = \"one {path}\"\r\n\r\n[[targets]]\r\nname = \"Two\"\r\ncommand = \"two {path}\""
	const noFinal = "[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"\n\n[[targets]]\nname = \"Two\"\ncommand = \"two {path}\""
	const multiline = "[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"\"\"one {path}\n[[targets]]\nname = \"fake\"\n\"\"\"\n\n[[targets]]\nname = \"Two\"\ncommand = \"two {path}\"\n"
	for _, c := range []struct {
		name   string
		before string
		at     int    // the target edited or removed, in the menu
		edited string // the file with that target renamed X
		gone   string // the file with that target removed
	}{
		{"comments, first", comments, 0,
			strings.Replace(comments, "[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"\n", "[[targets]]\nname = 'X'\nkey = '1'\ncommand = 'one {path}'\n", 1),
			strings.Replace(comments, "[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"\n\n", "", 1)},
		{"comments, middle", comments, 1,
			strings.Replace(comments, "[[targets]]\n# inside\nname = \"Two\"\ncommand = \"two {path}\" # trailing\n", "[[targets]]\nname = 'X'\ncommand = 'two {path}'\n", 1),
			strings.Replace(comments, "[[targets]]\n# inside\nname = \"Two\"\ncommand = \"two {path}\" # trailing\n\n", "", 1)},
		{"comments, last, keys in another order", comments, 2,
			strings.Replace(comments, "[[targets]]\ncommand = \"three {path}\"\nkey = \"3\"\nname = \"Three\"\n", "[[targets]]\nname = 'X'\nkey = '3'\ncommand = 'three {path}'\n", 1),
			strings.Replace(comments, "[[targets]]\ncommand = \"three {path}\"\nkey = \"3\"\nname = \"Three\"\n", "", 1)},
		{"CRLF, first", crlf, 0,
			strings.Replace(crlf, "name = \"One\"\r\nkey = \"1\"\r\ncommand = \"one {path}\"", "name = 'X'\r\nkey = '1'\r\ncommand = 'one {path}'", 1),
			strings.Replace(crlf, "[[targets]]\r\nname = \"One\"\r\nkey = \"1\"\r\ncommand = \"one {path}\"\r\n\r\n", "", 1)},
		{"CRLF, last", crlf, 1,
			strings.Replace(crlf, "name = \"Two\"\r\ncommand = \"two {path}\"", "name = 'X'\r\ncommand = 'two {path}'", 1),
			strings.Replace(crlf, "\r\n[[targets]]\r\nname = \"Two\"\r\ncommand = \"two {path}\"\r\n", "", 1)},
		{"CRLF and no final newline, last", crlfNoFinal, 1,
			strings.Replace(crlfNoFinal, "name = \"Two\"\r\ncommand = \"two {path}\"", "name = 'X'\r\ncommand = 'two {path}'", 1),
			"[[targets]]\r\nname = \"One\"\r\nkey = \"1\"\r\ncommand = \"one {path}\"\r\n"},
		{"no final newline, first", noFinal, 0,
			strings.Replace(noFinal, "name = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"", "name = 'X'\nkey = '1'\ncommand = 'one {path}'", 1),
			"[[targets]]\nname = \"Two\"\ncommand = \"two {path}\""},
		{"no final newline, last", noFinal, 1,
			strings.Replace(noFinal, "name = \"Two\"\ncommand = \"two {path}\"", "name = 'X'\ncommand = 'two {path}'", 1),
			"[[targets]]\nname = \"One\"\nkey = \"1\"\ncommand = \"one {path}\"\n"},
		{"a multi-line string with [[targets]] in it, the block with it", multiline, 0,
			"[[targets]]\nname = 'X'\nkey = '1'\ncommand = \"one {path}\\n[[targets]]\\nname = \\\"fake\\\"\\n\"\n\n[[targets]]\nname = \"Two\"\ncommand = \"two {path}\"\n",
			"[[targets]]\nname = \"Two\"\ncommand = \"two {path}\"\n"},
		{"a multi-line string with [[targets]] in it, the block after it", multiline, 1,
			strings.Replace(multiline, "name = \"Two\"\ncommand = \"two {path}\"", "name = 'X'\ncommand = 'two {path}'", 1),
			strings.Replace(multiline, "\n[[targets]]\nname = \"Two\"\ncommand = \"two {path}\"\n", "", 1)},
	} {
		for _, remove := range []bool{false, true} {
			e := newEnv(t)
			e.repo("code/alpha")
			e.config(c.before)
			d := e.start()
			d.press("enter")
			d.press(slices.Repeat([]string{"j"}, c.at)...)

			want := c.edited
			if remove {
				want = c.gone
				d.press("d")
			} else {
				d.press("e")
				d.retype("X")
				d.toLast()
				d.press("enter")
			}
			d.pass()
			d.press("enter")

			if got, _ := fileText(t, configPath()); got != want {
				t.Errorf("%s, remove %v: config is\n%q\nwant\n%q\n%s", c.name, remove, got, want, d.screen())
			}
		}
	}
}

func TestAFileChangedAfterTheConfirmationIsNotWrittenWithAReason(t *testing.T) {
	without := strings.Replace(editConfig, "[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n", "", 1)
	withComment := strings.Replace(editConfig, "name = \"Tab\"\n", "name = \"Tab\" # new\n", 1)
	for _, c := range []struct {
		name, edited, reason string
	}{
		{"the target gone", without, `target "Tab" is no longer in the file`},
		{"a comment added inside its block", withComment, `the lines of target "Tab" changed in the file since they were shown`},
	} {
		for _, remove := range []bool{false, true} {
			e := newEnv(t)
			d, _ := editStart(e, editConfig)
			d.press("enter", "j")
			if remove {
				d.press("d")
			} else {
				d.press("e")
				d.retype("Terminal")
				d.toLast()
				d.press("enter")
			}
			d.pass()
			e.config(c.edited)

			d.press("enter")

			if got := dialogText(d); !strings.Contains(got, c.reason) {
				t.Errorf("%s, remove %v: dialog reads %q, want the reason %q", c.name, remove, got, c.reason)
			}
			if got, _ := fileText(t, configPath()); got != c.edited {
				t.Errorf("%s, remove %v: config is %q, want it as edited", c.name, remove, got)
			}
		}
	}
}

func TestTwoIdenticalTargetBlocksAreRefusedForAnEditOrARemove(t *testing.T) {
	const same = "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\n"
	for _, key := range []string{"e", "d"} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(same + "\n" + same)
		d := e.start()

		d.press("enter", key)

		const want = `the file has more than one target written as "Same"; change it there by hand`
		if got := dialogText(d); !strings.Contains(got, want) || strings.Contains(got, "Name") {
			t.Errorf("%s: dialog reads %q, want the reason %q and no fields", key, got, want)
		}
		d.press("enter")
		if got, _ := fileText(t, configPath()); got != same+"\n"+same {
			t.Errorf("%s: config is %q, want it unchanged", key, got)
		}
	}
}

func TestAnInlineTargetsListIsRefusedForAnEditOrARemove(t *testing.T) {
	const inline = "targets = [{ name = \"Zed\", key = \"z\", command = \"zed {path}\" }]\n"
	for _, key := range []string{"e", "d"} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(inline)
		d := e.start()

		d.press("enter", key)

		if got := dialogText(d); !strings.Contains(got, "lists its targets inline") {
			t.Errorf("%s: dialog reads %q, want the refusal", key, got)
		}
		d.press("enter")
		if got, _ := fileText(t, configPath()); got != inline {
			t.Errorf("%s: config is %q, want it unchanged", key, got)
		}
	}
}

func TestEnterAgainWhileAnEditOrRemoveIsWrittenWritesOnce(t *testing.T) {
	for _, remove := range []bool{false, true} {
		e := newEnv(t)
		d, _ := editStart(e, editConfig)
		d.press("enter", "j")
		want := strings.Replace(editConfig, "name = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n", "name = 'Terminal'\nkey = 't'\ncommand = 'tab {path}'\n", 1)
		if remove {
			d.press("d")
			want = strings.Replace(editConfig, "[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n", "", 1)
		} else {
			d.press("e")
			d.retype("Terminal")
			d.toLast()
			d.press("enter")
		}
		d.pass()

		d.hold = true
		d.press("enter", "enter")
		d.release()

		if got, _ := fileText(t, configPath()); got != want {
			t.Errorf("remove %v: config is %q, want %q", remove, got, want)
		}
		if got := boxRows(d); !slices.ContainsFunc(got, func(r string) bool { return strings.HasSuffix(r, "z Zed") }) {
			t.Errorf("remove %v: box reads %q, want the menu back, with no refusal", remove, got)
		}
	}
}

func TestACheckForAnEarlierDialogIsIgnored(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.hold = true
	d.press("enter", "d", "esc", "esc", "a") // the lines to remove arrive at the add dialog
	d.release()

	if got := boxRows(d); len(got) == 0 || got[0] != "> Name" || !strings.Contains(d.screen(), "╭─ Add a target ─") {
		t.Errorf("dialog reads %q, want the add dialog's fields:\n%s", got, d.screen())
	}
}

func TestAnEditOrRemoveKeepsASymlinkTheFilesModeAndWritesOnlyTheConfigFlagsFile(t *testing.T) {
	for _, remove := range []bool{false, true} {
		e := newEnv(t)
		e.repo("code/alpha")
		real := filepath.Join(e.root, "dotfiles", "loper.toml")
		e.writeFile(real, editConfig)
		if err := os.Chmod(real, 0o640); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(e.root, "other.toml")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		e.configFile = link
		d := e.start()

		d.press("enter")
		want := strings.Replace(editConfig, "[[targets]]\nname = \"Zed\" # the editor\nkey = \"z\"\ncommand = \"zed {path}\"\n", "[[targets]]\nname = 'Editor'\nkey = 'z'\ncommand = 'zed {path}'\n", 1)
		if remove {
			d.press("d")
			want = "# my targets\n# a tab\n[[targets]]\nname = \"Tab\"\nkey = \"t\"\ncommand = \"tab {path}\"\n# below\n"
		} else {
			d.press("e")
			d.retype("Editor")
			d.toLast()
			d.press("enter")
		}
		d.pass()
		d.press("enter")

		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("remove %v: config is %v (%v), want it still a symlink", remove, info.Mode(), err)
		}
		if got, _ := fileText(t, real); got != want {
			t.Errorf("remove %v: linked file is %q, want %q", remove, got, want)
		}
		if info, err := os.Stat(real); err != nil || info.Mode().Perm() != 0o640 {
			t.Errorf("remove %v: linked file has mode %v (%v), want 0640 as before", remove, info.Mode().Perm(), err)
		}
		if _, err := os.Stat(filepath.Dir(configPath())); !os.IsNotExist(err) {
			t.Errorf("remove %v: the standard config folder exists (%v), want it untouched", remove, err)
		}
	}
}

func TestEAndDDoNothingOnTheListInASearchTheErrorDialogOrTheHelp(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	const body = "\n[[targets]]\nname = \"Broken\"\nkey = \"b\"\ncommand = \"false {path}\"\n"
	e.config(body)
	for _, c := range []struct {
		name  string
		keys  []string
		title string // of what stays on screen
	}{
		{"list", nil, " loper · 1 repo"},
		{"error dialog", []string{"b"}, "╭─ Could not open alpha in Broken ─"},
		{"help", []string{"?"}, "╭─ Help ─"},
		{"search", []string{"/"}, " /ed"},
	} {
		d := e.start()
		d.press(c.keys...)
		before := boxRows(d)

		d.press("e", "d")

		if screen := d.screen(); !strings.Contains(screen, c.title) || strings.Contains(screen, "a target ─") || !slices.Equal(boxRows(d), before) {
			t.Errorf("%s: e and d changed the screen:\n%s", c.name, screen)
		}
		if got, _ := fileText(t, configPath()); got != body {
			t.Errorf("%s: config is %q, want it unchanged", c.name, got)
		}
	}
}

func TestTheMenuSaysEEditsAndDRemoves(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.press("enter")

	if got, want := menuBottom(d), "╰─ e edit d remove ─╯"; got != want {
		t.Errorf("the menu's bottom border reads %q, want %q", got, want)
	}
}

func TestAnEditConfirmationFitsTheStandardScreenWithALongConfigPath(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.configFile = filepath.Join(e.root, "a folder with a long name", "loper.toml")
	e.writeFile(e.configFile, zedTarget)
	d := e.start()

	d.press("enter", "e")
	d.retype("Zed two")
	d.toLast()
	d.press("enter")

	if got := dialogText(d); !strings.Contains(got, "with these") || !strings.Contains(got, "enter write") {
		t.Errorf("at 80x24 the confirmation reads %q, want it whole:\n%s", got, d.screen())
	}
}

func TestALongBlockIsShortenedInTheConfirmationSoTheEditAndRemoveStillFit(t *testing.T) {
	block := "[[targets]]\n" + strings.Repeat("# a note\n", 10) + "name = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n"
	body := "# mine\n" + block
	for _, remove := range []bool{false, true} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(body)
		d := e.start()

		d.press("enter")
		want := "# mine\n[[targets]]\nname = 'Editor'\nkey = 'z'\ncommand = 'zed {path}'\n"
		if remove {
			d.press("d")
			want = "# mine\n"
		} else {
			d.press("e")
			d.retype("Editor")
			d.toLast()
			d.press("enter")
		}
		if rows := boxRows(d); len(rows) < 5 || !slices.Equal(rows[1:5], []string{"[[targets]]", "# a note", "… 11 more lines", `command = "zed {path}"`}) {
			t.Errorf("remove %v: confirmation reads %q, want the block shortened:\n%s", remove, rows, d.screen())
		}
		d.pass()
		d.press("enter")

		if got, _ := fileText(t, configPath()); got != want {
			t.Errorf("remove %v: config is %q, want %q", remove, got, want)
		}
	}
}

func TestAFileCheckForAnEarlierEditIsIgnored(t *testing.T) {
	const same = "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\n"
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(same + "\n" + same + zedTarget)
	d := e.start()

	d.hold = true
	d.press("enter", "e", "esc", "j", "j", "e") // the refusal for Same arrives at the dialog for Zed
	d.release()

	if got := boxRows(d); len(got) == 0 || got[0] != "> Name Zed" {
		t.Errorf("dialog reads %q, want the fields of Zed:\n%s", got, d.screen())
	}
}

func TestAReadOnlyConfigIsRefusedForAnAddEditOrRemove(t *testing.T) {
	for _, keys := range [][]string{{"a"}, {"enter", "e"}, {"enter", "d"}} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(zedConfig)
		if err := os.Chmod(configPath(), 0o444); err != nil {
			t.Fatal(err)
		}
		d := e.start()

		d.press(keys...)

		if want := "is read-only"; !strings.Contains(dialogText(d), want) || strings.Contains(dialogText(d), "Name") {
			t.Errorf("%q: dialog reads %q, want %q and no fields", keys, dialogText(d), want)
		}
		d.press("enter")
		if got, _ := fileText(t, configPath()); got != zedConfig {
			t.Errorf("%q: config is %q, want it unchanged", keys, got)
		}
		if info, err := os.Stat(configPath()); err != nil || info.Mode().Perm() != 0o444 {
			t.Errorf("%q: config mode %v (%v), want 0444 still", keys, info.Mode().Perm(), err)
		}
	}
}

func TestAnEditThatChangesNothingClosesWithoutWriting(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, editConfig)

	d.press("enter", "e")
	d.toLast()
	d.press("enter")

	if got, want := boxRows(d), []string{"> z Zed", "t Tab"}; !slices.Equal(got, want) {
		t.Errorf("box reads %q, want the menu back, %q", got, want)
	}
	if got := footer(d); strings.Contains(got, "Changed") {
		t.Errorf("footer reads %q, want no notice", got)
	}
	if got, _ := fileText(t, configPath()); got != editConfig {
		t.Errorf("config is %q, want it byte for byte as it was", got)
	}
}

func TestAnEditKeepsTheCloseLineAndShowsItInTheOldAndTheNewBlock(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, "[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\nclose = true\n")

	d.press("enter", "e")
	if got, want := boxRows(d), "After open ‹ close loper ›"; !slices.Contains(got, want) {
		t.Fatalf("dialog reads %q, want the row %q", got, want)
	}
	d.retype("Zed two")
	d.toLast()
	d.press("enter")

	want := []string{
		"Replace these lines in ~/.config/loper/config.toml:",
		"[[targets]]", `name = "Zed"`, `key = "z"`, `command = "zed {path}"`, "close = true",
		"",
		"with these (comments inside the block are not kept):",
		"[[targets]]", "name = 'Zed two'", "key = 'z'", "command = 'zed {path}'", "close = true",
		"",
		"enter write esc back",
	}
	if got := boxRows(d); !slices.Equal(got, want) {
		t.Errorf("confirmation reads %q, want %q", got, want)
	}
	d.pass()
	d.press("enter")
	if got, _ := fileText(t, configPath()); got != "[[targets]]\nname = 'Zed two'\nkey = 'z'\ncommand = 'zed {path}'\nclose = true\n" {
		t.Errorf("config is %q, want the close line kept", got)
	}
}

func TestAnEditOfOnlyTheChoiceIsWritten(t *testing.T) {
	e := newEnv(t)
	d, _ := editStart(e, "[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\nclose = true\n")

	d.press("enter", "e")
	d.toLast()
	d.press("left")
	d.confirmChoice()

	if got, _ := fileText(t, configPath()); got != "[[targets]]\nname = 'Zed'\nkey = 'z'\ncommand = 'zed {path}'\nclose = false\n" {
		t.Errorf("config is %q, want close = false", got)
	}
	if got, want := footer(d), "Changed Zed"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestAnEditThatChangesNothingOnATargetWithCloseWritesNothing(t *testing.T) {
	const body = "[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\nclose = false\n"
	e := newEnv(t)
	d, _ := editStart(e, body)

	d.press("enter", "e")
	d.toLast()
	d.press("right", "right", "right", "enter") // round to where it was

	if got, want := boxRows(d), []string{"> z Zed"}; !slices.Equal(got, want) {
		t.Errorf("box reads %q, want the menu back, %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != body {
		t.Errorf("config is %q, want it byte for byte as it was", got)
	}
}

func TestTwoBlocksThatDifferOnlyInCloseAreEachEditedAndRemoved(t *testing.T) {
	const first = "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\n"
	for _, value := range []string{"true", "false"} { // false is not the same as no close line
		second := "[[targets]]\nname = \"Same\"\ncommand = \"same {path}\"\nclose = " + value + "\n"
		body := "# one\n" + first + "\n# two\n" + second + "# end\n"
		for _, c := range []struct {
			at     int
			remove bool
			want   string
		}{
			{0, false, "# one\n[[targets]]\nname = 'X'\ncommand = 'same {path}'\n\n# two\n" + second + "# end\n"},
			{1, false, "# one\n" + first + "\n# two\n[[targets]]\nname = 'X'\ncommand = 'same {path}'\nclose = " + value + "\n# end\n"},
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
				t.Errorf("close = %s, target %d, remove %v: no confirmation:\n%s", value, c.at, c.remove, d.screen())
				continue
			}
			d.pass()
			d.press("enter")

			if got, _ := fileText(t, configPath()); got != c.want {
				t.Errorf("close = %s, target %d, remove %v: config is\n%q\nwant\n%q\n%s", value, c.at, c.remove, got, c.want, d.screen())
			}
		}
	}
}
