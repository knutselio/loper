package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// buildLoper builds the binary into a temporary folder and returns its path.
// It runs before HOME moves, as go build keeps its caches under the home.
func buildLoper(t *testing.T) string {
	t.Helper()
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return loper
}

// drainsOf returns the resident memory in KiB of each running drain that the
// binary loper started, by process id.
func drainsOf(t *testing.T, loper string) map[int]int {
	t.Helper()
	out, err := exec.Command("ps", "-axo", "pid=,rss=,command=").Output()
	if err != nil {
		t.Fatal(err)
	}
	found := map[int]int{}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 4 && f[2] == loper && f[3] == drainArg {
			pid, _ := strconv.Atoi(f[0])
			rss, _ := strconv.Atoi(f[1])
			found[pid] = rss
		}
	}
	return found
}

// poll calls try until it reports true, failing the test after a generous
// deadline. Unlike a session's eventually it does not care whether loper runs.
func poll(t *testing.T, what string, try func() bool) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); !try(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// writer is a fake target that keeps running and writes when the test says
// so, with its files at r plus a suffix: it records its process id in .pid,
// in .kind whether its stdout and stderr are pipes, and in .fds which of the
// descriptors 3 to 63 it has open. Then, for each of two rounds, it waits for
// .go1 or .go2, writes 8 MiB to its stdout and, if all of it was written,
// creates .wrote1 or .wrote2. It ends when .done appears, or by itself after
// 30 seconds of waiting.
func writer(r string) string {
	return "#!/bin/sh\nr='" + r + "'\n" + `await() {
	for i in $(seq 600); do [ -e "$r.$1" ] && return; [ -e "$r.done" ] && exit 0; sleep 0.05; done
	exit 9
}
echo $$ > "$r.pid.tmp" && mv "$r.pid.tmp" "$r.pid"
kind=other
[ -p /dev/fd/1 ] && [ -p /dev/fd/2 ] && kind=pipe
echo $kind > "$r.kind"
fds=
for fd in $(seq 3 63); do [ -e /dev/fd/$fd ] && fds="$fds $fd"; done
echo "$fds" > "$r.fds"
for round in 1 2; do
	await go$round
	dd if=/dev/zero bs=65536 count=128 2>/dev/null && : > "$r.wrote$round"
done
await done
`
}

// writerHome makes a home with the repo alpha, the writer script at
// home/writer and a temporary folder of its own in TMPDIR, and returns the
// home. When the test ends it tells the writer to end, and kills it and any
// drain loper left running.
func writerHome(t *testing.T, loper string) string {
	t.Helper()
	home := tempHome(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.Mkdir(filepath.Join(home, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if out, err := exec.Command("git", "init", "-b", "main", filepath.Join(home, "code", "alpha")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	r := filepath.Join(home, "writer")
	writeConfig(t, r, writer(r))
	if err := os.Chmod(r, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		writeConfig(t, r+".done", "")
		if p := readPid(r + ".pid"); p > 0 {
			syscall.Kill(p, syscall.SIGKILL)
		}
		for p := range drainsOf(t, loper) {
			syscall.Kill(p, syscall.SIGKILL)
		}
	})
	return home
}

// writerConfig writes a config with the target Writer on the key w, which
// runs command, and returns its path.
func writerConfig(t *testing.T, home, command string) string {
	t.Helper()
	config := filepath.Join(home, "try.toml")
	writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Writer\"\nkey = \"w\"\ncommand = %q\n",
		filepath.Join(home, "code"), command))
	return config
}

// theDrain returns the one drain loper started, checking that it runs in a
// session of its own, away from the terminal's signals.
func theDrain(t *testing.T, loper string) int {
	t.Helper()
	drains := drainsOf(t, loper)
	if len(drains) != 1 {
		t.Fatalf("loper runs the drains %v, want one", drains)
	}
	for p := range drains {
		if sid, err := syscall.Getsid(p); err != nil || sid != p {
			t.Errorf("the drain %d is in session %d (%v), want one of its own", p, sid, err)
		}
		return p
	}
	return 0
}

// checkWriter checks that the writer's output is a pipe and that it got no
// descriptor beyond its stdout and stderr.
func checkWriter(t *testing.T, r string) {
	t.Helper()
	poll(t, "the target to record its descriptors", func() bool {
		_, err := os.Stat(r + ".fds")
		return err == nil
	})
	if b, _ := os.ReadFile(r + ".kind"); string(b) != "pipe\n" {
		t.Errorf("the target's stdout and stderr are %q, want pipe: a file would keep what it writes", b)
	}
	if b, _ := os.ReadFile(r + ".fds"); string(b) != "\n" {
		t.Errorf("the target has the descriptors %q open, want only stdin, stdout and stderr", b)
	}
}

// checkDrainFiles checks that the drain d has only pipes, the null device
// and its kqueue open, so no file of any kind, also not a removed one, can
// keep what it reads, and that it keeps no folder busy but the root.
func checkDrainFiles(t *testing.T, d int) {
	t.Helper()
	out, err := exec.Command("lsof", "-n", "-P", "-p", strconv.Itoa(d), "-F", "ftn").Output()
	if err != nil {
		t.Fatalf("lsof: %v", err)
	}
	var fd, kind string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch v := line[1:]; line[0] {
		case 'f':
			fd, kind = v, ""
		case 't':
			kind = v
		case 'n':
			if _, err := strconv.Atoi(fd); err != nil {
				if fd == "cwd" && v != "/" {
					t.Errorf("the drain works in %s, want the root folder", v)
				}
				continue
			}
			if !(kind == "PIPE" || kind == "KQUEUE" || kind == "CHR" && v == os.DevNull) {
				t.Errorf("the drain has %s open on descriptor %s, a %s; want only pipes, %s and its kqueue", v, fd, kind, os.DevNull)
			}
		}
	}
}

// writes tells the writer to write its 8 MiB of round round, waits until it
// has, and checks that the drain d reads it without growing or keeping it.
func writes(t *testing.T, loper, r string, round, d int) {
	t.Helper()
	before := drainsOf(t, loper)[d]
	writeConfig(t, fmt.Sprintf("%s.go%d", r, round), "")
	poll(t, fmt.Sprintf("the target to write 8 MiB in round %d", round), func() bool {
		_, err := os.Stat(fmt.Sprintf("%s.wrote%d", r, round))
		return err == nil
	})
	after, ok := drainsOf(t, loper)[d]
	if !ok {
		t.Fatalf("the drain %d is gone while the target still writes", d)
	}
	checkDrainFiles(t, d)
	if after-before > 1024 {
		t.Errorf("the drain grew from %d KiB to %d KiB while the target wrote 8 MiB", before, after)
	}
	if p := readPid(r + ".pid"); syscall.Kill(p, 0) != nil {
		t.Errorf("the target %d is gone after writing, want it left running", p)
	}
}

// ends tells the writer to end and waits for the drain d to end with it and
// be collected, not left a zombie, then checks that nothing was left in the
// temporary folder.
func ends(t *testing.T, home, r string, d int) {
	t.Helper()
	writeConfig(t, r+".done", "")
	poll(t, "the drain to end with the last writer and be collected", func() bool {
		stat, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(d)).Output()
		return len(stat) == 0
	})
	if names, err := os.ReadDir(filepath.Join(home, "tmp")); err != nil || len(names) > 0 {
		t.Errorf("the temporary folder holds %v (%v), want nothing", names, err)
	}
}

func TestTheBuiltBinaryDrainsWhatATargetWritesAfterTheWait(t *testing.T) {
	loper := buildLoper(t)

	// opened starts loper with the writer as its target, opens alpha in it
	// and waits for the open to be recorded, past the wait, and returns the
	// session, the writer's files and the drain.
	opened := func(t *testing.T) (s *session, home, r string, d int) {
		home = writerHome(t, loper)
		r = filepath.Join(home, "writer")
		config := writerConfig(t, home, r+" {path}")
		s = start(t, "alpha", loper, "--config", config)
		s.within("the list to be drawn", s.shown)
		s.press("w")
		s.eventually("the target to start", func() bool { return readPid(r+".pid") > 0 })
		d = theDrain(t, loper)
		s.eventually("the open to be recorded", func() bool {
			_, err := os.Stat(filepath.Join(home, ".local", "state", "loper", "history.json"))
			return err == nil
		})
		checkWriter(t, r)
		return s, home, r, d
	}

	t.Run("while loper runs", func(t *testing.T) {
		s, home, r, d := opened(t)
		writes(t, loper, r, 1, d)
		writes(t, loper, r, 2, d)
		ends(t, home, r, d) // loper still runs, so it is loper that collects the drain
		s.quit()
	})

	t.Run("after loper quits", func(t *testing.T) {
		s, home, r, d := opened(t)
		s.quit()
		writes(t, loper, r, 1, d)
		writes(t, loper, r, 2, d)
		ends(t, home, r, d)
	})

	t.Run("after the drain is asked to stop as killall loper would", func(t *testing.T) {
		s, home, r, d := opened(t)
		s.quit()
		for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP} {
			if err := syscall.Kill(d, sig); err != nil {
				t.Fatalf("%v: %v", sig, err)
			}
		}
		writes(t, loper, r, 1, d)
		ends(t, home, r, d)
	})

	t.Run("after loper hands over to an in-place target", func(t *testing.T) {
		home := writerHome(t, loper)
		r := filepath.Join(home, "writer")
		here := filepath.Join(home, "here")
		writeConfig(t, here, "#!/bin/sh\nfor i in $(seq 600); do [ -e '"+r+".done' ] && exit 0; sleep 0.05; done\n")
		if err := os.Chmod(here, 0o755); err != nil {
			t.Fatal(err)
		}
		config := filepath.Join(home, "try.toml")
		writeConfig(t, config, fmt.Sprintf("paths = [%q]\n\n[[targets]]\nname = \"Writer\"\nkey = \"w\"\ncommand = %q\n\n"+
			"[[targets]]\nname = \"Here\"\nkey = \"h\"\ncommand = %q\nin_place = true\n",
			filepath.Join(home, "code"), r+" {path}", here+" {path}"))

		s := start(t, "alpha", loper, "--config", config)
		s.within("the list to be drawn", s.shown)
		s.press("w")
		s.eventually("the target to start", func() bool { return readPid(r+".pid") > 0 })
		d := theDrain(t, loper)
		// Until the open is done, the in-place target's key does nothing.
		s.eventually("the open to be recorded", func() bool {
			_, err := os.Stat(filepath.Join(home, ".local", "state", "loper", "history.json"))
			return err == nil
		})
		s.press("h")
		s.eventually("the in-place target to take loper's place", func() bool {
			out, _ := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(s.pid)).Output()
			return strings.Contains(string(out), here)
		})
		checkWriter(t, r)
		writes(t, loper, r, 1, d)
		ends(t, home, r, d)
		s.within("the in-place target to end", s.exited)
	})

	t.Run("after loper is killed during the wait", func(t *testing.T) {
		home := writerHome(t, loper)
		r := filepath.Join(home, "writer")
		config := writerConfig(t, home, r+" {path}")

		s := start(t, "alpha", loper, "--config", config)
		s.within("the list to be drawn", s.shown)
		s.press("w")
		s.eventually("the target to start", func() bool { return readPid(r+".pid") > 0 })
		d := theDrain(t, loper)
		if err := syscall.Kill(s.pid, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		s.within("loper to die", s.exited)
		if _, err := os.Stat(filepath.Join(home, ".local", "state", "loper", "history.json")); !os.IsNotExist(err) {
			t.Fatalf("the open was recorded (%v): loper was killed after its wait, not during it", err)
		}
		checkWriter(t, r)
		writes(t, loper, r, 1, d)
		ends(t, home, r, d)
	})

	t.Run("from what a failed launcher started, after loper quits", func(t *testing.T) {
		home := writerHome(t, loper)
		r := filepath.Join(home, "writer")
		launcher := filepath.Join(home, "launcher")
		writeConfig(t, launcher, "#!/bin/sh\n'"+r+"' &\necho launcher failed\nexit 3\n")
		if err := os.Chmod(launcher, 0o755); err != nil {
			t.Fatal(err)
		}
		config := writerConfig(t, home, launcher+" {path}")

		s := start(t, "alpha", loper, "--config", config)
		s.within("the list to be drawn", s.shown)
		s.press("w")
		s.eventually("the failure to be shown", func() bool { return s.shows("exit status 3") })
		if !s.shows("launcher failed") {
			t.Error("the error dialog does not show the launcher's output")
		}
		d := theDrain(t, loper)
		s.press("\x03")
		s.within("loper to quit", s.exited)
		s.within("the terminal to close", s.drained)
		checkWriter(t, r)
		writes(t, loper, r, 1, d)
		ends(t, home, r, d)
	})
}

func TestTheHiddenDrainModeIsNotInTheHelpAndRunByHandOnlySaysSo(t *testing.T) {
	loper := buildLoper(t)
	home := tempHome(t)

	help, err := exec.Command(loper, "--help").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(help), "drain") {
		t.Errorf("--help mentions the drain:\n%s", help)
	}

	// Once with stdin on the null device, once on a file, whose state it
	// must leave as it was.
	input := filepath.Join(t.TempDir(), "input")
	writeConfig(t, input, "typed\n")
	file, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	flags := func() uintptr {
		v, _, errno := syscall.Syscall(syscall.SYS_FCNTL, file.Fd(), syscall.F_GETFL, 0)
		if errno != 0 {
			t.Fatal(errno)
		}
		return v
	}
	before := flags()
	for _, stdin := range []*os.File{nil, file} {
		cmd := exec.Command(loper, drainArg)
		var stdout, stderr strings.Builder
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, &stderr
		err = cmd.Run()

		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			t.Errorf("run by hand it exited with %v, want 2", err)
		}
		if got := stderr.String(); got != "loper: "+drainArg+": only loper itself starts this\n" || stdout.Len() > 0 {
			t.Errorf("run by hand it printed %q and %q, want one short line on stderr", stdout.String(), got)
		}
	}
	if after := flags(); after != before {
		t.Errorf("run by hand it changed its stdin's flags from %#x to %#x", before, after)
	}
	if names, err := os.ReadDir(home); err != nil || len(names) > 0 {
		t.Errorf("run by hand it left %v (%v) in the home, want nothing: no config, cache or state", names, err)
	}
}

func TestTheDrainReadsFromWhenLoperLetsGoUntilTheLastWriterCloses(t *testing.T) {
	loper := buildLoper(t)
	home := tempHome(t)
	out, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	hold, letGo, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(loper, drainArg)
	cmd.Stdin, cmd.ExtraFiles = out, []*os.File{hold}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var waitErr error
	ended := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(ended)
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		<-ended
	})
	out.Close()
	hold.Close()

	// Until loper lets go the drain reads nothing, so the pipe fills up.
	if err := w.SetWriteDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, err := io.Copy(w, io.LimitReader(zeros{}, 8<<20)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("wrote %d bytes (%v) before loper let go, want the pipe to fill up", n, err)
	}
	letGo.Close()
	if err := w.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := io.Copy(w, io.LimitReader(zeros{}, 8<<20)); err != nil {
		t.Fatalf("wrote %d bytes (%v) after loper let go, want 8 MiB", n, err)
	}
	w.Close()

	select {
	case <-ended:
		if waitErr != nil {
			t.Errorf("the drain exited with %v, want 0", waitErr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the drain still runs after the last writer closed")
	}
	if names, err := os.ReadDir(home); err != nil || len(names) > 0 {
		t.Errorf("the drain left %v (%v) in the home, want nothing", names, err)
	}
}

// zeros reads as an endless run of zero bytes.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestAnOpenFailsSayingToRestartLoperWhenItsProgramIsGone(t *testing.T) {
	built := buildLoper(t)
	home := writerHome(t, built)
	// A copy, run and then removed, as when loper is reinstalled or moved
	// while it runs.
	loper := filepath.Join(home, "bin", "loper")
	b, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, loper, string(b))
	if err := os.Chmod(loper, 0o755); err != nil {
		t.Fatal(err)
	}
	r := filepath.Join(home, "writer")
	config := writerConfig(t, home, r+" {path}")

	s := start(t, "alpha", loper, "--config", config)
	s.within("the list to be drawn", s.shown)
	if err := os.Remove(loper); err != nil {
		t.Fatal(err)
	}
	s.press("w")
	s.eventually("the failure to be shown", func() bool { return s.shows("no such file or directory; loper's own program is gone, restart loper") })
	s.quit()

	if _, err := os.Stat(r + ".pid"); !os.IsNotExist(err) {
		t.Errorf("the target ran (%v), want it never started without a drain", err)
	}
}
