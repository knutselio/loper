package ui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
)

// env is a test's fixed environment: a temporary home with its config,
// cache and state folders, and a fake target that records its arguments.
type env struct {
	t    *testing.T
	home string
	log  string // where the fake target writes its arguments
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{t: t, home: filepath.Join(root, "home"), log: filepath.Join(root, "target.log")}
	t.Setenv("HOME", e.home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(e.home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(e.home, ".local", "state"))
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.MkdirAll(e.home, 0o755); err != nil {
		t.Fatal(err)
	}
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

// fakeTarget writes a script that records each argument on its own line.
func (e *env) fakeTarget() target.Target {
	e.t.Helper()
	script := filepath.Join(filepath.Dir(e.log), "fake-target")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + e.log + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		e.t.Fatal(err)
	}
	return target.Target{Name: "Fake", Command: script + " {path}"}
}

// targetArgs returns the arguments the fake target was called with, or nil
// if it never ran.
func (e *env) targetArgs() []string {
	e.t.Helper()
	b, err := os.ReadFile(e.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// driver feeds keys to the real program one at a time and runs any
// resulting background work on the spot.
type driver struct {
	t     *testing.T
	model tea.Model
	quit  bool
}

// start launches the program on a fixed 80x24 screen and clock, scanning
// ~/code as the entry point does.
func (e *env) start() *driver {
	e.t.Helper()
	d := &driver{t: e.t}
	d.model = ui.New(ui.Deps{
		Home:   e.home,
		Paths:  []string{filepath.Join(e.home, "code")},
		Scan:   scanner.Scan,
		Open:   target.Run,
		Target: e.fakeTarget(),
		Now:    func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) },
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
		if rest, ok := strings.CutPrefix(line, "> "); ok {
			return strings.Join(strings.Fields(rest), " ")
		}
	}
	d.t.Fatalf("no selected row on screen:\n%s", d.screen())
	return ""
}
