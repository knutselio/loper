package ui_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// configPath is the config file at the standard location.
func configPath() string {
	return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "loper", "config.toml")
}

// fileText returns the content of the file at path, or "" and false when
// there is none.
func fileText(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// fill opens the add dialog and types name, key and command into its
// fields, moving between them with tab.
func (d *driver) fill(name, key, command string) {
	d.t.Helper()
	d.press("a")
	d.typeText(name)
	d.press("tab")
	d.typeText(key)
	d.press("tab")
	d.typeText(command)
}

// dialogText is the text inside the box on screen, its rows joined by
// spaces, so a reason wrapped over several rows reads as one.
func dialogText(d *driver) string {
	return strings.Join(boxRows(d), " ")
}

// tabBlock is the block the dialog appends for a target Tab, key t, that
// runs the fake target tab.
const tabBlock = "[[targets]]\nname = 'Tab'\nkey = 't'\ncommand = 'tab {path}'\n"

// zedConfig is a config with a comment and one target, the fake zed.
const zedConfig = "# my targets\n[[targets]]\nname = \"Zed\" # the editor\nkey = \"z\"\ncommand = \"zed {path}\"\n"

func TestAOpensTheAddDialogAndEveryPrintableKeyLandsInItsFields(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.repo("code/beta")
	e.config(zedTarget)
	d := e.start()

	d.press("a")
	if !strings.Contains(d.screen(), "╭─ Add a target ─") {
		t.Fatalf("a shows no dialog:\n%s", d.screen())
	}
	d.typeText("qsja?/rz g")
	d.press("tab")
	d.typeText("x")
	d.press("tab")
	d.typeText("tab {path}")

	if got, want := boxRows(d), []string{"Name qsja?/rz g", "Key x", "> Command tab {path}"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("fields read %q, want %q:\n%s", got, want, d.screen())
	}
	if d.quit {
		t.Errorf("q typed in the dialog quit the tool")
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("z typed in the dialog ran zed with %q", got)
	}
	if strings.Contains(d.screen(), "sort: name") || strings.Contains(d.screen(), "╭─ Help ─") {
		t.Errorf("s or ? typed in the dialog acted:\n%s", d.screen())
	}

	d.press("shift+tab", "backspace", "up", "backspace", "backspace", "down", "y", "down", "down", "backspace")
	if got, want := boxRows(d), []string{"Name qsja?/rz", "Key y", "> Command tab {path"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("after moving and backspace: fields read %q, want %q", got, want)
	}

	d.press("esc", "z")
	if got, want := e.targetArgs("zed"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("after esc, zed called with %q, want the first repo, still selected, %q", got, want)
	}
}

func TestAValueTooLongForItsFieldShowsItsEnd(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 30, Height: 24})

	d.fill("Tab", "t", "tab --title x {path}")

	if got, want := boxRows(d), "> Command …title x {path}"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q", got, want)
	}
}

func TestEscInTheFieldsClosesTheDialogAndWritesNothing(t *testing.T) {
	for _, body := range []string{"", zedConfig} {
		e := newEnv(t)
		e.repo("code/alpha")
		if body != "" {
			e.config(body)
		}
		d := e.start()

		d.fill("Tab", "t", "tab {path}")
		d.press("esc")

		if got := boxRows(d); got != nil {
			t.Errorf("dialog still shown after esc: %q", got)
		}
		if got, ok := fileText(t, configPath()); got != body || ok != (body != "") {
			t.Errorf("config is %q (exists %v), want it as it was, %q", got, ok, body)
		}
	}
}

func TestAnInvalidTargetIsRefusedWithTheReasonAndNothingIsWritten(t *testing.T) {
	for _, c := range []struct {
		name, key, command string
		reason             string
	}{
		{"", "t", "tab {path}", "a target needs a name"},
		{"   ", "t", "tab {path}", "a target needs a name"},
		{"Tab", "t", "tab", `target command "tab" has no {path}`},
		{"Tab", "t", "tab '{path}", `target command "tab '{path}" has an unbalanced quote`},
		{"Tab", "tt", "tab {path}", `target key "tt" is not a single character`},
		{"Tab", "q", "tab {path}", `target key "q" is one of loper's own keys`},
		{"Tab", "a", "tab {path}", `target key "a" is one of loper's own keys`},
		{"Tab", "z", "tab {path}", `target key "z" is already used by the target on line 2`},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(zedConfig)
		d := e.start()

		d.fill(c.name, c.key, c.command)
		d.press("enter")

		if got := dialogText(d); !strings.Contains(got, c.reason) || strings.Contains(got, "config.toml:") || !strings.Contains(got, "Command "+c.command) {
			t.Errorf("%q: dialog reads %q, want the fields and the reason %q", c, got, c.reason)
		}
		if got, _ := fileText(t, configPath()); got != zedConfig {
			t.Errorf("%q: config is %q, want it unchanged", c, got)
		}
	}
}

func TestTheConfirmationShowsTheBlockAndTheFileAndEscGoesBackToTheFields(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.press("enter")

	rows := boxRows(d)
	for _, want := range []string{"Append to ~/.config/loper/config.toml", "[[targets]]", "name = 'Tab'", "key = 't'", "command = 'tab {path}'"} {
		if !slices.Contains(rows, want) {
			t.Errorf("confirmation has no row %q:\n%s", want, d.screen())
		}
	}

	d.press("esc")

	if got, want := boxRows(d), []string{"Name Tab", "Key t", "> Command tab {path}"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("after esc: dialog reads %q, want the fields with their contents %q", got, want)
	}
	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}
}

func TestConfirmingAppendsOneBlockAndKeepsEveryByteBeforeIt(t *testing.T) {
	for _, c := range []struct{ before, after string }{
		{zedConfig, zedConfig + "\n" + tabBlock},
		{strings.TrimSuffix(zedConfig, "\n"), zedConfig + "\n" + tabBlock}, // no line break at the end
		{"paths = [\"~/code\"]\nsort = \"name\"\n", "paths = [\"~/code\"]\nsort = \"name\"\n\n" + tabBlock},
		{"", tabBlock},
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(c.before)
		d := e.start()

		d.fill("Tab", "t", "tab {path}")
		d.press("enter", "enter")

		if got, _ := fileText(t, configPath()); got != c.after {
			t.Errorf("config %q became %q, want %q", c.before, got, c.after)
		}
		if got := boxRows(d); got != nil {
			t.Errorf("dialog still shown after writing: %q", got)
		}
		if got, want := footer(d), "Added Tab"; got != want {
			t.Errorf("footer reads %q, want %q", got, want)
		}
	}
}

func TestConfirmingWithNoConfigFileCreatesItsFolderAndTheFile(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")

	if got, _ := fileText(t, configPath()); got != tabBlock {
		t.Errorf("config is %q, want just the block %q", got, tabBlock)
	}
	info, err := os.Stat(filepath.Dir(configPath()))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("config folder has mode %v, want it private to the user", got)
	}
}

func TestWithConfigTheDialogWritesThatFileAndNotTheStandardOne(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.configFile = filepath.Join(e.root, "other.toml")
	e.writeFile(e.configFile, zedConfig)
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter")
	// The path is long enough to wrap, so spaces are left out of the comparison.
	if want := "Append to " + e.configFile; !strings.Contains(strings.ReplaceAll(dialogText(d), " ", ""), strings.ReplaceAll(want, " ", "")) {
		t.Errorf("confirmation does not name the file: want %q in %q", want, dialogText(d))
	}
	d.press("enter")

	if got, _ := fileText(t, e.configFile); got != zedConfig+"\n"+tabBlock {
		t.Errorf("--config file is %q, want the block appended", got)
	}
	if _, err := os.Stat(filepath.Dir(configPath())); !os.IsNotExist(err) {
		t.Errorf("the standard config folder exists (%v), want it untouched", err)
	}
}

func TestASymlinkedConfigStaysALinkAndTheFileItPointsAtGetsTheBlock(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	real := filepath.Join(e.root, "dotfiles", "loper.toml")
	e.writeFile(real, zedConfig)
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, configPath()); err != nil {
		t.Fatal(err)
	}
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")

	info, err := os.Lstat(configPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("config is a %v, want it still a symlink", info.Mode())
	}
	if got, _ := fileText(t, real); got != zedConfig+"\n"+tabBlock {
		t.Errorf("linked file is %q, want the block appended", got)
	}
}

func TestAConfigWithAnInlineTargetsArrayIsNotModifiedAndTheDialogSaysWhy(t *testing.T) {
	const inline = "targets = [{ name = \"Zed\", key = \"z\", command = \"zed {path}\" }]\n"
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(inline)
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter")

	if want := "the file lists its targets inline, as targets = [...]; add this one there by hand"; !strings.Contains(dialogText(d), want) {
		t.Errorf("dialog reads %q, want the loader's reason %q", dialogText(d), want)
	}
	if got, _ := fileText(t, configPath()); got != inline {
		t.Errorf("config is %q, want it unchanged", got)
	}
}

func TestTheFileIsReadAgainAtWriteTimeAndNotWrittenWhenItNoLongerLoads(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("enter")
	const edited = "paths = [\"~/code\"\n" // edited into a broken state meanwhile
	e.config(edited)

	d.press("enter")

	if got, want := dialogText(d), "Command tab {path}"; !strings.Contains(got, want) || !strings.Contains(got, "config.toml:") {
		t.Errorf("dialog reads %q, want the fields and the loader's message", got)
	}
	if got, _ := fileText(t, configPath()); got != edited {
		t.Errorf("config is %q, want it as edited, %q", got, edited)
	}
}

func TestAFailedWriteShowsTheErrorAndLeavesTheFileWhole(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	dir := filepath.Dir(configPath())
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("enter")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	d.press("enter")

	if got := dialogText(d); !strings.Contains(got, "permission denied") {
		t.Errorf("dialog reads %q, want the write error", got)
	}
	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("config folder holds %d entries, want only the config file", len(entries))
	}
}

func TestEnterAgainWhileTheWriteRunsAppendsTheBlockOnce(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "", "tab {path}") // no key, so a second block would load
	d.press("enter")

	d.hold = true
	d.press("enter", "enter")
	d.release()

	if got, _ := fileText(t, configPath()); strings.Count(got, "[[targets]]") != 2 {
		t.Errorf("config is %q, want one block appended", got)
	}
}

func TestACheckThatArrivesAfterTheFieldsChangedIsIgnored(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.hold = true
	d.press("enter", "x")
	d.release()

	if got := dialogText(d); strings.Contains(got, "[[targets]]") || !strings.Contains(got, "Command tab {path}x") {
		t.Errorf("dialog reads %q, want the fields as typed since, not the confirmation of before", got)
	}
}

func TestCtrlCQuitsFromTheDialogAndItsConfirmation(t *testing.T) {
	for _, keys := range [][]string{{}, {"enter"}} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()
		d.fill("Tab", "t", "tab {path}")
		d.press(keys...)

		d.press("ctrl+c")

		if !d.quit {
			t.Errorf("after %q: ctrl-c did not quit", keys)
		}
	}
}

func TestQuotesBackslashesAndNonASCIIRoundTripThroughTheWrittenFile(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.fakeTarget("tab")
	name := `Tom's "tab" \ café`
	d := e.start()

	d.fill(name, "é", `tab {path} "say \"hi\"" 'back\slash' café`)
	d.press("enter", "enter")

	d = e.start() // a relaunch with the written file
	d.press("enter")
	if got, want := boxRows(d), []string{"> é " + name}; !slices.Equal(got, want) {
		t.Errorf("menu after a relaunch reads %q, want %q", got, want)
	}
	d.press("é")
	if got, want := e.targetArgs("tab"), []string{alpha, `say "hi"`, `back\slash`, "café"}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}
}

func TestAnAddedTargetWorksAtOnceAndAfterARelaunch(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(zedConfig)
	e.fakeTarget("tab")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 100, Height: 60}) // room for the whole footer and help

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")

	d.press("enter")
	if got, want := boxRows(d), []string{"> z Zed", "t Tab"}; !slices.Equal(got, want) {
		t.Errorf("menu reads %q, want %q", got, want)
	}
	d.press("esc", "?")
	if rows := boxRows(d); !slices.Contains(rows, "t Tab") {
		t.Errorf("help lists no t Tab:\n%s", d.screen())
	}
	d.press("esc")
	if got := footer(d); !strings.HasSuffix(got, "| z Zed | t Tab | a add") {
		t.Errorf("footer reads %q, want it to end with the new target", got)
	}
	d.press("t")
	if got, want := e.targetArgs("tab"), []string{alpha}; !slices.Equal(got, want) {
		t.Errorf("tab called with %q, want %q", got, want)
	}

	d = e.start()
	d.send(tea.WindowSizeMsg{Width: 100, Height: 24})
	if got := footer(d); !strings.HasSuffix(got, "| z Zed | t Tab | a add") {
		t.Errorf("after a relaunch: footer reads %q, want it to end with the new target", got)
	}
}

func TestADoesNothingInTheMenuTheErrorDialogTheHelpOrASearch(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`
[[targets]]
name = "Broken"
key = "b"
command = "false {path}"
`)
	for _, c := range []struct {
		name  string
		keys  []string
		title string // of what stays on screen
	}{
		{"menu", []string{"enter"}, "╭─ Open alpha ─"},
		{"error dialog", []string{"b"}, "╭─ Could not open alpha in Broken ─"},
		{"help", []string{"?"}, "╭─ Help ─"},
		{"search", []string{"/"}, " /a"},
	} {
		d := e.start()
		d.press(c.keys...)

		d.press("a")

		if screen := d.screen(); !strings.Contains(screen, c.title) || strings.Contains(screen, "╭─ Add a target") {
			t.Errorf("%s: a changed the screen:\n%s", c.name, screen)
		}
	}
}

func TestANameIsWrittenWithoutTheSpacesAroundIt(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.fill("  Tab  ", "t", "tab {path}")
	d.press("enter", "enter")

	if got, _ := fileText(t, configPath()); got != tabBlock {
		t.Errorf("config is %q, want %q", got, tabBlock)
	}
	if got, want := footer(d), "Added Tab"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestAPasteGoesToTheFieldWithoutLineBreaksOrControlCharacters(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "")

	d.paste("tab\n{pa\x1b[31mth}")

	if got, want := boxRows(d), "> Command tab{path}"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q", got, want)
	}
	d.press("enter")
	d.paste("x") // the confirmation takes no text
	d.press("esc")
	if got, want := boxRows(d), "> Command tab{path}"; !slices.Contains(got, want) {
		t.Errorf("after a paste in the confirmation: dialog reads %q, want the row %q", got, want)
	}
}

func TestAConfigFileKeepsItsModeAndANewOneIsPrivateToTheUser(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	if err := os.Chmod(configPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")

	info, err := os.Stat(configPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("config has mode %v after the write, want 0644 as before", got)
	}

	e = newEnv(t)
	e.repo("code/alpha")
	d = e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")
	if info, err = os.Stat(configPath()); err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("new config has mode %v, want 0600", got)
	}
}

func TestAConfigThatLinksToAFileThatDoesNotExistIsRefusedAndLeftAlone(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	gone := filepath.Join(e.root, "dotfiles", "loper.toml")
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gone, configPath()); err != nil {
		t.Fatal(err)
	}
	d := e.start()

	d.fill("Tab", "t", "tab {path}")
	d.press("enter")

	if want := "is a link to " + gone + ", which does not exist"; !strings.Contains(strings.ReplaceAll(dialogText(d), " ", ""), strings.ReplaceAll(want, " ", "")) {
		t.Errorf("dialog reads %q, want %q", dialogText(d), want)
	}
	if dest, err := os.Readlink(configPath()); err != nil || dest != gone {
		t.Errorf("config link is %q (%v), want it untouched, pointing at %q", dest, err, gone)
	}
	if _, err := os.Lstat(gone); !os.IsNotExist(err) {
		t.Errorf("the link's destination exists (%v), want nothing written", err)
	}
}

func TestWithCloseOnOpenAnOpenThatSucceedsWhileTheDialogIsOpenDoesNotQuit(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config("close_on_open = true\n" + zedTarget)
	d := e.start()

	d.hold = true
	d.press("z") // the open runs, and its result is held back
	d.press("a")
	d.typeText("Tab")
	d.release()

	if d.quit {
		t.Errorf("the tool quit with the add dialog open")
	}
	if got, want := boxRows(d), "> Name Tab"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q", got, want)
	}
}

// dialogRows is the add dialog's rows as styled shows them, with their
// spacing collapsed.
func dialogRows(d *driver) []string {
	var rows []string
	for _, line := range strings.Split(d.screen(), "\n") {
		if s := styled(line); strings.Contains(s, "│") {
			rows = append(rows, strings.Join(strings.Fields(s), " "))
		}
	}
	return rows
}

func TestTheEmptyFieldsShowTheZedExampleDimmedOnlyWithColour(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/alpha")
	d := e.start()

	d.press("a")
	for _, want := range []string{"{│} [>] Name {Zed} {│}", "{│} Key {z} {│}", "{│} Command {zed {path}} {│}"} {
		if !slices.Contains(dialogRows(d), want) {
			t.Errorf("dialog has no row %q: %q", want, dialogRows(d))
		}
	}
	d.typeText("T")
	if rows := dialogRows(d); !slices.Contains(rows, "{│} [>] Name T {│}") {
		t.Errorf("after typing: dialog reads %q, want the name without its example", rows)
	}

	e.color = false
	d = e.start()
	d.press("a")
	if got, want := boxRows(d), []string{"> Name", "Key", "Command"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("colour off: fields read %q, want them empty, %q", got, want)
	}
}

func TestAFieldLeftEmptyIsNeverTakenAsItsExample(t *testing.T) {
	for _, c := range []struct {
		name, key, command string
		reason             string // why it is refused, or "" when it is written
	}{
		{"Tab", "", "tab {path}", ""},
		{"", "t", "tab {path}", "a target needs a name"},
		{"Tab", "t", "", `target command "" has no {path}`},
	} {
		e := newEnv(t)
		e.color = true
		e.repo("code/alpha")
		d := e.start()

		d.fill(c.name, c.key, c.command)
		d.press("enter", "enter")

		got, _ := fileText(t, configPath())
		if c.reason == "" {
			if want := "[[targets]]\nname = 'Tab'\ncommand = 'tab {path}'\n"; got != want {
				t.Errorf("%q: config is %q, want %q", c, got, want)
			}
			continue
		}
		if got != "" {
			t.Errorf("%q: config is %q, want nothing written", c, got)
		}
		if text := strings.Join(dialogRows(d), " "); !strings.Contains(text, c.reason) {
			t.Errorf("%q: dialog reads %q, want the reason %q", c, text, c.reason)
		}
	}
}
