// Package target runs the command that opens a repo somewhere.
package target

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Target is a place a repo can be opened in.
type Target struct {
	Name string
	Key  string   // the key that opens the selected repo here, or ""
	Args []string // the command split into words, with {path} and {name} placeholders
	// Command is the command as written in the config file, which the edit
	// dialog shows and by which the target is found in the file again.
	Command string
	Close   *bool // whether the tool quits after a successful open here, or nil to follow close_on_open
	InPlace bool  // the command takes the tool's place in its terminal
}

// Change is a change to the targets of the config file: a target added, or
// one changed or removed.
type Change struct {
	Old *Target // the target to change or remove, as loaded; nil adds one
	// Lines are Old's lines in the file as the confirmation showed them, or ""
	// before it has; the change is refused when the file's lines differ.
	Lines              string
	Remove             bool   // remove Old rather than change it
	Name, Key, Command string // the target to add, or Old's new values
	Close              *bool  // as Target's
	InPlace            bool   // as Target's
}

// HandOver is a command that takes the tool's place in its terminal.
type HandOver struct {
	Program string   // the program to run: looked up, or for a cd command the shell
	Args    []string // the command's words, its placeholders substituted
	Dir     string   // the repo, to run it in
	// Cd is set for a cd command: Dir is its folder, and Program the shell
	// to start there when no shell wrapper changes to it instead.
	Cd bool
}

// Failure is a target that did not open.
type Failure struct {
	Command string // the command line that ran, quoted as a shell would need it
	Output  string // what the command printed, then why it failed
}

func (f *Failure) Error() string {
	return f.Command + ": " + f.Output
}

// Run opens the repo at path in t, with the repo as the working folder and
// the tool's own environment. {path} in an argument stands for path and
// {name} for the repo's name; no shell is involved.
//
// Run waits for the command for at most wait. A command that exits 0 within
// it, or is still running when it ends, has opened the repo; one still
// running is left to run on its own and never stopped. Otherwise Run returns
// a *Failure.
//
// The command's stdout and stderr are one pipe. During the wait Run reads it,
// keeping the end for a failure to show. Whatever is written to it after
// that, also by what the command started and after the tool has gone, is
// read and thrown away by the drain: a process of its own that Run starts
// with the command drain before the command, as Drain describes. It holds the
// pipe until the last writer closes it, so a writer never gets SIGPIPE, and
// reads from when Run lets go of the pipe or the tool dies, so a writer never
// waits on a full pipe for longer than the drain takes to wake.
func Run(t Target, path string, wait time.Duration, drain []string) error {
	words := substitute(t, path)
	fail := func(why string, output []byte) error {
		if len(output) > 0 && output[len(output)-1] != '\n' {
			output = append(output, '\n')
		}
		return &Failure{Command: commandLine(words), Output: string(output) + why}
	}
	out, w, err := os.Pipe()
	if err != nil {
		return fail(err.Error(), nil)
	}
	// The drain reads out from when hold ends: when letGo is closed, here
	// or by the system as the tool dies.
	hold, letGo, err := os.Pipe()
	if err != nil {
		out.Close()
		w.Close()
		return fail(err.Error(), nil)
	}
	defer func() {
		letGo.Close() // first, so that there is always a reader
		out.Close()
	}()
	err = startDrain(drain, out, hold)
	hold.Close()
	if err != nil {
		w.Close()
		// The drain is the tool's own program, so it fails to start when
		// that has been moved or removed since the tool started.
		var path *fs.PathError
		if errors.As(err, &path) {
			err = fmt.Errorf("%w: %s", path.Err, path.Path)
		}
		return fail("loper's own program is gone, restart loper ("+err.Error()+")", nil)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = path
	cmd.Stdout, cmd.Stderr = w, w // stdin stays nil: the null device, never the tool's terminal
	// A session of its own detaches the command from the tool's terminal, so
	// it gets none of the terminal's signals and lives on when the tool quits.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = cmd.Start()
	w.Close() // the command's own copies are the only writers
	if err != nil {
		return fail(err.Error(), nil)
	}
	var output tail
	read := make(chan struct{})
	go func() {
		defer close(read)
		buf := make([]byte, 32<<10)
		for {
			n, err := out.Read(buf)
			output.add(buf[:n])
			if err != nil { // the end, or Run has stopped reading
				return
			}
		}
	}()
	done := make(chan error, 1)
	// The goroutine outlives a command that is still running after the wait,
	// so that its exit is collected and it never lingers as a zombie.
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		// What the command started may keep the pipe open, so its end may
		// never come: stop reading, then take what the command left in it.
		out.SetReadDeadline(time.Now())
		<-read
		out.SetReadDeadline(time.Time{})
		output.sweep(out)
		return fail(err.Error(), output.end())
	case <-time.After(wait):
		return nil
	}
}

// startDrain starts drain with out as its stdin and hold as its descriptor 3,
// in the root folder and a session of its own, away from the tool's terminal
// and its signals.
func startDrain(drain []string, out, hold *os.File) error {
	// The drain gets a copy of out rather than out itself: handing out to a
	// command would make the tool treat it as blocking, and then it could
	// not stop reading out when it wants to. The copy shares out's
	// non-blocking state, which starting the drain leaves as it is.
	raw, err := out.SyscallConn()
	if err != nil {
		return err
	}
	var fd int
	var dupErr error
	syscall.ForkLock.RLock() // so that no other command gets the copy
	err = raw.Control(func(f uintptr) {
		if fd, dupErr = syscall.Dup(int(f)); dupErr == nil {
			syscall.CloseOnExec(fd)
		}
	})
	syscall.ForkLock.RUnlock()
	if err = cmp.Or(err, dupErr); err != nil {
		return err
	}
	copied := os.NewFile(uintptr(fd), "output")
	defer copied.Close()
	cmd := exec.Command(drain[0], drain[1:]...)
	cmd.Stdin = copied
	cmd.ExtraFiles = []*os.File{hold}
	cmd.Dir = "/" // so that it keeps no folder busy
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // collects its exit while the tool runs
	return nil
}

// Drain is the drain Run starts: it waits for the end of descriptor 3, then
// reads its stdin to the end, throwing it away. Its stdin is a target's
// output, and descriptor 3 ends when Run lets go of it or the tool dies. It
// returns an error at once when either is not a pipe, as when a person runs
// it, and never changes them before that.
func Drain() error {
	for _, fd := range []int{0, 3} {
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
			return errors.New("only loper itself starts this")
		}
	}
	if _, err := io.Copy(io.Discard, os.NewFile(3, "hold")); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, os.NewFile(0, "output"))
	return err
}

// Prepare returns t's command for the repo at path, to run in the tool's
// place, or a *Failure when its program cannot be found. A command whose
// program is cd is never run: it hands over to a new shell, $SHELL or
// /bin/sh, in its one folder, or a *Failure when that is not a folder.
func Prepare(t Target, path string) (HandOver, error) {
	words := substitute(t, path)
	if t.Args[0] == "cd" { // as written, so a repo named cd never makes {name} one
		info, err := os.Stat(words[1])
		if err == nil && !info.IsDir() {
			err = errors.New(words[1] + " is not a folder")
		}
		if err != nil {
			return HandOver{}, &Failure{Command: commandLine(words), Output: err.Error()}
		}
		shell := cmp.Or(os.Getenv("SHELL"), "/bin/sh")
		return HandOver{Program: shell, Args: []string{shell}, Dir: words[1], Cd: true}, nil
	}
	program := words[0]
	if strings.Contains(program, "/") && !filepath.IsAbs(program) { // as Run finds it: in the repo
		program = filepath.Join(path, program)
	}
	program, err := exec.LookPath(program)
	if err != nil {
		return HandOver{}, &Failure{Command: commandLine(words), Output: err.Error()}
	}
	return HandOver{Program: program, Args: words, Dir: path}, nil
}

// substitute returns t's words with {path} standing for path and {name} for
// the repo's name.
func substitute(t Target, path string) []string {
	r := strings.NewReplacer("{path}", path, "{name}", filepath.Base(path))
	words := make([]string, len(t.Args))
	for i, a := range t.Args {
		words[i] = r.Replace(a)
	}
	return words
}

// keep is how much of a failed command's output is kept: the end, which
// is where a program usually says what went wrong.
const keep = 64 << 10

// tail is the end of a command's output: never more than twice keep bytes.
type tail struct {
	b   []byte
	cut bool // whether output before b was dropped
}

func (t *tail) add(p []byte) {
	t.b = append(t.b, p...)
	if len(t.b) > 2*keep {
		t.b = append(t.b[:0], t.b[len(t.b)-keep:]...)
		t.cut = true
	}
}

// sweep adds what is in out now, without waiting for more, and at most keep
// bytes of it, as a program the command left running may write on and on.
func (t *tail) sweep(out *os.File) {
	raw, err := out.SyscallConn()
	if err != nil {
		return
	}
	buf := make([]byte, 32<<10)
	raw.Read(func(fd uintptr) bool {
		for swept := 0; swept < keep; {
			n, err := syscall.Read(int(fd), buf)
			if n <= 0 || err != nil { // empty for now, or the end
				break
			}
			t.add(buf[:n])
			swept += n
		}
		return true
	})
}

// end returns the last keep bytes, from the first whole line in them.
func (t *tail) end() []byte {
	output := t.b
	if len(output) > keep {
		output = output[len(output)-keep:]
	}
	if len(output) < len(t.b) || t.cut {
		if _, rest, ok := bytes.Cut(output, []byte("\n")); ok {
			output = rest
		}
	}
	return output
}

// safeWord is a word a shell reads as itself without quotes.
var safeWord = regexp.MustCompile(`^[\w@%+=:,./-]+$`)

// commandLine joins words into a line a shell would split back into them.
func commandLine(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = w
		if !safeWord.MatchString(w) {
			quoted[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}
