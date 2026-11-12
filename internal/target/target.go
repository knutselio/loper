// Package target runs the command that opens a repo somewhere.
package target

import (
	"bytes"
	"cmp"
	"errors"
	"io"
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
func Run(t Target, path string, wait time.Duration) error {
	words := substitute(t, path)
	fail := func(why string, output []byte) error {
		if len(output) > 0 && output[len(output)-1] != '\n' {
			output = append(output, '\n')
		}
		return &Failure{Command: commandLine(words), Output: string(output) + why}
	}
	// The output goes to a file rather than a pipe: a command still running
	// after the wait keeps writing to it without anyone reading, and never
	// gets SIGPIPE once the tool has gone. The file is removed straight away,
	// so it disappears when the last writer closes it; until then, a chatty
	// command left running keeps growing it.
	out, err := os.CreateTemp("", "loper-target-")
	if err != nil {
		return fail(err.Error(), nil)
	}
	defer out.Close()
	os.Remove(out.Name()) // best effort: a leftover temporary file harms nothing
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = path
	cmd.Stdout, cmd.Stderr = out, out // stdin stays nil: the null device, never the tool's terminal
	// A session of its own detaches the command from the tool's terminal, so
	// it gets none of the terminal's signals and lives on when the tool quits.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fail(err.Error(), nil)
	}
	done := make(chan error, 1)
	// The goroutine outlives a command that is still running after the wait,
	// so that its exit is collected and it never lingers as a zombie.
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		return fail(err.Error(), tail(out))
	case <-time.After(wait):
		return nil
	}
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

// tail returns the last keep bytes of out, from the first whole line in
// them. It reads without moving the file's offset, which a program the
// command left running in the background still writes at.
func tail(out *os.File) []byte {
	var start int64
	if info, err := out.Stat(); err == nil {
		start = max(info.Size()-keep, 0)
	}
	output, _ := io.ReadAll(io.NewSectionReader(out, start, keep)) // what was read is still worth showing
	if start > 0 {
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
