package ui_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/config"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/repoinfo"
	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
)

// scripts holds every fake target script, written once per test binary and
// named by a hash of its content: macOS takes well over 100 ms to run a newly
// written executable for the first time, so tests share the files and each
// test links them into its own bin folder.
var scripts string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "loper-scripts")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scripts = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// zedTarget is a config's target Zed, key z, which runs the fake zed. It goes
// at the end of a config, after the settings.
const zedTarget = "\n[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n"

// env is a test's fixed environment: a temporary home with its config,
// cache and state folders, and a fake zed first on the PATH, so a target
// that runs zed never runs the real program.
type env struct {
	t    *testing.T
	root string // holds the home folder, the fake targets and their records
	home string
	wait time.Duration // how long a target runs before it counts as opened
	// notice is how long a footer notice shows; the driver never waits for
	// it, but records it with the notice's tick.
	notice time.Duration
	// pause is how long after the add dialog's confirmation appears an enter
	// is ignored; the driver never waits for it either.
	pause time.Duration
	now   time.Time // the time on the program's clock
	// color turns colour on, as an unset NO_COLOR does; tests leave it off
	// unless they are about colour.
	color bool
	// configFile is the config file named with --config, or "" for the
	// standard location.
	configFile string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := filepath.Join(t.TempDir(), "temp folder") // a space, so commands must quote paths
	// The wait is far longer than any fake target that exits takes, so a slow
	// start never turns a failure into a success; tests of targets that keep
	// running shorten it.
	e := &env{t: t, root: root, home: filepath.Join(root, "home"), wait: 10 * time.Second, notice: 3 * time.Second,
		pause: 300 * time.Millisecond, now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(filepath.ListSeparator)+os.Getenv("PATH"))
	t.Setenv("LOPER_TEST_RECORDS", root) // where fake targets write their records
	t.Setenv("HOME", e.home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(e.home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(e.home, ".local", "state"))
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	if err := os.MkdirAll(e.home, 0o755); err != nil {
		t.Fatal(err)
	}
	e.fakeTarget("zed")
	return e
}

// repo creates a real git repo at rel, relative to the home folder.
func (e *env) repo(rel string) string {
	e.t.Helper()
	dir := filepath.Join(e.home, rel)
	out, err := exec.Command("git", "init", "-q", "-b", "main", dir).CombinedOutput()
	if err != nil {
		e.t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
	return dir
}

// git runs git with args in dir.
func (e *env) git(dir string, args ...string) {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// gitOutput runs git with args in dir and returns its trimmed output.
func (e *env) gitOutput(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		e.t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// symlink creates a symlink at rel, relative to the home folder, pointing
// at target.
func (e *env) symlink(target, rel string) {
	e.t.Helper()
	if err := os.Symlink(target, filepath.Join(e.home, rel)); err != nil {
		e.t.Fatal(err)
	}
}

// config writes body as the config file at the standard location.
func (e *env) config(body string) {
	e.t.Helper()
	e.writeFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "loper", "config.toml"), body)
}

// writeFile writes body to path, creating its folders.
func (e *env) writeFile(path, body string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// fakeTarget writes a program called name, on the PATH, that records its
// arguments and its working folder. It returns the program's path quoted for
// a target command.
func (e *env) fakeTarget(name string) string {
	e.t.Helper()
	record := `"$LOPER_TEST_RECORDS"/'` + name + `'`
	return e.script(name, "pwd > "+record+".dir\nprintf '%s\\n' \"$@\" > "+record+".args\n")
}

// script puts a shell script called name, with body as its commands, on the
// PATH, as a link to the shared copy of that script. It returns the script's
// path quoted for a target command.
func (e *env) script(name, body string) string {
	e.t.Helper()
	content := "#!/bin/sh\n" + body
	shared := filepath.Join(scripts, fmt.Sprintf("%x", sha256.Sum256([]byte(content))))
	if _, err := os.Stat(shared); os.IsNotExist(err) {
		// Written under another name and renamed, so a script is never run
		// half written.
		e.writeFile(shared+".tmp", content)
		if err := os.Chmod(shared+".tmp", 0o755); err != nil {
			e.t.Fatal(err)
		}
		if err := os.Rename(shared+".tmp", shared); err != nil {
			e.t.Fatal(err)
		}
	}
	script := filepath.Join(e.root, "bin", name)
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		e.t.Fatal(err)
	}
	os.Remove(script) // a test may replace a script it made earlier
	if err := os.Symlink(shared, script); err != nil {
		e.t.Fatal(err)
	}
	return "'" + script + "'"
}

// targetArgs returns the arguments the fake target name was called with, or
// nil if it never ran.
func (e *env) targetArgs(name string) []string {
	e.t.Helper()
	b := e.record(name + ".args")
	if b == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(b, "\n"), "\n")
}

// targetDir returns the working folder the fake target name ran in, or ""
// if it never ran.
func (e *env) targetDir(name string) string {
	e.t.Helper()
	return strings.TrimSuffix(e.record(name+".dir"), "\n")
}

// record returns what a fake target wrote to the file rel, or "" if it
// never ran.
func (e *env) record(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, rel))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

// driver feeds keys to the real program one at a time and runs any
// resulting background work on the spot.
type driver struct {
	t     *testing.T
	model tea.Model
	quit  bool
	// hold keeps back what background work returns, in the order the work
	// ran, instead of handing it to the program, until next or release.
	hold bool
	held []tea.Msg
	// ticks are the pending ticks of the spinner and of footer notices, which
	// never come by themselves: a test delivers them with tick, so neither
	// needs real time and the driver never loops on the spinner.
	ticks []pendingTick
	now   time.Time
	pause time.Duration // as env's
}

// pendingTick is a tick on its way: after how long it was asked for, and
// what it delivers.
type pendingTick struct {
	after time.Duration
	fn    func(time.Time) tea.Msg
}

// start launches the program on a fixed 80x24 screen at e.now, with the
// settings loaded from the config file as the entry point does, and hands
// it the results of its launch work: the cached list, the history and the
// refresh.
func (e *env) start() *driver {
	e.t.Helper()
	d := e.launch()
	d.release()
	return d
}

// launch is start with the results of the launch work held back, in the
// order the work ran: the cache read, then the history read. next and
// release hand them over.
func (e *env) launch() *driver {
	e.t.Helper()
	settings, err := config.Load(e.configFile, e.home)
	if err != nil {
		e.t.Fatal(err)
	}
	d := &driver{t: e.t, hold: true, now: e.now, pause: e.pause}
	d.model = ui.New(ui.Deps{
		Home:       e.home,
		ConfigFile: settings.File,
		Paths:      settings.Paths,
		Scan:       scanner.Scan,
		Missing:    scanner.Missing,
		Branch:     repoinfo.Branch,
		Exists:     repoinfo.Exists,
		Open:       func(t target.Target, path string) error { return target.Run(t, path, e.wait) },
		Targets:    settings.Targets,
		History:    history.New(e.home),
		Cache:      cache.New(e.home),
		Now:        func() time.Time { return e.now },
		Tick: func(after time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
			return func() tea.Msg {
				d.ticks = append(d.ticks, pendingTick{after, fn})
				return nil
			}
		},
		NoticeFor:   e.notice,
		EnterPause:  e.pause,
		CloseOnOpen: settings.CloseOnOpen,
		SortByName:  settings.SortByName,
		CacheMaxAge: settings.CacheMaxAge,
		Color:       e.color,
		Columns:     settings.Columns,
		RowStyle:    settings.RowStyle,
		CheckTarget: config.CheckTarget,
		CheckFile:   config.CheckFile,
		AddTarget:   config.AddTarget,
	})
	d.run(d.model.Init())
	d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	return d
}

func (d *driver) send(msg tea.Msg) {
	var cmd tea.Cmd
	d.model, cmd = d.model.Update(msg)
	d.run(cmd)
}

func (d *driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.QuitMsg:
		d.quit = true
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	default:
		if d.hold {
			d.held = append(d.held, msg)
			return
		}
		d.send(msg)
	}
}

// next hands the program the oldest result held back. What that sets off
// is held back as well.
func (d *driver) next() {
	d.t.Helper()
	if len(d.held) == 0 {
		d.t.Fatal("nothing is held back")
	}
	msg := d.held[0]
	d.held = d.held[1:]
	d.send(msg)
}

// release stops holding back and hands the program what was held, in the
// order the work ran.
func (d *driver) release() {
	held := d.held
	d.hold, d.held = false, nil
	for _, msg := range held {
		d.send(msg)
	}
}

// releaseLastFirst is release with the results handed over last first, as
// a real program may receive them in any order.
func (d *driver) releaseLastFirst() {
	slices.Reverse(d.held)
	d.release()
}

// tick delivers the oldest pending tick, if there is one, and
// reports whether there was.
func (d *driver) tick() bool {
	if len(d.ticks) == 0 {
		return false
	}
	next := d.ticks[0]
	d.ticks = d.ticks[1:]
	d.send(next.fn(d.now))
	return true
}

// pass delivers the oldest pending tick that ends the moment in which the
// add dialog's confirmation ignores enter, as if that moment had passed.
func (d *driver) pass() {
	d.t.Helper()
	i := slices.IndexFunc(d.ticks, func(p pendingTick) bool { return p.after == d.pause })
	if i < 0 {
		d.t.Fatalf("no tick of %v is pending", d.pause)
	}
	next := d.ticks[i]
	d.ticks = slices.Delete(d.ticks, i, i+1)
	d.send(next.fn(d.now))
}

var namedKeys = map[string]tea.KeyPressMsg{
	"down":      {Code: tea.KeyDown},
	"up":        {Code: tea.KeyUp},
	"enter":     {Code: tea.KeyEnter},
	"tab":       {Code: tea.KeyTab},
	"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
	"esc":       {Code: tea.KeyEscape},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+d":    {Code: 'd', Mod: tea.ModCtrl},
	"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
	"ctrl+n":    {Code: 'n', Mod: tea.ModCtrl},
	"ctrl+p":    {Code: 'p', Mod: tea.ModCtrl},
	"backspace": {Code: tea.KeyBackspace},
	"space":     {Code: tea.KeySpace, Text: " "},
}

// press sends each key in turn: a named key such as "down" or "ctrl+c",
// or a single printable character.
func (d *driver) press(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		msg, ok := namedKeys[k]
		if !ok {
			r := []rune(k)
			if len(r) != 1 {
				d.t.Fatalf("unknown key %q", k)
			}
			msg = tea.KeyPressMsg{Code: r[0], Text: k}
		}
		d.send(msg)
	}
}

// paste sends text as the terminal sends a paste.
func (d *driver) paste(text string) {
	d.send(tea.PasteMsg{Content: text})
}

func (d *driver) screen() string {
	return d.model.View().Content
}

// selected returns the text of the selected row, without its marker.
func (d *driver) selected() string {
	d.t.Helper()
	for _, line := range strings.Split(d.screen(), "\n") {
		if rest, ok := strings.CutPrefix(line, " > "); ok {
			return strings.Join(strings.Fields(rest), " ")
		}
	}
	d.t.Fatalf("no selected row on screen:\n%s", d.screen())
	return ""
}

// hasRow reports whether a line of the screen starts with the words of
// want, ignoring the margin, the selection marker and the spacing between
// columns, so a test names only the columns it is about.
func hasRow(screen, want string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if startsWith(strings.TrimPrefix(line, " > "), want) {
			return true
		}
	}
	return false
}

// hasExactRow is hasRow for a whole row, every column included.
func hasExactRow(screen, want string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Join(strings.Fields(strings.TrimPrefix(line, " > ")), " ") == want {
			return true
		}
	}
	return false
}

// startsWith reports whether the words of row start with the words of want,
// compared as whole words.
func startsWith(row, want string) bool {
	words, prefix := strings.Fields(row), strings.Fields(want)
	return len(words) >= len(prefix) && slices.Equal(words[:len(prefix)], prefix)
}
