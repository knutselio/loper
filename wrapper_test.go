package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
// it must not use.
const aliases = "shopt -s expand_aliases 2>/dev/null\nalias cat=echo rm=echo\n"

func TestTheReadmesWrapperChangesTheShellsFolderAndKeepsLopersExitCode(t *testing.T) {
	wrapper := readmeWrapper(t)
	for _, sh := range []struct {
		name string
		args []string
	}{
		{"zsh", []string{"-f", "-c"}},
		{"/bin/bash", []string{"--noprofile", "--norc", "-c"}},
	} {
		t.Run(filepath.Base(sh.name), func(t *testing.T) {
			program, err := exec.LookPath(sh.name)
			if err != nil {
				t.Skipf("%s is not installed", sh.name)
			}
			for _, folder := range []string{"a repo", ""} {
				root := t.TempDir()
				bin, from, to := filepath.Join(root, "bin"), filepath.Join(root, "from"), filepath.Join(root, "a repo")
				for _, dir := range []string{bin, from, to} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if folder != "" {
					folder = to
				}
				// The fake loper records its arguments and the wrapper's file,
				// writes the folder, if any, to that file and exits 3.
				args, named := filepath.Join(root, "args"), filepath.Join(root, "file")
				writeConfig(t, filepath.Join(bin, "loper"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+args+"'\nprintf '%s' \"$LOPER_CD_FILE\" > '"+named+"'\n"+
					"printf '%s' \"$FAKE_FOLDER\" > \"$LOPER_CD_FILE\"\nexit 3\n")
				if err := os.Chmod(filepath.Join(bin, "loper"), 0o755); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(program, append(sh.args, aliases+wrapper+"\ncd \"$FROM\" || exit 9\nloper --config 'a b'\ncode=$?\nprintf '%s\\n%s\\n%s\\n' \"$PWD\" \"$OLDPWD\" \"$code\"\n")...)
				// Started in root, so that $OLDPWD is root until a cd after the
				// one to from, even one to "".
				cmd.Dir = root
				cmd.Env = []string{"PWD=" + root, "HOME=" + root, "PATH=" + bin + ":/usr/bin:/bin", "FROM=" + from, "FAKE_FOLDER=" + folder}
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("folder %q: %v\n%s", folder, err, out)
				}

				want := from + "\n" + root + "\n3\n"
				if folder != "" {
					want = folder + "\n" + from + "\n3\n"
				}
				if string(out) != want {
					t.Errorf("folder %q: the shell printed %q, want its folder, the one before and loper's exit code, %q", folder, out, want)
				}
				if b, _ := os.ReadFile(args); string(b) != "--config\na b\n" {
					t.Errorf("folder %q: loper got the arguments %q, want --config and a b", folder, b)
				}
				b, _ := os.ReadFile(named)
				if _, err := os.Stat(string(b)); len(b) == 0 || !os.IsNotExist(err) {
					t.Errorf("folder %q: the wrapper's file %q is still there (%v)", folder, b, err)
				}
			}
		})
	}
}
