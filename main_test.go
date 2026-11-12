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
	"github.com/knutselio/loper/internal/ui"
)

// startup runs the entry point with args in a temporary home and returns the
// exit code and what it wrote to stderr. Every case here must fail before the
// UI starts.
func startup(t *testing.T, args ...string) (int, string) {
	t.Helper()
	code, _, stderr := withoutUI(t, args...)
	return code, stderr
}

// withoutUI runs the entry point with args and returns the exit code and what
// it wrote to stdout and stderr, failing the test if the UI starts.
func withoutUI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs strings.Builder
	code = run(args, &out, &errs, func(string, config.Settings) error {
		t.Fatal("the UI started")
		return nil
	})
	return code, out.String(), errs.String()
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
	// A cd target hands over to $SHELL without the wrapper's file: never to
	// a real shell, and never to the developer's own file.
	t.Setenv("SHELL", filepath.Join(home, "no-shell"))
	t.Setenv("LOPER_CD_FILE", "")
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

	code := run(nil, &strings.Builder{}, &stderr, func(_ string, s config.Settings) error {
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
	for _, key := range []string{"j", "k", "g", "G", "/", "s", "r", "a", "?", "q", "e", "d"} {
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

func TestATargetWithoutANameStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("key = \"t\"\ncommand = \"tab {path}\"\n"),
		6, `a target needs a name`)
	for _, value := range []string{`""`, `"   "`} {
		startupFails(t, targetConfig("name = "+value+"\nkey = \"t\"\ncommand = \"tab {path}\"\n"),
			7, `a target needs a name`)
	}
}

func TestATargetNameWithAControlCharacterStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for toml, quoted := range map[string]string{`a\nb`: `"a\nb"`, `\u001b[31mred`: `"\x1b[31mred"`, `Rex\ttab`: `"Rex\ttab"`} {
		startupFails(t, targetConfig("name = \""+toml+"\"\nkey = \"t\"\ncommand = \"tab {path}\"\n"),
			7, `target name `+quoted+` has a control character`)
	}
}

func TestTheHelpFlagShowsWhyATargetNameIsRefused(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	writeConfig(t, file, targetConfig("name = \"\\u001b[31mred\"\ncommand = \"tab {path}\"\n"))

	code, stdout, stderr := withoutUI(t, "--help")

	if code != 0 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	if strings.Contains(stdout, "\x1b") {
		t.Errorf("stdout has a raw escape:\n%s", stdout)
	}
	want := []string{"  " + file + `:7: target name "\x1b[31mred" has a control character`}
	if got := targetsSection(t, stdout); !slices.Equal(got, want) {
		t.Errorf("Targets section %q, want %q", got, want)
	}
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

func TestATargetCloseThatIsNotTrueOrFalseStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Tab\"\ncommand = \"tab {path}\"\nclose = \"yes\"\n"),
		9, `invalid value for "targets.close": want true or false`)
}

func TestATargetInPlaceThatIsNotTrueOrFalseStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Lazygit\"\ncommand = \"lazygit -p {path}\"\nin_place = \"yes\"\n"),
		9, `invalid value for "targets.in_place": want true or false`)
}

func TestATargetInPlaceThatStaysOpenStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Lazygit\"\ncommand = \"lazygit -p {path}\"\nclose = false\nin_place = true\n"),
		6, `target "Lazygit" has in_place = true, which always closes loper, and close = false`)
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
		code := run(c.args, &strings.Builder{}, &strings.Builder{}, func(_ string, s config.Settings) error {
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

		code := run(nil, &strings.Builder{}, &strings.Builder{}, func(_ string, s config.Settings) error {
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

		code := run(nil, &strings.Builder{}, &strings.Builder{}, func(_ string, s config.Settings) error {
			got = s
			return nil
		})

		if code != 0 || !slices.Equal(got.Columns, c.columns) || got.RowStyle != c.rowStyle {
			t.Errorf("%q: exit code %d, UI told %q and %q, want 0, %q and %q", c.body, code, got.Columns, got.RowStyle, c.columns, c.rowStyle)
		}
	}
}

func TestTheVersionFlagPrintsTheVersion(t *testing.T) {
	home := tempHome(t)
	for _, args := range [][]string{{"--version"}, {"-v"}, {"--config", filepath.Join(home, "missing.toml"), "-v"}} {
		code, stdout, stderr := withoutUI(t, args...)

		if code != 0 || stderr != "" {
			t.Errorf("%q: exit code %d, stderr %q; want 0 and nothing", args, code, stderr)
		}
		// The Go toolchain stamps a test binary with the version (devel).
		if want := "loper (devel)\n"; stdout != want {
			t.Errorf("%q: stdout %q, want %q", args, stdout, want)
		}
	}
}

func TestTheHelpFlagPrintsTheUsageTheConfigFileAndTheKeys(t *testing.T) {
	home := tempHome(t)
	xdgConfig := filepath.Join(home, "elsewhere")
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	code, stdout, stderr := withoutUI(t, "--help")

	if code != 0 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	want := []string{"Usage: loper [--config <file>]", "\n  -h, --help       Print this help\n  -v, --version    Print the version\n", "Config file: " + filepath.Join(xdgConfig, "loper", "config.toml"), "\n  a  ", "Add a target to the config file",
		"\n  e  ", "Edit the highlighted target", "\n  d  ", "Remove the highlighted target", "enter, space  "}
	for _, s := range ui.KeyList() {
		want = append(want, s.Title)
		for _, b := range s.Bindings {
			want = append(want, b.Keys+"  ", b.Action)
		}
	}
	for _, w := range want {
		if !strings.Contains(stdout, w) {
			t.Errorf("stdout does not contain %q:\n%s", w, stdout)
		}
	}
	if got := targetsSection(t, stdout); !isFirstRunTarget(got) {
		t.Errorf("Targets section %q, want the first run's o cd", got)
	}
}

// noTargets is what the Targets section of --help says when there are none.
const noTargets = "none; add one in the picker with a, or in the config file"

// targetsSection returns the lines of the Targets section that ends the
// --help output in stdout, and checks that it is said only once.
func targetsSection(t *testing.T, stdout string) []string {
	t.Helper()
	before, section, ok := strings.Cut(stdout, "\nTargets:\n")
	if !ok || strings.Contains(section, "Targets") || strings.Contains(before, "Targets are") {
		t.Fatalf("stdout does not end with one Targets section:\n%s", stdout)
	}
	return strings.Split(strings.TrimSuffix(section, "\n"), "\n")
}

func TestTheHelpFlagListsTheConfiguredTargetsInOrder(t *testing.T) {
	home := tempHome(t)
	body := "[[targets]]\nname = \"Zed\"\nkey = \"z\"\ncommand = \"zed {path}\"\n\n" +
		"[[targets]]\nname = \"Rex tab\"\ncommand = \"rex window new -c {path}\"\n\n" +
		"[[targets]]\nname = \"Lazygit\"\nkey = \"l\"\ncommand = \"rex split -c {path} -- lazygit\"\n"
	writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"), body)
	other := filepath.Join(home, "other.toml")
	writeConfig(t, other, "[[targets]]\nname = \"Other\"\nkey = \"o\"\ncommand = \"other {path}\"\n")
	for _, c := range []struct {
		args  []string
		keys  []string
		names []string
	}{
		{[]string{"--help"}, []string{"z", " ", "l"}, []string{"Zed", "Rex tab", "Lazygit"}},
		{[]string{"--help", "--config", other}, []string{"o"}, []string{"Other"}},
		{[]string{"--config", other, "--help"}, []string{"o"}, []string{"Other"}},
	} {
		code, stdout, stderr := withoutUI(t, c.args...)

		if code != 0 || stderr != "" {
			t.Errorf("%q: exit code %d, stderr %q; want 0 and nothing", c.args, code, stderr)
		}
		if strings.Contains(stdout, "\x1b") {
			t.Errorf("%q: stdout has colour or other escapes:\n%s", c.args, stdout)
		}
		lines := targetsSection(t, stdout)
		if len(lines) != len(c.names) {
			t.Fatalf("%q: Targets section %q, want %q", c.args, lines, c.names)
		}
		column := strings.Index(lines[0], c.names[0])
		for i, line := range lines {
			// Two spaces, the key or a blank, then the name in one column.
			if !strings.HasPrefix(line, "  "+c.keys[i]+" ") || strings.Index(line, c.names[i]) != column ||
				strings.TrimSpace(line[3:]) != c.names[i] {
				t.Errorf("%q: Targets line %q, want key %q and %q at column %d", c.args, line, c.keys[i], c.names[i], column)
			}
		}
	}
}

func TestTheHelpFlagWinsOverTheOtherFlagsWhateverTheConfigFile(t *testing.T) {
	home := tempHome(t)
	standard := filepath.Join(home, ".config", "loper", "config.toml")
	bad := filepath.Join(home, "bad.toml")
	writeConfig(t, bad, "colour = \"blue\"\n")
	missing := filepath.Join(home, "missing.toml")
	for _, c := range []struct {
		args    []string
		file    string
		targets string
	}{
		{[]string{"-h"}, standard + notYet, firstRunTarget},
		{[]string{"--version", "--help"}, standard + notYet, firstRunTarget},
		{[]string{"--help", "--version"}, standard + notYet, firstRunTarget},
		{[]string{"-v", "--help"}, standard + notYet, firstRunTarget},
		{[]string{"-h", "-v"}, standard + notYet, firstRunTarget},
		{[]string{"--config", bad, "--help"}, bad, bad + `:1: unknown setting "colour"`},
		{[]string{"--help", "--config", missing}, missing, "config file " + missing + " does not exist"},
	} {
		code, stdout, stderr := withoutUI(t, c.args...)

		if code != 0 || stderr != "" {
			t.Errorf("%q: exit code %d, stderr %q; want 0 and nothing", c.args, code, stderr)
		}
		if want := "Config file: " + c.file + "\n"; !strings.Contains(stdout, want) {
			t.Errorf("%q: stdout does not contain %q:\n%s", c.args, want, stdout)
		}
		if got := targetsSection(t, stdout); c.targets == firstRunTarget && !isFirstRunTarget(got) ||
			c.targets != firstRunTarget && !slices.Equal(got, []string{"  " + c.targets}) {
			t.Errorf("%q: Targets section %q, want %q", c.args, got, c.targets)
		}
	}
}

// notYet follows the config file in --help when it does not exist.
const notYet = " (does not exist yet)"

// firstRunTarget stands for the Targets section of --help with only the
// first run's target, o cd.
const firstRunTarget = "o cd"

// isFirstRunTarget reports whether lines, the Targets section of --help,
// list only the first run's target: key o, name cd.
func isFirstRunTarget(lines []string) bool {
	return len(lines) == 1 && strings.HasPrefix(lines[0], "  o ") && strings.TrimSpace(lines[0][3:]) == "cd"
}

func TestTheHelpFlagPrintsEverythingAndTheLoadersMessageWhenTheStandardConfigFileIsInvalid(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	writeConfig(t, file, "paths = [\n")
	_, loadErr := config.Load("", home)

	code, stdout, stderr := withoutUI(t, "--help")

	if code != 0 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	for _, want := range []string{"Usage: loper [--config <file>]", "Config file: " + file + "\n", "Add a target to the config file"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	if loadErr == nil || !strings.HasPrefix(loadErr.Error(), file+":1: ") {
		t.Fatalf("the loader accepted the file or did not name it and the line: %v", loadErr)
	}
	if got, want := targetsSection(t, stdout), []string{"  " + loadErr.Error()}; !slices.Equal(got, want) {
		t.Errorf("Targets section %q, want %q", got, want)
	}
}

func TestAUsageErrorPrintsWhatIsWrongAndTheUsage(t *testing.T) {
	tempHome(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour"}, "loper: unknown flag --colour\n"},
		{[]string{"-colour"}, "loper: unknown flag --colour\n"},
		{[]string{"-V"}, "loper: unknown flag --V\n"},
		{[]string{"open"}, "loper: unexpected argument \"open\"\n"},
		{[]string{"--config", "a.toml", "open"}, "loper: unexpected argument \"open\"\n"},
		{[]string{"--config"}, "loper: --config needs a file\n"},
		{[]string{"--config="}, "loper: --config needs a file\n"},
		{[]string{"--config", ""}, "loper: --config needs a file\n"},
	} {
		code, stdout, stderr := withoutUI(t, c.args...)

		if code != 2 || stdout != "" {
			t.Errorf("%q: exit code %d, stdout %q; want 2 and nothing", c.args, code, stdout)
		}
		if want := c.want + "\nUsage: loper [--config <file>]\n"; !strings.HasPrefix(stderr, want) {
			t.Errorf("%q: stderr %q does not start with %q", c.args, stderr, want)
		}
	}
}

func TestTheVersionAndHelpFlagsNeedNoHomeFolderUnlessHelpNamesAFileInIt(t *testing.T) {
	home := tempHome(t)
	t.Setenv("HOME", "")
	file := filepath.Join(home, "try.toml")
	for _, c := range []struct {
		args    []string
		xdg     string
		want    string
		wantErr bool
	}{
		{args: []string{"--version"}, want: "loper (devel)\n"},
		{args: []string{"--help", "--config", file}, want: "Config file: " + file + "\n"},
		{args: []string{"--help"}, xdg: filepath.Join(home, "xdg"), want: "Config file: " + filepath.Join(home, "xdg", "loper", "config.toml") + notYet + "\n"},
		{args: []string{"--help"}, wantErr: true},
	} {
		t.Setenv("XDG_CONFIG_HOME", c.xdg)

		code, stdout, stderr := withoutUI(t, c.args...)

		if c.wantErr {
			if code != 1 || !strings.HasPrefix(stderr, "loper: ") {
				t.Errorf("%q without XDG_CONFIG_HOME: exit code %d, stderr %q; want 1 and an error", c.args, code, stderr)
			}
			continue
		}
		if code != 0 || stderr != "" || !strings.Contains(stdout, c.want) {
			t.Errorf("%q: exit code %d, stderr %q, stdout %q; want 0, nothing and %q", c.args, code, stderr, stdout, c.want)
		}
	}
}

// firstRun is the config file a first run creates.
const firstRun = "[[targets]]\nname = \"cd\"\nkey = \"o\"\ncommand = \"cd {path}\"\nin_place = true\n"

// picker runs the entry point as the picker with args and returns the exit
// code, what it wrote to stderr and the settings the UI was started with.
func picker(t *testing.T, args ...string) (int, string, config.Settings) {
	t.Helper()
	var errs strings.Builder
	var got config.Settings
	code := run(args, &strings.Builder{}, &errs, func(_ string, s config.Settings) error {
		got = s
		return nil
	})
	return code, errs.String(), got
}

func TestAFirstRunCreatesTheConfigFileWithTheCdTarget(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")

	code, stderr, s := picker(t)

	if code != 0 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != firstRun {
		t.Fatalf("config file is %q (%v), want %q", b, err, firstRun)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("config file has mode %v, want 0600", info.Mode().Perm())
	}
	if len(s.Targets) != 1 || s.Targets[0].Name != "cd" || s.Targets[0].Key != "o" || !s.Targets[0].InPlace || s.Notice != "" {
		t.Errorf("UI started with targets %+v and notice %q, want the cd target and none", s.Targets, s.Notice)
	}
}

func TestAnExistingConfigFileIsNeverChangedAtStartup(t *testing.T) {
	for _, body := range []string{"", "# mine\n", "paths = [\"~/code\"]\n", targetConfig("name = \"Tab\"\ncommand = \"tab {path}\"\n"), "paths = [\n", firstRun} {
		home := tempHome(t)
		file := filepath.Join(home, ".config", "loper", "config.toml")
		writeConfig(t, file, body)

		picker(t)
		picker(t)

		if b, _ := os.ReadFile(file); string(b) != body {
			t.Errorf("%q: config file is now %q", body, b)
		}
		if info, _ := os.Stat(file); info.Mode().Perm() != 0o644 {
			t.Errorf("%q: config file has mode %v, want 0644 as written", body, info.Mode().Perm())
		}
	}
}

func TestASecondRunLeavesTheCreatedFileAsItIs(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")
	picker(t)
	before, err := os.Stat(file)
	if err != nil {
		t.Fatalf("the first run created no config file: %v", err)
	}

	picker(t)

	after, _ := os.Stat(file)
	if b, _ := os.ReadFile(file); string(b) != firstRun || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("config file is %q, modified %v then %v; want it untouched", b, before.ModTime(), after.ModTime())
	}
}

func TestTheHelpAndVersionFlagsCreateNoConfigFile(t *testing.T) {
	home := tempHome(t)
	folder := filepath.Join(home, ".config")
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-v"}} {
		withoutUI(t, args...)

		if _, err := os.Stat(folder); !os.IsNotExist(err) {
			t.Errorf("%q: %s exists (%v); want nothing created", args, folder, err)
		}
	}
}

func TestTheHelpFlagWithNoConfigFileListsTheFirstRunsTargetAndSaysTheFileIsNotThereYet(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, ".config", "loper", "config.toml")

	code, stdout, stderr := withoutUI(t, "--help")

	if code != 0 || stderr != "" {
		t.Errorf("exit code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	if want := "Config file: " + file + notYet + "\n"; !strings.Contains(stdout, want) {
		t.Errorf("stdout does not contain %q:\n%s", want, stdout)
	}
	if lines := targetsSection(t, stdout); !isFirstRunTarget(lines) {
		t.Errorf("Targets section %q, want key o and cd", lines)
	}
}

func TestAConfigFlagNamingAMissingFileCreatesNothing(t *testing.T) {
	home := tempHome(t)
	file := filepath.Join(home, "nope.toml")

	if code, _ := startup(t, "--config", file); code == 0 {
		t.Errorf("exit code 0, want non-zero")
	}

	for _, f := range []string{file, filepath.Join(home, ".config")} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s exists (%v); want nothing created", f, err)
		}
	}
}

func TestAConfigFileThatCannotBeCreatedStartsWithTheCdTargetAndSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(t *testing.T, folder, file string)
		why   string
	}{
		{"read-only folder", func(t *testing.T, folder, _ string) {
			if err := os.MkdirAll(folder, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(folder, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(folder, 0o700) })
		}, "permission denied"},
		{"link to nothing", func(t *testing.T, folder, file string) {
			if err := os.MkdirAll(folder, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(folder, "gone.toml"), file); err != nil {
				t.Fatal(err)
			}
		}, "it is a link to "},
	} {
		home := tempHome(t)
		folder := filepath.Join(home, ".config", "loper")
		file := filepath.Join(folder, "config.toml")
		c.setup(t, folder, file)

		code, stderr, s := picker(t)

		if code != 0 || stderr != "" {
			t.Errorf("%s: exit code %d, stderr %q; want 0 and nothing", c.name, code, stderr)
		}
		if len(s.Targets) != 1 || s.Targets[0].Name != "cd" || s.Targets[0].Key != "o" || !s.Targets[0].InPlace {
			t.Errorf("%s: UI started with targets %+v, want the cd target", c.name, s.Targets)
		}
		if want := "The config file could not be written: " + c.why; !strings.HasPrefix(s.Notice, want) {
			t.Errorf("%s: notice %q, want it to start with %q", c.name, s.Notice, want)
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("%s: %s now exists (%v); want nothing written", c.name, file, err)
		}
		if _, err := os.Stat(filepath.Join(folder, "gone.toml")); !os.IsNotExist(err) {
			t.Errorf("%s: the link was written through (%v)", c.name, err)
		}
	}
}

func TestACdCommandWithoutInPlaceStopsTheToolNamingTheFileAndLine(t *testing.T) {
	startupFails(t, targetConfig("name = \"Go\"\nkey = \"c\"\ncommand = \"cd {path}\"\n"),
		6, `target "Go" runs cd, which needs in_place = true`)
}

func TestACdCommandWithoutOneFolderStopsTheToolNamingTheFileAndLine(t *testing.T) {
	for _, command := range []string{"cd", "cd {path} extra", "cd {path} {path}"} {
		startupFails(t, targetConfig("name = \"Go\"\ncommand = \""+command+"\"\nin_place = true\n"),
			8, `target command "`+command+`" needs one folder after cd`)
	}
}

func TestACommandWhoseProgramIsNotCdNeedsNoInPlace(t *testing.T) {
	home := tempHome(t)
	writeConfig(t, filepath.Join(home, ".config", "loper", "config.toml"),
		"[[targets]]\nname = \"X\"\ncommand = \"cdx {path}\"\n\n[[targets]]\nname = \"Y\"\ncommand = \"/usr/bin/cd {path} extra\"\n")

	code, stderr, s := picker(t)

	if code != 0 || len(s.Targets) != 2 {
		t.Errorf("exit code %d, stderr %q, targets %+v; want 0 and both targets", code, stderr, s.Targets)
	}
}

func TestTheHelpFlagWithAConfigFileWithoutTargetsSaysThereAreNone(t *testing.T) {
	for _, body := range []string{"", "# mine\n"} {
		home := tempHome(t)
		file := filepath.Join(home, ".config", "loper", "config.toml")
		writeConfig(t, file, body)

		code, stdout, stderr := withoutUI(t, "--help")

		if code != 0 || stderr != "" {
			t.Errorf("%q: exit code %d, stderr %q; want 0 and nothing", body, code, stderr)
		}
		if want := "Config file: " + file + "\n"; !strings.Contains(stdout, want) {
			t.Errorf("%q: stdout does not contain %q:\n%s", body, want, stdout)
		}
		if got, want := targetsSection(t, stdout), []string{"  " + noTargets}; !slices.Equal(got, want) {
			t.Errorf("%q: Targets section %q, want %q", body, got, want)
		}
	}
}
