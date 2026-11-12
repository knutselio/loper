package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knutselio/loper/internal/config"
	"github.com/knutselio/loper/internal/target"
)

// TestHandOverInAChild is the child process of the hand-over tests: it runs
// the entry point with a UI that ends asking to hand over to the program
// LOPER_TEST_PROGRAM in the folder LOPER_TEST_HAND_OVER. A hand-over that
// works replaces this process, so it never runs in the test process itself.
func TestHandOverInAChild(t *testing.T) {
	dir := os.Getenv("LOPER_TEST_HAND_OVER")
	if dir == "" {
		t.Skip("run by the hand-over tests, in a child process")
	}
	h := target.HandOver{Program: os.Getenv("LOPER_TEST_PROGRAM"), Args: []string{"fake", "a b", "{path}"}, Dir: dir}
	os.Exit(run(nil, os.Stdout, os.Stderr, func(string, config.Settings) error { return handOver(h, takeCdFile()) }))
}

// handOverChild runs TestHandOverInAChild with the fake program body in a
// temporary home, handing over in dir, and returns its exit code and what it
// wrote to stdout and stderr.
func handOverChild(t *testing.T, home, dir, body string) (code int, stdout, stderr string) {
	t.Helper()
	program := filepath.Join(home, "bin", "fake")
	writeConfig(t, program, "#!/bin/sh\n"+body)
	if err := os.Chmod(program, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHandOverInAChild$")
	cmd.Env = append(os.Environ(), "LOPER_TEST_HAND_OVER="+dir, "LOPER_TEST_PROGRAM="+program, "LOPER_TEST_MARK=kept")
	var out, errs strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errs.String()
}

func TestTheHandOverRunsTheCommandInTheRepoInTheToolsPlace(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(home, "code", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := handOverChild(t, home, dir,
		"pwd\nprintf '%s\\n' \"$0\" \"$@\" \"$LOPER_TEST_MARK\"\nexit 7\n")

	if want := dir + "\n" + filepath.Join(home, "bin", "fake") + "\na b\n{path}\nkept\n"; stdout != want {
		t.Errorf("the program printed %q, want its folder, its words and loper's environment, %q", stdout, want)
	}
	if code != 7 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want the program's 7 and nothing", code, stderr)
	}
}

func TestAHandOverWhoseFolderIsGoneSaysWhyAndExits1(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(home, "code", "gone")

	code, stdout, stderr := handOverChild(t, home, dir, "echo ran\n")

	if want := "loper: chdir " + dir + ": no such file or directory\n"; code != 1 || stderr != want {
		t.Errorf("exit code %d, stderr %q; want 1 and %q", code, stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout %q, want nothing: the program must not run", stdout)
	}
}

func TestAHandOverThatIsNotCdLeavesTheWrappersFileAlone(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(home, "code", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "cd-file")
	writeConfig(t, file, "")
	t.Setenv("LOPER_CD_FILE", file)

	code, stdout, _ := handOverChild(t, home, dir, "pwd\n")

	if code != 0 || stdout != dir+"\n" {
		t.Errorf("exit code %d, stdout %q; want the program to run in %q", code, stdout, dir)
	}
	if b, _ := os.ReadFile(file); len(b) != 0 {
		t.Errorf("the wrapper's file holds %q, want nothing", b)
	}
}
