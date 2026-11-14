package ui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

// confirm presses enter on the command and on After open, lets the moment
// in which the confirmation ignores enter pass, and presses enter to write.
func (d *driver) confirm() {
	d.t.Helper()
	d.press("enter", "enter")
	d.pass()
	d.press("enter")
}

// confirmChoice is confirm from After open: it presses enter there, lets the
// moment pass, and presses enter to write.
func (d *driver) confirmChoice() {
	d.t.Helper()
	d.press("enter")
	d.pass()
	d.press("enter")
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

	d.press("shift+tab", "backspace", "up", "backspace", "backspace", "down", "y", "down", "backspace")
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

	if got, want := boxRows(d), "> Command …le x {path}"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q", got, want)
	}
	if got, want := boxRows(d), "After open …tay open) ›"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q, the choice cut from its start", got, want)
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
		d.press("enter", "enter")

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

	d.press("enter", "enter")

	rows := boxRows(d)
	for _, want := range []string{"Append to ~/.config/loper/config.toml", "[[targets]]", "name = 'Tab'", "key = 't'", "command = 'tab {path}'"} {
		if !slices.Contains(rows, want) {
			t.Errorf("confirmation has no row %q:\n%s", want, d.screen())
		}
	}

	d.press("esc")

	if got, want := boxRows(d), []string{"Name Tab", "Key t", "Command tab {path}", "> After open ‹ default (stay open) ›"}; len(got) < 4 || !slices.Equal(got[:4], want) {
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
		d.confirm()

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
	d.confirm()

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
	d.press("enter", "enter")
	d.pass()
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
	d.confirm()

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

func TestAConfigWithAnInlineTargetsListIsRefusedWhenAIsPressedAndLeftAlone(t *testing.T) {
	for _, inline := range []string{
		"targets = []\n",
		"targets = [{ name = \"Zed\", key = \"z\", command = \"zed {path}\" }]\n",
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config(inline)
		d := e.start()

		d.press("a")

		const want = "the file lists its targets inline, as targets = [...]; edit them there by hand"
		if got := dialogText(d); !strings.Contains(got, want) || strings.Contains(got, "Name") {
			t.Errorf("%q: a shows %q, want the reason %q and no fields", inline, got, want)
		}
		d.press("j", "tab", "enter") // do nothing in the message
		if got := dialogText(d); !strings.Contains(got, want) {
			t.Errorf("%q: after other keys the message reads %q, want it still showing", inline, got)
		}
		d.press("esc")
		if got := boxRows(d); got != nil {
			t.Errorf("%q: after esc the screen still has a box: %q", inline, got)
		}
		if got, _ := fileText(t, configPath()); got != inline {
			t.Errorf("%q: config is %q, want it unchanged", inline, got)
		}
	}
}

func TestAConfigThatNoLongerLoadsIsRefusedWhenAIsPressed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	const edited = "paths = [\"~/code\"\n" // edited into a broken state after the launch
	e.config(edited)

	d.press("a")

	if got := dialogText(d); !strings.Contains(got, "~/.config/loper/config.toml:") || strings.Contains(got, "Name") {
		t.Errorf("a shows %q, want the loader's message and no fields", got)
	}
	if got, _ := fileText(t, configPath()); got != edited {
		t.Errorf("config is %q, want it as edited", got)
	}
}

func TestQQuitsFromTheMessageThatTheConfigCannotTakeATarget(t *testing.T) {
	for _, keys := range [][]string{{"q"}, {"ctrl+c"}} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config("targets = []\n")
		d := e.start()
		d.press("a")

		d.press(keys...)

		if !d.quit {
			t.Errorf("%q in the message did not quit", keys)
		}
	}
}

func TestAInTheNoTargetsMessageIsRefusedTooWhenTheConfigCannotTakeATarget(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config("targets = []\n")
	d := e.start()

	d.press("enter", "a")

	if got := dialogText(d); !strings.Contains(got, "lists its targets inline") || strings.Contains(got, "No targets yet") {
		t.Errorf("a in the no-targets message shows %q, want the reason in its place", got)
	}
}

func TestTextTypedBeforeTheCheckOfTheConfigArrivesLandsInTheNameField(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()

	d.hold = true
	d.press("a")
	d.typeText("Zed")
	d.release()

	if got, want := boxRows(d), "> Name Zed"; !slices.Contains(got, want) {
		t.Errorf("dialog reads %q, want the row %q", got, want)
	}
}

func TestTextTypedBeforeARefusalArrivesNeitherOpensATargetNorQuits(t *testing.T) {
	const inline = "targets = [{ name = \"Zed\", key = \"z\", command = \"zed {path}\" }]\n"
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(inline)
	d := e.start()

	d.hold = true
	d.press("a")
	d.typeText("zq")
	d.release()

	if got := dialogText(d); !strings.Contains(got, "lists its targets inline") {
		t.Errorf("dialog reads %q, want the refusal", got)
	}
	if got := e.targetArgs("zed"); got != nil {
		t.Errorf("z typed before the refusal ran zed with %q", got)
	}
	if d.quit {
		t.Errorf("q typed before the refusal quit the tool")
	}
}

func TestTheFileIsReadAgainAtConfirmationAndRefusedWhenItNoLongerLoads(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	const edited = "paths = [\"~/code\"\n" // edited into a broken state while the fields were open
	e.config(edited)

	d.press("enter", "enter")

	if got := dialogText(d); !strings.Contains(got, "Command tab {path}") || !strings.Contains(got, "config.toml:") {
		t.Errorf("dialog reads %q, want the fields and the loader's message", got)
	}
	if got, _ := fileText(t, configPath()); got != edited {
		t.Errorf("config is %q, want it as edited, %q", got, edited)
	}
}

func TestAnEnterRightAfterTheConfirmationAppearsIsIgnored(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.press("enter", "enter", "enter")

	if got := dialogText(d); !strings.Contains(got, "[[targets]]") {
		t.Errorf("after a quick enter, enter: dialog reads %q, want the confirmation", got)
	}
	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("after a quick enter, enter: config is %q, want it unchanged", got)
	}

	d.pass()
	d.press("enter")

	if got, _ := fileText(t, configPath()); got != zedConfig+"\n"+tabBlock {
		t.Errorf("after the moment: config is %q, want the block appended", got)
	}
}

func TestTheMomentOfAnEarlierConfirmationDoesNotCountForALaterOne(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.press("enter", "enter", "esc", "enter")
	d.pass() // the first confirmation's moment ends
	d.press("enter")

	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("config is %q, want it unchanged", got)
	}
	d.pass()
	d.press("esc", "enter", "enter") // a confirmation after one whose moment passed
	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("after a quick enter on a new confirmation: config is %q, want it unchanged", got)
	}
	d.pass()
	d.press("enter")
	if got, _ := fileText(t, configPath()); got != zedConfig+"\n"+tabBlock {
		t.Errorf("after the second moment: config is %q, want the block appended", got)
	}
}

func TestTheFieldsNeedRoomForTheLongestLabelAndACellOfValue(t *testing.T) {
	const notice = "Window too small for the dialog"
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.send(tea.WindowSizeMsg{Width: 18, Height: 24}) // 16 cells inside the border: no cell for a value after After open
	if screen := d.screen(); !strings.Contains(screen, ansi.Truncate(notice, 17, "…")) || strings.Contains(screen, "After") {
		t.Errorf("at 18 cells: want the notice and no dialog:\n%s", screen)
	}
	d.send(tea.WindowSizeMsg{Width: 19, Height: 24})
	if screen := d.screen(); strings.Contains(screen, "Window too") || !strings.Contains(screen, "After open") {
		t.Errorf("at 19 cells: want the dialog:\n%s", screen)
	}
}

func TestTheMomentStartsAgainWhenAHiddenConfirmationShows(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.send(tea.WindowSizeMsg{Width: 80, Height: 8})
	d.press("enter", "enter") // the confirmation, hidden by the notice
	d.pass()

	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	d.press("enter")

	if got, _ := fileText(t, configPath()); got != zedConfig {
		t.Errorf("an enter as the confirmation showed wrote the config: %q", got)
	}
	d.pass()
	d.press("enter")
	if got, _ := fileText(t, configPath()); got != zedConfig+"\n"+tabBlock {
		t.Errorf("after the moment: config is %q, want the block appended", got)
	}
}

func TestOnAScreenTooSmallForTheDialogANoticeShowsAndTheTextIsKept(t *testing.T) {
	const notice = "Window too small for the dialog"
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("enter")

	for _, step := range []struct {
		name string
		want string // a row of the dialog at 80x24
	}{
		{"fields", "> After open ‹ default (stay open) ›"},
		{"confirmation", "command = 'tab {path}'"},
	} {
		for _, size := range [][2]int{{80, 1}, {80, 4}, {80, 5}, {80, 10}, {13, 24}} {
			d.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			screen := d.screen()
			if want := ansi.Truncate(notice, size[0]-1, "…"); !strings.Contains(screen, want) || strings.Contains(screen, "Add a target") {
				t.Errorf("%s at %dx%d: want the notice %q and no dialog:\n%s", step.name, size[0], size[1], want, screen)
			}
		}
		if step.name == "confirmation" {
			d.pass()
			d.press("enter") // never writes what cannot be seen
			if got, _ := fileText(t, configPath()); got != zedConfig {
				t.Errorf("an enter with the confirmation hidden wrote the config: %q", got)
			}
		}

		d.send(tea.WindowSizeMsg{Width: 80, Height: 24})

		if got := boxRows(d); !slices.Contains(got, step.want) || strings.Contains(d.screen(), notice) {
			t.Errorf("%s after growing: dialog reads %q, want the row %q", step.name, got, step.want)
		}
		if step.name == "confirmation" {
			d.pass() // the moment starts again once the confirmation shows
		}
		d.press("enter")
	}
	if got, _ := fileText(t, configPath()); got != zedConfig+"\n"+tabBlock {
		t.Errorf("config is %q, want the block appended", got)
	}
}

func TestTheFileIsReadAgainAtWriteTimeAndNotWrittenWhenItNoLongerLoads(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("enter", "enter")
	d.pass()
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

func TestAWriteRefusedGoesBackToTheFieldsWithTheChoiceKept(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("tab", "right", "right", "enter")
	d.pass()
	e.config("paths = [\"~/code\"\n") // edited into a broken state meanwhile

	d.press("enter")

	if got := dialogText(d); !strings.Contains(got, "> After open ‹ close loper ›") || !strings.Contains(got, "config.toml:") {
		t.Errorf("dialog reads %q, want the fields with the choice and the loader's message", got)
	}
}

// immutable is UF_IMMUTABLE from macOS's sys/stat.h, which the syscall
// package does not name: a file with this flag cannot be changed, removed or
// renamed over, even by its owner.
const immutable = 0x2

// writeFailure is a way to make the write of the config file fail.
type writeFailure struct {
	name string
	why  string // the error the dialog shows
	// set makes the write fail in the config folder dir and returns what
	// undoes that, which it also leaves to t's cleanup.
	set func(t *testing.T, dir string) (undo func())
}

// undoing registers undo with t's cleanup and returns it, so it runs once,
// early or at the end, whatever happens in between.
func undoing(t *testing.T, undo func()) func() {
	undo = sync.OnceFunc(undo)
	t.Cleanup(undo)
	return undo
}

var (
	tempFileNotCreated = writeFailure{"the temporary file cannot be created", "permission denied", func(t *testing.T, dir string) func() {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		return undoing(t, func() { os.Chmod(dir, 0o755) })
	}}
	// A process may write no more than its file size limit into a file, so
	// the write stops partway. The limit holds for the whole process, so it
	// is set only in a child, see the test below, and lifted right after.
	writeCutShort = writeFailure{"the write is cut short", "file too large", func(t *testing.T, _ string) func() {
		var was syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &was); err != nil {
			t.Fatal(err)
		}
		limit := was
		limit.Cur = 16
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		return undoing(t, func() { syscall.Setrlimit(syscall.RLIMIT_FSIZE, &was) })
	}}
	// The file can still be read and its folder written, so the temporary
	// file is made and written, but it cannot be renamed over an immutable
	// file.
	renameRefused = writeFailure{"the rename is refused", "operation not permitted", func(t *testing.T, dir string) func() {
		file := filepath.Join(dir, "config.toml")
		if err := syscall.Chflags(file, immutable); err != nil {
			t.Fatal(err)
		}
		return undoing(t, func() { syscall.Chflags(file, 0) })
	}}
)

// failedWrites adds, edits and removes a target with the write failing as
// failure makes it, and checks that the dialog shows the error and the file
// is as it was, with its mode, and nothing is left beside it.
func failedWrites(t *testing.T, failure writeFailure) {
	t.Helper()
	for _, op := range []struct {
		name string
		keys func(d *driver) // up to the confirmation
	}{
		{"add", func(d *driver) { d.fill("Vim", "v", "vim {path}"); d.press("enter", "enter") }},
		{"edit", func(d *driver) { d.press("enter", "e"); d.retype("Zedd"); d.toLast(); d.press("enter") }},
		{"remove", func(d *driver) { d.press("enter", "d") }},
	} {
		e := newEnv(t)
		d, _ := editStart(e, editConfig)
		if err := os.Chmod(configPath(), 0o640); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(configPath())
		op.keys(d)
		d.pass()
		undo := failure.set(t, dir)

		d.press("enter")
		undo()

		if got := dialogText(d); !strings.Contains(got, failure.why) {
			t.Errorf("%s, %s: dialog reads %q, want the error %q", failure.name, op.name, got, failure.why)
		}
		if got, _ := fileText(t, configPath()); got != editConfig {
			t.Errorf("%s, %s: config is %q, want it byte for byte as it was", failure.name, op.name, got)
		}
		if info, err := os.Stat(configPath()); err != nil || info.Mode().Perm() != 0o640 {
			t.Errorf("%s, %s: config has mode %v (%v), want 0640 as before", failure.name, op.name, info.Mode().Perm(), err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Errorf("%s, %s: config folder holds %v, want only the config file", failure.name, op.name, entries)
		}
	}
}

func TestAnAddEditOrRemoveWhoseWriteFailsShowsTheErrorAndLeavesTheFileAndItsMode(t *testing.T) {
	failedWrites(t, tempFileNotCreated)
	failedWrites(t, renameRefused)

	// The file size limit would also stop go test's log of what the test
	// process opens, so the cut-short write runs in a child, which keeps no
	// such log, as it is started without -test.testlogfile.
	cmd := exec.Command(os.Args[0], "-test.v", "-test.run=^TestAWriteCutShortInAChild$")
	cmd.Env = append(os.Environ(), "LOPER_TEST_CUT_SHORT=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestAWriteCutShortInAChild") {
		t.Errorf("the cut-short write in a child: %v\n%s", err, out)
	}
}

// TestAWriteCutShortInAChild is the child process of the test above.
func TestAWriteCutShortInAChild(t *testing.T) {
	if os.Getenv("LOPER_TEST_CUT_SHORT") == "" {
		t.Skip("run by the test of failed writes, in a child process")
	}
	failedWrites(t, writeCutShort)
}

func TestEnterAgainWhileTheWriteRunsAppendsTheBlockOnce(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(zedConfig)
	d := e.start()
	d.fill("Tab", "", "tab {path}") // no key, so a second block would load
	d.press("enter", "enter")
	d.pass()

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
	d.press("enter", "enter", "up", "x")
	d.release()

	if got := dialogText(d); strings.Contains(got, "[[targets]]") || !strings.Contains(got, "Command tab {path}x") {
		t.Errorf("dialog reads %q, want the fields as typed since, not the confirmation of before", got)
	}
}

func TestACheckThatArrivesAfterTheChoiceChangedIsIgnored(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("tab")

	d.hold = true
	d.press("enter", "right")
	d.release()

	if got := dialogText(d); strings.Contains(got, "[[targets]]") || !strings.Contains(got, "> After open ‹ stay open ›") {
		t.Errorf("dialog reads %q, want the fields with the choice made since, not the confirmation of before", got)
	}
}

func TestCtrlCQuitsFromTheDialogAndItsConfirmation(t *testing.T) {
	for _, keys := range [][]string{{}, {"enter", "enter"}} {
		e := newEnv(t)
		e.repo("code/alpha")
		d := e.start()
		d.fill("Tab", "t", "tab {path}")
		d.press(keys...)
		if confirming := strings.Contains(dialogText(d), "Append to"); confirming != (len(keys) > 0) {
			t.Fatalf("after %q: confirmation showing %v, want %v:\n%s", keys, confirming, len(keys) > 0, d.screen())
		}

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
	d.confirm()

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
	d.confirm()

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
	d.confirm()

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
	d.press("enter", "enter")
	d.paste("x") // the confirmation takes no text
	d.press("esc")
	if got, want := boxRows(d), "Command tab{path}"; !slices.Contains(got, want) {
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
	d.confirm()

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
	d.confirm()
	if info, err = os.Stat(configPath()); err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("new config has mode %v, want 0600", got)
	}
}

func TestAConfigThatLinksToAFileThatDoesNotExistIsRefusedWhenAIsPressedAndLeftAlone(t *testing.T) {
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

	d.press("a")

	if want := "is a link to " + gone + ", which does not exist"; !strings.Contains(strings.ReplaceAll(dialogText(d), " ", ""), strings.ReplaceAll(want, " ", "")) || strings.Contains(dialogText(d), "Name") {
		t.Errorf("a shows %q, want %q and no fields", dialogText(d), want)
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
	d.hold = false
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

func TestTheDialogShowsAfterOpenWithWhatTheDefaultMeansNow(t *testing.T) {
	for setting, want := range map[string]string{"false": "After open ‹ default (stay open) ›", "true": "After open ‹ default (close loper) ›"} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.config("close_on_open = " + setting + "\n")
		d := e.start()

		d.press("a")

		if got, want := boxRows(d), []string{"> Name", "Key", "Command", want}; len(got) < 4 || !slices.Equal(got[:4], want) {
			t.Errorf("close_on_open %s: fields read %q, want %q", setting, got, want)
		}
		d.press("up", "up", "up", "up", "down", "down", "down") // to After open
		if got, want := boxRows(d), "> "+want; !slices.Contains(got, want) {
			t.Errorf("close_on_open %s: selected, fields read %q, want the row %q", setting, got, want)
		}
	}
}

func TestEnterOnTheCommandMovesToAfterOpenAndFromThereToTheConfirmation(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab {path}")

	d.press("enter")
	if got, want := boxRows(d), "> After open ‹ default (stay open) ›"; !slices.Contains(got, want) {
		t.Fatalf("after enter on the command: dialog reads %q, want the row %q", got, want)
	}
	d.press("up", "down", "shift+tab", "tab")
	if got, want := boxRows(d), "> After open ‹ default (stay open) ›"; !slices.Contains(got, want) {
		t.Fatalf("after moving away and back: dialog reads %q, want the row %q", got, want)
	}
	d.press("tab", "down") // nowhere further to go
	d.press("enter")
	if got := dialogText(d); !strings.Contains(got, "Append to") {
		t.Errorf("after enter on After open: dialog reads %q, want the confirmation", got)
	}
}

func TestLeftRightAndSpaceChangeTheChoiceAndOtherKeysDoNothingThere(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab")
	d.press("space")
	d.typeText("{path}")
	d.press("tab")

	for _, step := range []struct {
		key, want string
	}{
		{"right", "stay open"},
		{"right", "close loper"},
		{"space", "take over this window"},
		{"right", "default (stay open)"},
		{"left", "take over this window"},
		{"x", "take over this window"},
		{"backspace", "take over this window"},
		{"left", "close loper"},
	} {
		d.press(step.key)
		if got, want := boxRows(d), "> After open ‹ "+step.want+" ›"; !slices.Contains(got, want) {
			t.Fatalf("after %s: dialog reads %q, want the row %q", step.key, got, want)
		}
	}
	if got, want := boxRows(d), []string{"Name Tab", "Key t", "Command tab {path}"}; len(got) < 3 || !slices.Equal(got[:3], want) {
		t.Errorf("fields read %q, want %q, with the space in the command", got, want)
	}
}

func TestAPasteOnAfterOpenDoesNothing(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.fill("Tab", "t", "tab {path}")
	d.press("tab")

	d.paste("close loper")

	if got, want := boxRows(d), []string{"Name Tab", "Key t", "Command tab {path}", "> After open ‹ default (stay open) ›"}; len(got) < 4 || !slices.Equal(got[:4], want) {
		t.Errorf("dialog reads %q, want %q", got, want)
	}
}

func TestAnAddedTargetWritesItsChoiceAndBehavesSoAtOnce(t *testing.T) {
	for _, c := range []struct {
		setting string
		keys    []string // on After open
		block   string   // what is written after tabBlock
		quits   bool
	}{
		{"false", []string{"right", "right"}, "close = true\n", true},
		{"true", []string{"right"}, "close = false\n", false},
		{"true", nil, "", true},
	} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.fakeTarget("tab")
		e.config("close_on_open = " + c.setting + "\n")
		d := e.start()
		d.fill("Tab", "t", "tab {path}")
		d.press("tab")
		d.press(c.keys...)

		d.press("enter")
		d.pass()
		d.press("enter")

		if got, _ := fileText(t, configPath()); got != "close_on_open = "+c.setting+"\n\n"+tabBlock+c.block {
			t.Errorf("%q: config is %q, want the block with %q", c.keys, got, c.block)
		}
		d.press("t")
		if got, want := e.targetArgs("tab"), []string{alpha}; !slices.Equal(got, want) {
			t.Errorf("%q: tab called with %q, want %q", c.keys, got, want)
		}
		if d.quit != c.quits {
			t.Errorf("%q: quit %v after the open, want %v", c.keys, d.quit, c.quits)
		}
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
		if c.reason == "" {
			d.pass()
		}
		d.press("enter")

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
