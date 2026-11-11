package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/knutselio/loper/internal/config"
)

// startup runs the entry point with args in a temporary home and returns the
// exit code and what it wrote to stderr. Every case here must fail before the
// UI starts.
func startup(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stderr strings.Builder
	code := run(args, &stderr, func(string, config.Settings) error {
		t.Fatal("the UI started")
		return nil
	})
	return code, stderr.String()
}

// tempHome points the home and XDG folders at a temporary tree and returns
// the home folder.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return home
}

// writeConfig writes body to path, creating its folders.
func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnknownSettingStopsTheToolNamingTheFileAndLine(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	writeConfig(t, file, "# loper\npaths = [\"~/code\"]\ncolour = \"blue\"\n")

	code, stderr := startup(t)

	if code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}
	if want := file + ":3: unknown setting \"colour\""; !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain %q", stderr, want)
	}
}

func TestAWrongTypeForPathsStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, value := range []string{`"~/code"`, `[1, 2]`} {
		home := tempHome(t)
		file := filepath.Join(home, ".config", "loper", "config.toml")
		writeConfig(t, file, "# loper\n\npaths = "+value+"\n")

		code, stderr := startup(t)

		if code == 0 {
			t.Errorf("paths = %s: exit code 0, want non-zero", value)
		}
		if want := file + `:3: invalid value for "paths": want a list of folders, each absolute or starting with ~/, such as ["~/code"]`; !strings.Contains(stderr, want) {
			t.Errorf("paths = %s: stderr %q does not contain %q", value, stderr, want)
		}
	}
}

func TestATOMLSyntaxErrorStopsTheToolNamingTheFileAndLine(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	writeConfig(t, file, "# loper\n\npaths = [\"~/code\"\n")

	code, stderr := startup(t)

	if code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}
	if want := file + ":3: "; !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain %q", stderr, want)
	}
}

func TestTheConfigFlagUsesThatFileInstead(t *testing.T) {
	home := tempHome(t)
	writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), "paths = [\"~/code\"]\n")
	file := filepath.Join(home, "try.toml")
	writeConfig(t, file, "# trying\n\ncolour = \"blue\"\n")

	code, stderr := startup(t, "--config", file)

	if code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}
	if want := file + ":3: unknown setting \"colour\""; !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain %q", stderr, want)
	}
}

func TestAConfigFlagNamingAMissingFileStopsTheTool(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, "nope.toml")

	code, stderr := startup(t, "--config", file)

	if code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}
	if want := "loper: config file " + file + " does not exist\n"; stderr != want {
		t.Errorf("stderr %q, want %q", stderr, want)
	}
}

func TestAValidConfigStartsTheUIWithTheConfiguredPaths(t *testing.T) {
	home := tempHome(t)
	writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), "paths = [\"~/code\", \"/srv/repos\"]\n")
	var stderr strings.Builder
	var got []string

	code := run(nil, &stderr, func(_ string, s config.Settings) error {
		got = s.Paths
		return nil
	})

	if code != 0 {
		t.Errorf("exit code %d, want 0; stderr %q", code, stderr.String())
	}
	if want := []string{filepath.Join(home, "code"), "/srv/repos"}; !slices.Equal(got, want) {
		t.Errorf("UI started with paths %q, want %q", got, want)
	}
}

func TestARelativePathsEntryStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, entry := range []string{`"code"`, `""`, `"~user/code"`} {
		home := tempHome(t)
		file := filepath.Join(home, ".config", "loper", "config.toml")
		writeConfig(t, file, "paths = [\n  \"~/code\",\n  "+entry+",\n]\n")

		code, stderr := startup(t)

		if code == 0 {
			t.Errorf("entry %s: exit code 0, want non-zero", entry)
		}
		if want := file + `:3: invalid value for "paths": want a list of folders`; !strings.Contains(stderr, want) {
			t.Errorf("entry %s: stderr %q does not contain %q", entry, stderr, want)
		}
	}
}

// targetConfig is a config file with a good target followed by second, so
// that an error in second is on line 6 or later.
func targetConfig(second string) string {
	return "[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n\n[[targets]]\n" + second
}

// startupFails runs the entry point on the config body and checks that it
// stops with an error containing want, after the file name and line.
func startupFails(t *testing.T, body string, line int, want string) {
	t.Helper()
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	writeConfig(t, file, body)

	code, stderr := startup(t)

	if code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}
	if want := fmt.Sprintf("%s:%d: %s", file, line, want); !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain %q", stderr, want)
	}
}

func TestATargetCommandWithoutPathStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"t\"\ncommand = \"rex window new\"\n"),
		9, `target command "rex window new" has no {path}`)
}

func TestATargetCommandWithAnUnbalancedQuoteStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"t\"\ncommand = \"rex window new --title 'my tab {path}\"\n"),
		9, `target command "rex window new --title 'my tab {path}" has an unbalanced quote`)
}

func TestATargetKeyLongerThanOneCharacterStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"tt\"\ncommand = \"rex window new -c {path}\"\n"),
		8, `target key "tt" is not a single character`)
}

func TestATargetKeyThatIsOneOfTheToolsOwnKeysStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, key := range []string{"j", "k", "g", "G", "/", "s", "r", "?", "q"} {
		startupFails(t, targetConfig("name = \"Tab\"\nkey = \""+key+"\"\ncommand = \"rex window new -c {path}\"\n"),
			8, `target key "`+key+`" is one of loper's own keys`)
	}
}

func TestTwoTargetsWithTheSameKeyStopTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Zed preview\"\nkey = \"z\"\ncommand = \"zed-preview {path}\"\n"),
		8, `target key "z" is already used by the target on line 1`)
}

func TestTwoInlineTargetsWithTheSameKeyStopTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, "targets = [\n  { name = \"Zed\", key = \"z\", command = \"zed {path}\" },\n  { name = \"Zed preview\", key = \"z\", command = \"zed-preview {path}\" },\n]\n",
		3, `target key "z" is already used by the target on line 2`)
}

func TestATargetWithoutACommandStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"t\"\n"),
		6, `target "Tab" has no command; it needs one with {path}`)
}

func TestTargetsWrittenAsAPlainTableStopTheToolNamingTheFileAndLine(t *testing.T) {
	for _, body := range []string{
		"paths = [\"~/code\"]\n\n[targets]\nname = \"Tab\"\ncommand = \"tab {path}\"\n",
		"paths = [\"~/code\"]\n\ntargets.name = \"Tab\"\ntargets.command = \"tab {path}\"\n",
	} {
		startupFails(t, body, 3, `write each target as a [[targets]] table`)
	}
}

func TestATargetCommandEndingInALoneBackslashStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"t\"\ncommand = 'tab {path} \\'\n"),
		9, `target command "tab {path} \\" ends with a lone backslash`)
}

func TestATargetCommandStartingWithAnEmptyWordStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\nkey = \"t\"\ncommand = \"'' {path}\"\n"),
		9, `target command "'' {path}" has no program to run`)
}

func TestATargetKeyThatIsNotAPrintableCharacterStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for toml, quoted := range map[string]string{`\u001b`: `"\x1b"`, `\t`: `"\t"`, ` `: `" "`} {
		startupFails(t, targetConfig("name = \"Tab\"\nkey = \""+toml+"\"\ncommand = \"tab {path}\"\n"),
			8, `target key `+quoted+` is not a printable character`)
	}
}
