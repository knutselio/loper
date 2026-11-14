package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// privateMode matches a sequence that switches a DEC private mode on (h) or
// off (l), such as the alternate screen, the cursor or bracketed paste.
var privateMode = regexp.MustCompile(`\x1b\[\?(\d+)([hl])`)

// keyboard matches a sequence that changes how the terminal reports keys:
// modifyOtherKeys on (>4;2m) or off (>4m), and a push (>…u) or pop (<…u) of
// the Kitty keyboard flags.
var keyboard = regexp.MustCompile(`\x1b\[(>4;2m|>4m|>\d+u|<(\d+)u)`)

// switchedOn returns the terminal modes that out leaves switched on, by the
// sequences Bubble Tea v2.1.1 writes: its renderer (cursed_renderer.go)
// turns on the alternate screen (?1049), bracketed paste (?2004),
// modifyOtherKeys and the Kitty keyboard flags at the start and hides the
// cursor (?25l) while it draws, and its close undoes each. A private mode is
// on when its last sequence switched it on, the cursor when its last one hid
// it; the Kitty flags are on while there were more pushes than pops.
func switchedOn(out []byte) []string {
	last := map[string]string{}
	for _, m := range privateMode.FindAllSubmatch(out, -1) {
		last[string(m[1])] = string(m[2])
	}
	var on []string
	for mode, state := range last {
		if mode == "25" && state == "l" || mode != "25" && state == "h" {
			on = append(on, "?"+mode+state)
		}
	}
	slices.Sort(on)
	otherKeys, kitty := false, 0
	for _, m := range keyboard.FindAllSubmatch(out, -1) {
		switch s := string(m[1]); {
		case s == ">4;2m":
			otherKeys = true
		case s == ">4m":
			otherKeys = false
		case s[0] == '>':
			kitty++
		default:
			n, _ := strconv.Atoi(string(m[2]))
			kitty = max(kitty-n, 0)
		}
	}
	if otherKeys {
		on = append(on, "modifyOtherKeys")
	}
	if kitty > 0 {
		on = append(on, fmt.Sprintf("%d Kitty keyboard flags pushed", kitty))
	}
	return on
}

// sttySettings returns the terminal settings that stty -g wrote to file, by
// field, such as lflag, eof or ispeed, in hexadecimal, with one flag left
// out: PENDIN in lflag. macOS sets it whenever a terminal is switched back
// to canonical input, so that what was typed in raw mode is read again, and
// clears it at the next read, so it differs after any program that restores
// the settings, also outside loper. Every other setting must be as it was.
func sttySettings(t *testing.T, file string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, f := range strings.Split(strings.TrimSpace(string(b)), ":")[1:] { // after the format, gfmt1
		name, value, _ := strings.Cut(f, "=")
		fields[name] = value
	}
	lflag, err := strconv.ParseUint(fields["lflag"], 16, 64)
	if err != nil || len(fields) < 5 {
		t.Fatalf("%s holds %q, not what stty -g prints", file, b)
	}
	fields["lflag"] = strconv.FormatUint(lflag&^syscall.PENDIN, 16)
	return fields
}

// sameSettings fails the test unless the settings in file are want.
func sameSettings(t *testing.T, want map[string]string, file, when string) {
	t.Helper()
	if got := sttySettings(t, file); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s the terminal settings are\n%v\nwant them as before loper started,\n%v", when, got, want)
	}
}

func TestTheBuiltBinaryLeavesTheTerminalAsItFoundIt(t *testing.T) {
	loper := buildLoper(t)
	for _, c := range []struct {
		name string
		keys func(s *session)
	}{
		{"q", func(s *session) { s.press("q") }},
		{"ctrl-c", func(s *session) { s.press("\x03") }},
		{"q in the error dialog", func(s *session) {
			s.press("f")
			s.eventually("the error dialog", func() bool { return s.shows("Could not open alpha in Fail") })
			s.press("q")
		}},
		{"a hand-over", func(s *session) { s.press("h") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := tempHome(t)
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("NO_COLOR", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			if out, err := exec.Command("git", "init", "-b", "main", filepath.Join(home, "code", "alpha")).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			fail, inPlace := filepath.Join(home, "bin", "fail"), filepath.Join(home, "bin", "in-place")
			observed := filepath.Join(home, "observed")
			// The failing target opens the error dialog. The in-place program
			// records the settings of the terminal it gets and says on it that
			// it runs.
			writeConfig(t, fail, "#!/bin/sh\necho failing\nexit 3\n")
			writeConfig(t, inPlace, "#!/bin/sh\nstty -g > '"+observed+"'\necho handed over\n")
			for _, script := range []string{fail, inPlace} {
				if err := os.Chmod(script, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			config := filepath.Join(home, "try.toml")
			writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Fail\"\nkey = \"f\"\ncommand = %q\n\n"+
				"[[targets]]\nname = \"Here\"\nkey = \"h\"\ncommand = %q\nin_place = true\n",
				filepath.Join(home, "code"), fail+" {path}", inPlace+" {path}"))
			// A shell on the terminal records its settings before loper starts
			// and after it, or the program it handed over to, has ended.
			before, after := filepath.Join(home, "before"), filepath.Join(home, "after")
			s := start(t, "alpha", "/bin/sh", "-c", `stty -g > "$0"; after=$1; shift; "$@"; code=$?; stty -g > "$after"; exit $code`,
				before, after, loper, "--config", config)
			s.within("the list to be drawn", s.shown)

			c.keys(s)
			s.within("loper to end", s.exited)
			s.within("the terminal to close", s.drained)

			if s.waitErr != nil {
				t.Errorf("exited with %v, want 0", s.waitErr)
			}
			out := s.out
			if c.name == "a hand-over" {
				i := strings.Index(string(out), "handed over")
				if i < 0 {
					t.Fatal("the in-place program did not run")
				}
				out = out[:i] // the terminal is restored before the program gets it
			}
			// Loper did switch them on, so their being off shows the restore.
			for _, on := range []string{"\x1b[?1049h", "\x1b[?2004h", "\x1b[?25l", "\x1b[>4;2m"} {
				if !strings.Contains(string(out), on) {
					t.Errorf("loper never wrote %q, so its restore shows nothing", on)
				}
			}
			if on := switchedOn(out); len(on) > 0 {
				t.Errorf("loper left the terminal with %q on, want the main screen, the cursor shown and every mode it switched on off", on)
			}
			want := sttySettings(t, before)
			sameSettings(t, want, after, "at the end")
			if c.name == "a hand-over" {
				sameSettings(t, want, observed, "for the in-place program")
			}
		})
	}
}
