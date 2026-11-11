package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestACloseOnOpenThatIsNotTrueOrFalseStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, "paths = [\"~/code\"]\n\nclose_on_open = \"yes\"\n",
		3, `invalid value for "close_on_open": want true or false`)
}

func TestASortThatIsNotLastOpenedOrNameStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, value := range []string{`"size"`, `"Name"`, `1`} {
		startupFails(t, "paths = [\"~/code\"]\n\nsort = "+value+"\n",
			3, `invalid value for "sort": want "last_opened" or "name"`)
	}
}

func TestTheUIIsToldWhichConfigFileWasUsedSoItCanNameIt(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, "try.toml")
	writeConfig(t, file, "paths = [\"~/code\"]\n")
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, filepath.Join(home, ".config", "loper", "config.toml")},
		{[]string{"--config", file}, file},
	} {
		var got string
		code := run(c.args, &strings.Builder{}, func(_ string, s config.Settings) error {
			got = s.File
			return nil
		})
		if code != 0 || got != c.want {
			t.Errorf("args %q: exit code %d, UI told %q, want 0 and %q", c.args, code, got, c.want)
		}
	}
}

func TestACacheMaxAgeThatIsNotAnIntervalStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, value := range []string{`"soon"`, `"-1h"`, `""`, `"4d"`, `0`, `"4"`} {
		startupFails(t, "paths = [\"~/code\"]\n\ncache_max_age = "+value+"\n",
			3, `invalid value for "cache_max_age": want "0" or a positive interval such as "4h" or "30m" (units h, m, s)`)
	}
}

func TestACacheMaxAgeIsHandedToTheUI(t *testing.T) {
	for body, want := range map[string]time.Duration{
		"":                        0,
		`cache_max_age = "0"`:     0,
		`cache_max_age = "4h"`:    4 * time.Hour,
		`cache_max_age = "1h30m"`: 90 * time.Minute,
	} {
		home := tempHome(t)
		writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), body+"\n")
		var got time.Duration

		code := run(nil, &strings.Builder{}, func(_ string, s config.Settings) error {
			got = s.CacheMaxAge
			return nil
		})

		if code != 0 || got != want {
			t.Errorf("%q: exit code %d, UI told %v, want 0 and %v", body, code, got, want)
		}
	}
}

func TestAColumnsListWithoutNameStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, value := range []string{`["branch"]`, `[]`, "[\n  \"folder\",\n  \"branch\",\n]"} {
		startupFails(t, "paths = [\"~/code\"]\n\ncolumns = "+value+"\n",
			3, `columns must include "name"`)
	}
}

func TestAnUnknownColumnStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, "paths = [\"~/code\"]\n\ncolumns = [\n  \"name\",\n  \"size\",\n]\n",
		5, `invalid value for "columns": want a list of "name", "folder", "branch" and "last_opened", including "name"`)
	startupFails(t, "columns = \"name\"\n",
		1, `invalid value for "columns": want a list of "name", "folder", "branch" and "last_opened", including "name"`)
}

func TestAColumnListedTwiceStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, "paths = [\"~/code\"]\n\ncolumns = [\n  \"name\",\n  \"branch\",\n  \"branch\",\n]\n",
		6, `column "branch" is listed twice`)
}

func TestARowStyleThatIsNotOneOfTheThreeStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, value := range []string{`"wide"`, `"Name"`, `1`} {
		startupFails(t, "paths = [\"~/code\"]\n\nrow_style = "+value+"\n",
			3, `invalid value for "row_style": want "name-folder", "root-path" or "name"`)
	}
}

func TestColumnsAndRowStyleAreHandedToTheUIWithTheirDefaults(t *testing.T) {
	for _, c := range []struct {
		body     string
		columns  []string
		rowStyle string
	}{
		{"", []string{"name", "folder", "branch", "last_opened"}, "name-folder"},
		{"columns = [\"branch\", \"name\"]\nrow_style = \"root-path\"", []string{"branch", "name"}, "root-path"},
	} {
		home := tempHome(t)
		writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), c.body+"\n")
		var got config.Settings

		code := run(nil, &strings.Builder{}, func(_ string, s config.Settings) error {
			got = s
			return nil
		})

		if code != 0 || !slices.Equal(got.Columns, c.columns) || got.RowStyle != c.rowStyle {
			t.Errorf("%q: exit code %d, UI told %q and %q, want 0, %q and %q", c.body, code, got.Columns, got.RowStyle, c.columns, c.rowStyle)
		}
	}
}
