package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// session is the built binary running in a pseudo-terminal.
type session struct {
	t       *testing.T
	pty     *os.File
	shown   chan struct{} // closed once the terminal has shown the awaited text
	exited  chan struct{} // closed once the process has been reaped
	waitErr error         // the process's exit, once exited is closed
	drained chan struct{} // closed once the terminal has nothing more to read
	out     []byte        // all the terminal showed, complete once drained is closed
}

// start runs the binary with args in a 24x80 pseudo-terminal and closes
// shown once the terminal has shown awaited. The process is killed and
// reaped when the test ends, should it still be running.
func start(t *testing.T, awaited string, binary string, args ...string) *session {
	t.Helper()
	cmd := exec.Command(binary, args...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, pty: f, shown: make(chan struct{}), exited: make(chan struct{}), drained: make(chan struct{})}
	go func() {
		s.waitErr = cmd.Wait()
		close(s.exited)
	}()
	// Reading the terminal all along keeps the program from blocking on a
	// full buffer.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			s.out = append(s.out, buf[:n]...)
			if awaited != "" && bytes.Contains(s.out, []byte(awaited)) {
				close(s.shown)
				awaited = ""
			}
			if err != nil { // the terminal closed with the process
				break
			}
		}
		close(s.drained)
	}()
	t.Cleanup(func() {
		select {
		case <-s.exited:
		default:
			_ = cmd.Process.Kill() // the test failed with the program still running
			<-s.exited
		}
		f.Close()
		<-s.drained
		if t.Failed() {
			t.Logf("the terminal showed:\n%q", s.out)
		}
	})
	return s
}

// within waits for done, failing the test after a generous deadline.
func (s *session) within(what string, done <-chan struct{}) {
	s.t.Helper()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		s.t.Fatalf("timed out waiting for %s", what)
	}
}

// press types keys into the terminal.
func (s *session) press(keys string) {
	s.t.Helper()
	if _, err := s.pty.WriteString(keys); err != nil {
		s.t.Fatal(err)
	}
}

// eventually calls try until it reports true, failing the test when that
// takes longer than a generous deadline or the program exits first.
func (s *session) eventually(what string, try func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !try() {
		select {
		case <-s.exited:
			s.t.Fatalf("loper exited (%v) while waiting for %s", s.waitErr, what)
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// quit presses q, waits for the program to exit successfully and returns
// all the terminal showed.
func (s *session) quit() []byte {
	s.t.Helper()
	s.press("q")
	s.within("loper to quit on q", s.exited)
	if s.waitErr != nil {
		s.t.Fatalf("loper exited with %v", s.waitErr)
	}
	s.within("the terminal to close", s.drained)
	return s.out
}

func TestTheBuiltBinaryOpensTheRepoPickedWithKeysAndRecordsIt(t *testing.T) {
	// Built before HOME moves, as go build keeps its caches under the home.
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	home := tempHome(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	alpha, beta := filepath.Join(home, "code", "alpha"), filepath.Join(home, "code", "beta")
	for _, repo := range []string{alpha, beta} {
		if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
	}
	record := filepath.Join(home, "record")
	fake := filepath.Join(home, "record-target")
	writeConfig(t, fake, "#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+record+".tmp'\nmv '"+record+".tmp' '"+record+"'\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "try.toml")
	writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Record\"\ncommand = %q\n",
		filepath.Join(home, "code"), fake+" {path}"))

	s := start(t, "beta", loper, "--config", config)
	// Keys that arrive before the list is drawn would act on an empty list.
	s.within("the list to be drawn", s.shown)
	s.press("j\r\r") // down to beta, open the menu, run its only target
	s.eventually("the target to run", func() bool {
		_, err := os.Stat(record)
		return err == nil
	})
	historyFile := filepath.Join(home, ".local", "state", "loper", "history.json")
	var history map[string]time.Time
	s.eventually("the open to be recorded", func() bool {
		b, _ := os.ReadFile(historyFile)
		return json.Unmarshal(b, &history) == nil
	})
	out := s.quit()

	if b, _ := os.ReadFile(record); string(b) != beta+"\n" {
		t.Errorf("the target was called with %q, want %q", b, beta)
	}
	if _, ok := history[beta]; !ok || len(history) != 1 {
		t.Errorf("the history holds %v, want only %q", history, beta)
	}
	// NO_COLOR is set, so no colour or text style was drawn.
	if sgr := regexp.MustCompile(`\x1b\[[0-9;:]*m`).Find(out); sgr != nil {
		t.Errorf("with NO_COLOR set the terminal got the style %q", sgr)
	}
}

func TestTheBuiltBinaryHandsTheWindowOverToAnInPlaceTarget(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	home := tempHome(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	alpha := filepath.Join(home, "code", "alpha")
	if out, err := exec.Command("git", "init", "-b", "main", alpha).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	record := filepath.Join(home, "record")
	fake := filepath.Join(home, "in-place")
	writeConfig(t, fake, "#!/bin/sh\n{ pwd; printf '%s\\n' \"$@\"; } > '"+record+"'\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "try.toml")
	writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Here\"\nkey = \"h\"\ncommand = %q\nin_place = true\n",
		filepath.Join(home, "code"), fake+" {path}"))

	s := start(t, "alpha", loper, "--config", config)
	s.within("the list to be drawn", s.shown)
	s.press("h")
	s.within("the target to end in loper's place", s.exited)

	if s.waitErr != nil {
		t.Errorf("the process exited with %v, want the target's 0", s.waitErr)
	}
	if b, _ := os.ReadFile(record); string(b) != alpha+"\n"+alpha+"\n" {
		t.Errorf("the target recorded %q, want it ran in %q with it as its argument", b, alpha)
	}
}

func TestTheBuiltBinaryQuitDuringAnOpenWaitsOnQAndNotOnCtrlC(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	for _, c := range []struct {
		name, key string
		recorded  bool
	}{
		{"q", "q", true},
		{"ctrl-c", "\x03", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := tempHome(t)
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("NO_COLOR", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			alpha := filepath.Join(home, "code", "alpha")
			if out, err := exec.Command("git", "init", "-b", "main", alpha).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			// The target records its process id and keeps running, past loper's
			// wait, until the test ends; it gives up by itself after 20 seconds.
			pidFile, done := filepath.Join(home, "server.pid"), filepath.Join(home, "server.done")
			fake := filepath.Join(home, "server")
			writeConfig(t, fake, "#!/bin/sh\necho $$ > '"+pidFile+".tmp'\nmv '"+pidFile+".tmp' '"+pidFile+"'\n"+
				"for i in $(seq 400); do [ -e '"+done+"' ] && exit 0; sleep 0.05; done\n")
			if err := os.Chmod(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				writeConfig(t, done, "")
				if p := readPid(pidFile); p > 0 {
					syscall.Kill(p, syscall.SIGKILL)
				}
			})
			config := filepath.Join(home, "try.toml")
			writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Server\"\nkey = \"v\"\ncommand = %q\n",
				filepath.Join(home, "code"), fake+" {path}"))

			s := start(t, "alpha", loper, "--config", config)
			s.within("the list to be drawn", s.shown)
			s.press("v")
			s.eventually("the target to start", func() bool { return readPid(pidFile) > 0 })
			s.press(c.key)
			// The target has only just started and loper's wait is 2 seconds,
			// so the key arrives during the wait: the entry, written when the
			// wait ends, is not there yet.
			if _, err := os.Stat(filepath.Join(home, ".local", "state", "loper", "history.json")); !os.IsNotExist(err) {
				t.Fatalf("the open was recorded (%v) before %s was pressed", err, c.name)
			}
			s.within("loper to quit", s.exited)
			s.within("the terminal to close", s.drained)

			if c.recorded && s.waitErr != nil {
				t.Errorf("loper exited with %v, want 0", s.waitErr)
			}
			var history map[string]time.Time
			b, _ := os.ReadFile(filepath.Join(home, ".local", "state", "loper", "history.json"))
			_ = json.Unmarshal(b, &history)
			if _, ok := history[alpha]; ok != c.recorded {
				t.Errorf("the history holds %v; want the open of alpha recorded: %v", history, c.recorded)
			}
			if p := readPid(pidFile); syscall.Kill(p, 0) != nil {
				t.Errorf("the target %d is gone, want it left running", p)
			}
		})
	}
}

// readPid returns the process id in file, or 0 when there is none yet.
func readPid(file string) int {
	b, _ := os.ReadFile(file)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func TestTheBuiltBinaryOnAFirstRunLandsInTheRepoWithOrWithoutTheWrapper(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// setUp makes a first run's home with one repo, alpha, and a fake shell
	// in $SHELL that records its folder and its words, and returns them.
	setUp := func(t *testing.T) (home, alpha, shell, record string) {
		home = tempHome(t)
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("NO_COLOR", "1")
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		alpha = filepath.Join(home, "code", "alpha")
		if out, err := exec.Command("git", "init", "-b", "main", alpha).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		record = filepath.Join(home, "shell-record")
		shell = filepath.Join(home, "bin", "shell")
		writeConfig(t, shell, "#!/bin/sh\n{ pwd; printf '%s\\n' \"$0\" \"$@\"; } > '"+record+"'\n")
		if err := os.Chmod(shell, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SHELL", shell)
		return home, alpha, shell, record
	}
	// recorded reports whether the history holds only alpha.
	recorded := func(home, alpha string) bool {
		var history map[string]time.Time
		b, _ := os.ReadFile(filepath.Join(home, ".local", "state", "loper", "history.json"))
		if json.Unmarshal(b, &history) != nil {
			return false
		}
		_, ok := history[alpha]
		return ok && len(history) == 1
	}

	t.Run("with the wrapper's file", func(t *testing.T) {
		home, alpha, _, record := setUp(t)
		file := filepath.Join(home, "cd-file")
		writeConfig(t, file, "")
		t.Setenv("LOPER_CD_FILE", file)

		s := start(t, "o cd", loper)
		s.within("the list and the cd target to be drawn", s.shown)
		s.press("o")
		s.within("loper to end", s.exited)

		if s.waitErr != nil {
			t.Errorf("loper exited with %v, want 0", s.waitErr)
		}
		if b, _ := os.ReadFile(file); string(b) != alpha {
			t.Errorf("the wrapper's file holds %q, want %q", b, alpha)
		}
		if b, err := os.ReadFile(record); err == nil {
			t.Errorf("the shell ran, recording %q; want nothing handed over", b)
		}
		if !recorded(home, alpha) {
			t.Errorf("the open of %s was not recorded", alpha)
		}
		configFile := filepath.Join(home, ".config", "loper", "config.toml")
		if b, _ := os.ReadFile(configFile); string(b) != firstRun {
			t.Errorf("config file is %q, want %q", b, firstRun)
		}
		if info, err := os.Stat(configFile); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("config file: %v, %v; want mode 0600", info, err)
		}
	})

	t.Run("without the wrapper", func(t *testing.T) {
		home, alpha, shell, record := setUp(t)

		s := start(t, "o cd", loper)
		s.within("the list and the cd target to be drawn", s.shown)
		s.press("o")
		s.within("the shell to end in loper's place", s.exited)

		if s.waitErr != nil {
			t.Errorf("the process exited with %v, want the shell's 0", s.waitErr)
		}
		if b, _ := os.ReadFile(record); string(b) != alpha+"\n"+shell+"\n" {
			t.Errorf("the shell recorded %q, want it ran in %q with no arguments", b, alpha)
		}
		if !recorded(home, alpha) {
			t.Errorf("the open of %s was not recorded", alpha)
		}
	})

	t.Run("with the wrapper's file and a cd to a folder in the repo", func(t *testing.T) {
		home, alpha, _, _ := setUp(t)
		sub := filepath.Join(alpha, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"),
			"[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}/sub\"\nin_place = true\n")
		file := filepath.Join(home, "cd-file")
		writeConfig(t, file, "")
		t.Setenv("LOPER_CD_FILE", file)

		s := start(t, "o cd", loper)
		s.within("the list and the cd target to be drawn", s.shown)
		s.press("o")
		s.within("loper to end", s.exited)

		if b, _ := os.ReadFile(file); s.waitErr != nil || string(b) != sub {
			t.Errorf("exit %v, the wrapper's file holds %q; want 0 and %q", s.waitErr, b, sub)
		}
	})

	t.Run("with the wrapper's file kept from the targets", func(t *testing.T) {
		home, alpha, _, _ := setUp(t)
		normal, inPlace := filepath.Join(home, "normal-env"), filepath.Join(home, "in-place-env")
		for name, record := range map[string]string{"normal": normal, "in-place": inPlace} {
			writeConfig(t, filepath.Join(home, "bin", name), "#!/bin/sh\nenv > '"+record+".tmp'\nmv '"+record+".tmp' '"+record+"'\n")
			if err := os.Chmod(filepath.Join(home, "bin", name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), fmt.Sprintf(
			"[[targets]]\nname = \"Normal\"\nkey = \"n\"\ncommand = %q\n\n[[targets]]\nname = \"Here\"\nkey = \"h\"\ncommand = %q\nin_place = true\n",
			filepath.Join(home, "bin", "normal")+" {path}", filepath.Join(home, "bin", "in-place")+" {path}"))
		file := filepath.Join(home, "cd-file")
		writeConfig(t, file, "")
		t.Setenv("LOPER_CD_FILE", file)

		s := start(t, "alpha", loper)
		s.within("the list and the targets to be drawn", s.shown)
		s.press("n")
		s.eventually("the normal target to run", func() bool {
			_, err := os.Stat(normal)
			return err == nil
		})
		// Until its open is done, the in-place target's key does nothing.
		s.eventually("the normal open to be recorded", func() bool { return recorded(home, alpha) })
		s.press("h")
		s.within("the in-place target to end in loper's place", s.exited)

		for _, record := range []string{normal, inPlace} {
			b, err := os.ReadFile(record)
			if err != nil || bytes.Contains(b, []byte("LOPER_CD_FILE")) {
				t.Errorf("%s: %v; the target's environment has LOPER_CD_FILE or is missing:\n%s", filepath.Base(record), err, b)
			}
		}
		if b, _ := os.ReadFile(file); len(b) != 0 {
			t.Errorf("the wrapper's file holds %q, want nothing", b)
		}
	})

	t.Run("with a config folder that cannot be written", func(t *testing.T) {
		home, _, _, _ := setUp(t)
		folder := filepath.Join(home, ".config", "loper")
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(folder, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(folder, 0o700) })

		s := start(t, "The config file could not be written: permission denied", loper)
		s.within("the notice to be drawn", s.shown)
		s.quit()

		if _, err := os.Stat(filepath.Join(folder, "config.toml")); !os.IsNotExist(err) {
			t.Errorf("a config file was written (%v)", err)
		}
	})

	t.Run("with a file that cannot be written", func(t *testing.T) {
		home, _, _, record := setUp(t)
		file := filepath.Join(home, "gone", "cd-file")
		t.Setenv("LOPER_CD_FILE", file)

		s := start(t, "o cd", loper)
		s.within("the list and the cd target to be drawn", s.shown)
		s.press("o")
		s.within("loper to end", s.exited)
		s.within("the terminal to close", s.drained)

		var exit *exec.ExitError
		if !errors.As(s.waitErr, &exit) || exit.ExitCode() != 1 {
			t.Errorf("loper exited with %v, want 1", s.waitErr)
		}
		if want := "loper: open " + file + ": no such file or directory"; !bytes.Contains(s.out, []byte(want)) {
			t.Errorf("the terminal does not show %q", want)
		}
		if b, err := os.ReadFile(record); err == nil {
			t.Errorf("the shell ran, recording %q; want nothing handed over", b)
		}
	})
}
