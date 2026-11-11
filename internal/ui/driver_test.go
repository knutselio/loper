package ui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/config"
	"github.com/knutselio/loper/internal/repoinfo"
	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
)

// env is a test's fixed environment: a temporary home with its config,
// cache and state folders, and a fake zed first on the PATH, so the built-in
// Zed target never runs the real program.
type env struct {
	t    *testing.T
	root string // holds the home folder, the fake targets and their records
	home string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := filepath.Join(t.TempDir(), "temp folder") // a space, so commands must quote paths
	e := &env{t: t, root: root, home: filepath.Join(root, "home")}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(filepath.ListSeparator)+os.Getenv("PATH"))
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
	script := filepath.Join(e.root, "bin", name)
	record := filepath.Join(e.root, name)
	e.writeFile(script, "#!/bin/sh\npwd > '"+record+".dir'\nprintf '%s\\n' \"$@\" > '"+record+".args'\n")
	if err := os.Chmod(script, 0o755); err != nil {
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
}

// start launches the program on a fixed 80x24 screen and clock, with the
// settings loaded from the config file as the entry point does.
func (e *env) start() *driver {
	e.t.Helper()
	settings, err := config.Load("", e.home)
	if err != nil {
		e.t.Fatal(err)
	}
	d := &driver{t: e.t}
	d.model = ui.New(ui.Deps{
		Home:    e.home,
		Paths:   settings.Paths,
		Scan:    scanner.Scan,
		Branch:  repoinfo.Branch,
		Open:    target.Run,
		Targets: settings.Targets,
		Now:     func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) },
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
		d.send(msg)
	}
}

var namedKeys = map[string]tea.KeyPressMsg{
	"down":   {Code: tea.KeyDown},
	"up":     {Code: tea.KeyUp},
	"enter":  {Code: tea.KeyEnter},
	"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+d": {Code: 'd', Mod: tea.ModCtrl},
	"ctrl+u": {Code: 'u', Mod: tea.ModCtrl},
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

// hasRow reports whether a line of the screen reads as want, ignoring the
// margin, the selection marker and the spacing between columns.
func hasRow(screen, want string) bool {
	for _, line := range strings.Split(screen, "\n") {
		line = strings.TrimPrefix(line, " > ")
		if strings.Join(strings.Fields(line), " ") == want {
			return true
		}
	}
	return false
}
