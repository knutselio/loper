package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
