package main

import (
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
