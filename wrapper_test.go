package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// readmeWrapper returns the shell function in the README's "Change folder"
// section.
func readmeWrapper(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(b), "\n## Change folder\n")
	if !ok {
		t.Fatal(`README.md has no "Change folder" section`)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	_, code, ok := strings.Cut(section, "```sh\n")
	code, _, closed := strings.Cut(code, "```")
	if !ok || !closed || !strings.Contains(code, "loper() {") {
		t.Fatalf("the Change folder section has no sh block with the function loper:\n%s", section)
	}
	return code
}

// aliases are aliases a user may have when the function is defined, which
// it must not use. A shell expands them when it parses the function, so a
// script defines them before it evals the function, also in zsh, which
// parses all of a -c script before running any of it.
const aliases = "shopt -s expand_aliases 2>/dev/null\nalias cat=echo rm=echo printf=false\n"

func TestTheReadmesWrapperChangesTheShellsFolderAndReturnsTheRightCode(t *testing.T) {
	wrapper := readmeWrapper(t)
	// Each case names the folder loper hands over, relative to the test's
	// root, or "" for none, and loper's exit code. Every case also has the
	// folders "a repo" and "a repo\n" in the root.
	for _, c := range []struct {
		name   string
		folder string
		code   int
	}{
		{"hand-over", "a repo", 3},
		{"no hand-over, loper failed", "", 3},
		{"no hand-over, loper succeeded", "", 0},
		{"a folder ending in a newline", "a repo\n", 0},
		{"a folder ending in newlines", "a repo\n\n", 0},
		{"a folder with dashes, glob characters and spaces", "-a [b]* ?", 0},
		{"a folder that is gone", "gone", 0},
		{"a folder that is gone, loper failed", "gone", 3},
	} {
		for _, sh := range []struct {
			name string
			args []string
		}{
			{"zsh", []string{"-f", "-c"}},
			{"/bin/bash", []string{"--noprofile", "--norc", "-c"}},
		} {
			t.Run(c.name+"/"+filepath.Base(sh.name), func(t *testing.T) {
				program, err := exec.LookPath(sh.name)
				if err != nil {
					t.Skipf("%s is not installed", sh.name)
				}
				root := t.TempDir()
				bin, from := filepath.Join(root, "bin"), filepath.Join(root, "from")
				for _, dir := range []string{bin, from, filepath.Join(root, "a repo"), filepath.Join(root, "a repo\n"), filepath.Join(root, "a repo\n\n"), filepath.Join(root, "-a [b]* ?")} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				folder := ""
				if c.folder != "" {
					folder = filepath.Join(root, c.folder)
				}
				// The fake loper records its arguments and the wrapper's file,
				// writes the folder, if any, to that file and exits with the
				// case's code.
				args, named := filepath.Join(root, "args"), filepath.Join(root, "file")
				writeConfig(t, filepath.Join(bin, "loper"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+args+"'\nprintf '%s' \"$LOPER_CD_FILE\" > '"+named+"'\n"+
					"printf '%s' \"$FAKE_FOLDER\" > \"$LOPER_CD_FILE\"\nexit "+strconv.Itoa(c.code)+"\n")
				if err := os.Chmod(filepath.Join(bin, "loper"), 0o755); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(program, append(sh.args, aliases+"eval \"$WRAPPER\"\ncd \"$FROM\" || exit 9\nloper --config 'a b'\ncode=$?\ncommand printf '%s\\n%s\\n%s\\n' \"$PWD\" \"$OLDPWD\" \"$code\"\n")...)
				// Started in root, so that $OLDPWD is root until a cd after the
				// one to from, even one to "".
				cmd.Dir = root
				cmd.Env = []string{"PWD=" + root, "HOME=" + root, "ZDOTDIR=" + root, "PATH=" + bin + ":/usr/bin:/bin", "FROM=" + from, "FAKE_FOLDER=" + folder, "WRAPPER=" + wrapper}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("%v\n%s%s", err, out, stderr.Bytes())
				}

				want := from + "\n" + root + "\n" + strconv.Itoa(c.code) + "\n"
				switch {
				case c.folder == "gone":
					// The shell stays where it was and the function fails,
					// whatever loper's code. bash and zsh return 1 from a
					// failed cd.
					want = from + "\n" + root + "\n1\n"
				case folder != "":
					want = folder + "\n" + from + "\n" + strconv.Itoa(c.code) + "\n"
				}
				if string(out) != want {
					t.Errorf("the shell printed %q, want its folder, the one before and the function's exit code, %q\n%s", out, want, stderr.Bytes())
				}
				if b, _ := os.ReadFile(args); string(b) != "--config\na b\n" {
					t.Errorf("loper got the arguments %q, want --config and a b", b)
				}
				b, _ := os.ReadFile(named)
				if _, err := os.Stat(string(b)); len(b) == 0 || !os.IsNotExist(err) {
					t.Errorf("the wrapper's file %q is still there (%v)", b, err)
				}
			})
		}
	}
}

// TestTheReadmesWrapperWorksPastedAtAZshPrompt feeds the function to an
// interactive zsh, where a # does not start a comment by default.
func TestTheReadmesWrapperWorksPastedAtAZshPrompt(t *testing.T) {
	program, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	root := t.TempDir()
	bin, from, folder := filepath.Join(root, "bin"), filepath.Join(root, "from"), filepath.Join(root, "a repo\n")
	for _, dir := range []string{bin, from, folder} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(t, filepath.Join(bin, "loper"), "#!/bin/sh\nprintf '%s' \"$FAKE_FOLDER\" > \"$LOPER_CD_FILE\"\n")
	if err := os.Chmod(filepath.Join(bin, "loper"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	cmd := exec.Command(program, "-f", "-i")
	cmd.Stdin = strings.NewReader(readmeWrapper(t) + "cd \"$FROM\"\nloper\nprintf '%s|%s' \"$PWD\" $? > \"$OUT\"\nexit\n")
	cmd.Dir = root
	cmd.Env = []string{"PWD=" + root, "HOME=" + root, "ZDOTDIR=" + root, "PATH=" + bin + ":/usr/bin:/bin", "FROM=" + from, "FAKE_FOLDER=" + folder, "OUT=" + out}
	shown, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, shown)
	}
	if b, _ := os.ReadFile(out); string(b) != folder+"|0" {
		t.Errorf("the shell printed %q, want its folder and the function's exit code, %q\n%s", b, folder+"|0", shown)
	}
}
