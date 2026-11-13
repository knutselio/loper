package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/creack/pty"
)

// colours returns the parameters of the SGR sequences in out that set or
// reset a colour: foreground, background or underline colour.
func colours(out []byte) []string {
	var found []string
	for _, seq := range sgr.FindAll(out, -1) {
		for _, p := range strings.Split(string(seq[2:len(seq)-1]), ";") {
			n, _ := strconv.Atoi(strings.SplitN(p, ":", 2)[0])
			if 30 <= n && n <= 49 || 90 <= n && n <= 107 || n == 58 || n == 59 {
				found = append(found, p)
			}
		}
	}
	return found
}

var (
	// erase is how the renderer starts drawing the whole screen.
	erase = regexp.MustCompile(`\x1b\[2J`)
	// underline is a style that turns underline on.
	underline = regexp.MustCompile(`\x1b\[([0-9]+;)*4(;[0-9]+)*m`)
)

func TestTheBuiltBinaryWithNoColorSetDrawsTheStylesWithoutColour(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// run runs loper with NO_COLOR set to value, searches for "a", widens
	// the terminal so the screen is drawn again in full, and returns all the
	// terminal showed and that last full screen.
	run := func(value string) (out, screen []byte) {
		home := tempHome(t)
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("NO_COLOR", value)
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		for _, name := range []string{"alpha", "alps"} {
			if out, err := exec.Command("git", "init", "-b", "main", filepath.Join(home, "code", name)).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
		}
		config := filepath.Join(home, "try.toml")
		writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Never\"\nkey = \"z\"\ncommand = %q\n",
			filepath.Join(home, "code"), filepath.Join(home, "never")+" {path}"))

		s := start(t, "alps", loper, "--config", config)
		s.within("the list to be drawn", s.shown)
		// count returns how often the terminal has shown what pattern matches.
		count := func(pattern *regexp.Regexp) int {
			s.mu.Lock()
			defer s.mu.Unlock()
			return len(pattern.FindAll(s.out, -1))
		}
		s.press("/a")
		s.eventually("the match to be underlined", func() bool { return count(underline) > 0 })
		erased := count(erase)
		if err := pty.Setsize(s.pty, &pty.Winsize{Rows: 24, Cols: 100}); err != nil {
			t.Fatal(err)
		}
		s.eventually("the screen to be drawn again in full", func() bool { return count(erase) > erased })
		s.press("\x03")
		s.within("loper to quit", s.exited)
		s.within("the terminal to close", s.drained)
		all := erase.FindAllIndex(s.out, -1)
		return s.out, s.out[all[len(all)-1][0]:]
	}

	out, screen := run("1")
	if got := colours(out); len(got) > 0 {
		t.Errorf("with NO_COLOR=1 the terminal got the colours %q", got)
	}
	for _, c := range []struct{ what, pattern string }{
		{"the selected row in reverse video", `\x1b\[7m \x1b\[1m>`},
		{"the matched letters underlined", `\x1b\[4(;7)?ma\x1b`},
		{"the keys bold", `\x1b\[(22;)?1mz\x1b\[m Never`},
		{"the folders dimmed", `\x1b\[2m~/code`},
	} {
		if !regexp.MustCompile(c.pattern).Match(screen) {
			t.Errorf("with NO_COLOR=1, want %s (%s):\n%q", c.what, c.pattern, screen)
		}
	}
	for _, value := range []string{"yes", "false"} {
		got, gotScreen := run(value)
		if colours := colours(got); len(colours) > 0 {
			t.Errorf("with NO_COLOR=%s the terminal got the colours %q", value, colours)
		}
		if !bytes.Equal(gotScreen, screen) {
			t.Errorf("with NO_COLOR=%s the screen is\n%q\nwant the one of NO_COLOR=1\n%q", value, gotScreen, screen)
		}
	}
	if _, screen := run(""); len(colours(screen)) == 0 {
		t.Errorf("with NO_COLOR empty the screen has no colour:\n%q", screen)
	}
}

func TestTheBuiltBinaryDrawsAConfigFileNamedWithAColourEscapedAndWithoutColour(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	for _, value := range []string{"yes", "1"} {
		t.Run("NO_COLOR="+value, func(t *testing.T) {
			home := tempHome(t)
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("NO_COLOR", value)
			config := filepath.Join(home, "x\x1b[31mred", "c.toml")
			writeConfig(t, config, "paths = []\n")

			s := start(t, `Change paths in ~/x\x1b[31mred/c.toml`, loper, "--config", config)
			s.within("the config file to be named, escaped", s.shown)
			out := s.quit()

			if got := colours(out); len(got) > 0 {
				t.Errorf("the terminal got the colours %q", got)
			}
		})
	}
}
