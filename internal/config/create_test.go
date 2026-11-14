package config

// These tests stop and fail the first run's creation of the config file at
// each of its steps in turn, which no test outside the package can reach at a
// chosen point. They call only Open and check the files and the notice.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

// firstRunHome points the home and XDG folders at a temporary tree and
// returns the home folder and the config file a first run creates there.
func firstRunHome(t *testing.T) (home, file string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return home, filepath.Join(home, ".config", "loper", "config.toml")
}

// withStep makes f run before each step of the first run's creation, for
// the rest of the test.
func withStep(t *testing.T, f func(name string) error) {
	t.Helper()
	before := step
	step = f
	t.Cleanup(func() { step = before })
}

// steps returns the names of the steps a first run takes, in order, from
// one run that is left alone.
func steps(t *testing.T) []string {
	t.Helper()
	home, _ := firstRunHome(t)
	var names []string
	withStep(t, func(name string) error {
		names = append(names, name)
		return nil
	})
	Open("", home)
	want := []string{"make the folder", "look", "create the temporary file", "write", "link", "remove the temporary file"}
	if !slices.Equal(names, want) {
		t.Fatalf("a first run took the steps %q, want %q", names, want)
	}
	return names
}

// others returns the names in folder other than config.toml, such as a
// temporary file left behind.
func others(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != "config.toml" {
			names = append(names, e.Name())
		}
	}
	return names
}

// checkWhole fails the test unless file is absent or holds the whole
// first-run text, private to the user. It reports whether the file is there.
func checkWhole(t *testing.T, when, file string) bool {
	t.Helper()
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil || string(b) != firstRun {
		t.Errorf("%s: config file is %q (%v), want it absent or %q", when, b, err, firstRun)
	}
	if info, err := os.Stat(file); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("%s: config file has mode %v, want 0600", when, info.Mode().Perm())
	}
	return true
}

// TestAFirstRunStoppedInAChild is the child process of the stop test: it
// starts a first run and kills itself with SIGKILL before the step
// LOPER_TEST_STOP_AT, so nothing after it runs, deferred work included.
func TestAFirstRunStoppedInAChild(t *testing.T) {
	at := os.Getenv("LOPER_TEST_STOP_AT")
	if at == "" {
		t.Skip("run by the stop test, in a child process")
	}
	step = func(name string) error {
		if name == at {
			syscall.Kill(os.Getpid(), syscall.SIGKILL)
			select {} // the signal arrives; nothing more runs
		}
		return nil
	}
	Open("", os.Getenv("HOME"))
	os.Exit(3) // the step was never reached
}

func TestAFirstRunStoppedAtAnyStepLeavesNoFileOrTheWholeOneAndTheNextRunCompletesIt(t *testing.T) {
	for _, at := range steps(t) {
		home, file := firstRunHome(t)
		// Two runs stopped at the same step, as a user might do twice, unless
		// the first already left the file, so the second never gets there.
		for range 2 {
			cmd := exec.Command(os.Args[0], "-test.run=^TestAFirstRunStoppedInAChild$")
			cmd.Env = append(os.Environ(), "LOPER_TEST_STOP_AT="+at)
			err := cmd.Run()
			if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatalf("stopped before %q: the child ended with %v, want SIGKILL", at, err)
			}
			if checkWhole(t, "stopped before "+at, file) {
				break
			}
		}

		s, err := Open("", home)

		if err != nil || s.Notice != "" || len(s.Targets) != 1 || s.Targets[0].Name != "cd" {
			t.Errorf("stopped before %q, the next run: targets %+v, notice %q, error %v; want the cd target alone", at, s.Targets, s.Notice, err)
		}
		if !checkWhole(t, "stopped before "+at+", the next run", file) {
			t.Errorf("stopped before %q: the next run created no config file", at)
		}
		if left := others(t, filepath.Dir(file)); len(left) > 0 {
			t.Errorf("stopped before %q: after the next run the folder also holds %q, want nothing else", at, left)
		}
	}
}

func TestAFirstRunThatFailsAtAnyStepLeavesNoFileOrTheWholeOneAndNoTemporaryFile(t *testing.T) {
	for _, at := range steps(t) {
		home, file := firstRunHome(t)
		withStep(t, func(name string) error {
			if name == at {
				return errors.New("made to fail")
			}
			return nil
		})

		s, err := Open("", home)

		if err != nil || len(s.Targets) != 1 || s.Targets[0].Name != "cd" {
			t.Errorf("failed at %q: targets %+v, error %v; want the cd target alone", at, s.Targets, err)
		}
		if checkWhole(t, "failed at "+at, file) {
			if s.Notice != "" {
				t.Errorf("failed at %q: the file is there and the notice reads %q, want none", at, s.Notice)
			}
		} else if want := "The config file could not be written: made to fail"; s.Notice != want {
			t.Errorf("failed at %q: the notice reads %q, want %q", at, s.Notice, want)
		}
		if at == "remove the temporary file" {
			// The file is in place and its other name could not go: the next
			// start removes it.
			step = func(string) error { return nil }
			Open("", home)
		}
		if left := others(t, filepath.Dir(file)); len(left) > 0 {
			t.Errorf("failed at %q: the folder also holds %q, want nothing else", at, left)
		}
	}
}

func TestAFirstRunWhoseTemporaryFileIsTakenBeforeTheLinkSaysWhyAndLeavesNothing(t *testing.T) {
	home, file := firstRunHome(t)
	withStep(t, func(name string) error {
		if name == "link" {
			// As another start would that removes what stopped runs left.
			temps, _ := filepath.Glob(file + ".first-run.*")
			for _, temp := range temps {
				os.Remove(temp)
			}
		}
		return nil
	})

	s, err := Open("", home)

	if want := "The config file could not be written: no such file or directory"; err != nil || s.Notice != want || len(s.Targets) != 1 {
		t.Errorf("targets %+v, notice %q, error %v; want the cd target and %q", s.Targets, s.Notice, err, want)
	}
	if checkWhole(t, "temporary file taken", file) {
		t.Errorf("a config file was created")
	}
	if left := others(t, filepath.Dir(file)); len(left) > 0 {
		t.Errorf("the folder also holds %q, want nothing else", left)
	}
}

func TestAConfigFileThatAppearsDuringAFirstRunIsKeptByteForByte(t *testing.T) {
	const mine = "# mine\npaths = [\"~/mine\"]\n"
	for _, at := range steps(t) {
		home, file := firstRunHome(t)
		appeared := false
		withStep(t, func(name string) error {
			if name == at {
				if f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); err == nil {
					f.WriteString(mine)
					f.Close()
					appeared = true
				}
			}
			return nil
		})

		s, err := Open("", home)

		if !appeared {
			continue // the folder or the first run's own file was not there yet
		}
		if b, _ := os.ReadFile(file); string(b) != mine {
			t.Errorf("appeared before %q: config file is %q, want %q kept", at, b, mine)
		}
		if info, _ := os.Stat(file); info.Mode().Perm() != 0o644 {
			t.Errorf("appeared before %q: config file has mode %v, want 0644 as written", at, info.Mode().Perm())
		}
		if err != nil || s.Notice != "" || len(s.Targets) != 0 || s.Paths[0] != filepath.Join(home, "mine") {
			t.Errorf("appeared before %q: paths %q, targets %+v, notice %q, error %v; want the file's own settings", at, s.Paths, s.Targets, s.Notice, err)
		}
		if left := others(t, filepath.Dir(file)); len(left) > 0 {
			t.Errorf("appeared before %q: the folder also holds %q, want nothing else", at, left)
		}
	}
}

func TestALinkToNothingThatAppearsDuringAFirstRunIsLeftAloneAndNotWrittenThrough(t *testing.T) {
	for _, at := range steps(t) {
		home, file := firstRunHome(t)
		gone := filepath.Join(home, "gone.toml")
		appeared := false
		withStep(t, func(name string) error {
			if name == at && os.Symlink(gone, file) == nil {
				appeared = true
			}
			return nil
		})

		s, err := Open("", home)

		if !appeared {
			continue
		}
		if dest, err := os.Readlink(file); err != nil || dest != gone {
			t.Errorf("appeared before %q: config file links to %q (%v), want %q", at, dest, err, gone)
		}
		if _, err := os.Lstat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("appeared before %q: %s exists (%v), want nothing written through the link", at, gone, err)
		}
		if err != nil || len(s.Targets) != 1 || !strings.HasPrefix(s.Notice, "The config file could not be written: it is a link to ") {
			t.Errorf("appeared before %q: targets %+v, notice %q, error %v; want the cd target and the link's notice", at, s.Targets, s.Notice, err)
		}
		if left := others(t, filepath.Dir(file)); len(left) > 0 {
			t.Errorf("appeared before %q: the folder also holds %q, want nothing else", at, left)
		}
	}
}

func TestAnExistingConfigFileInAFolderThatCannotBeWrittenStartsWithoutANotice(t *testing.T) {
	home, file := firstRunHome(t)
	const mine = "paths = [\"~/mine\"]\n"
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(file), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(file), 0o700) })

	s, err := Open("", home)

	if err != nil || s.Notice != "" || s.Paths[0] != filepath.Join(home, "mine") {
		t.Errorf("paths %q, notice %q, error %v; want the file's own settings and no notice", s.Paths, s.Notice, err)
	}
	if b, _ := os.ReadFile(file); string(b) != mine {
		t.Errorf("config file is %q, want %q", b, mine)
	}
}

func TestAStartRemovesWhatStoppedFirstRunsLeftAndKeepsTheUsersOwnFiles(t *testing.T) {
	home, file := firstRunHome(t)
	folder := filepath.Dir(file)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	leftovers := map[string]string{
		"config.toml.first-run.1": "",
		"config.toml.first-run.2": firstRun[:20],
		"config.toml.first-run.3": firstRun,
	}
	for name, body := range leftovers {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	notes := filepath.Join(folder, "config.toml.first-run.notes")
	if err := os.WriteFile(notes, []byte("# my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(folder, "config.toml.first-run.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A link to a real leftover's text: it is the user's all the same.
	if err := os.Symlink(filepath.Join(folder, "config.toml.first-run.3"), filepath.Join(folder, "config.toml.first-run.link")); err != nil {
		t.Fatal(err)
	}

	Open("", home)

	if b, _ := os.ReadFile(notes); string(b) != "# my notes\n" {
		t.Errorf("the user's notes are %q, want them kept", b)
	}
	if info, err := os.Stat(filepath.Join(folder, "config.toml.first-run.d")); err != nil || !info.IsDir() {
		t.Errorf("the user's folder is gone (%v), want it kept", err)
	}
	if _, err := os.Lstat(filepath.Join(folder, "config.toml.first-run.link")); err != nil {
		t.Errorf("the user's link is gone (%v), want it kept", err)
	}
	left := others(t, folder)
	for name := range leftovers {
		if slices.Contains(left, name) {
			t.Errorf("the leftover %s with %q is still there, want it removed", name, leftovers[name])
		}
	}
	if want := []string{"config.toml.first-run.d", "config.toml.first-run.link", "config.toml.first-run.notes"}; !slices.Equal(left, want) {
		t.Errorf("the folder also holds %q, want %q", left, want)
	}
}
