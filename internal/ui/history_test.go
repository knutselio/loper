package ui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// savedHistory returns the history file in the state folder as last-opened times
// keyed by repo path, or nil when there is no file.
func savedHistory(t *testing.T, stateHome string) map[string]time.Time {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stateHome, "loper", "history.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var h map[string]time.Time
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("history file is not JSON: %v\n%s", err, b)
	}
	return h
}

func TestARepoNeverOpenedShowsADashForLastOpened(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")

	screen := e.start().screen()

	if !hasExactRow(screen, "alpha ~/code main -") {
		t.Errorf("screen has no row %q:\n%s", "alpha ~/code main -", screen)
	}
}

func TestASuccessfulOpenShowsJustNowAndRecordsThePathAndTimeInTheHistory(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	d := e.start()

	d.press("j", "z")

	if screen := d.screen(); !hasExactRow(screen, "beta ~/code main just now") || !hasExactRow(screen, "alpha ~/code main -") {
		t.Errorf("want beta opened just now and alpha never:\n%s", screen)
	}
	h := savedHistory(t, filepath.Join(e.home, ".local", "state"))
	if len(h) != 1 || !h[beta].Equal(e.now) {
		t.Errorf("history holds %v, want only %s at %v", h, beta, e.now)
	}
}

func TestTheHistoryIsWrittenUnderXDGStateHomeWhenItIsSet(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	state := filepath.Join(e.home, "xdg-state")
	t.Setenv("XDG_STATE_HOME", state)
	d := e.start()

	d.press("z")

	if h := savedHistory(t, state); !h[alpha].Equal(e.now) {
		t.Errorf("history under XDG_STATE_HOME holds %v, want %s", h, alpha)
	}
	if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); h != nil {
		t.Errorf("history also written under ~/.local/state: %v", h)
	}
}

func TestTheHistoryIsWrittenUnderDotLocalStateWhenXDGStateHomeIsUnsetOrRelative(t *testing.T) {
	for _, value := range []string{"", "relative/state"} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		t.Setenv("XDG_STATE_HOME", value)
		t.Chdir(e.home) // so a relative folder that is not ignored lands in the temporary home
		d := e.start()

		d.press("z")

		if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); !h[alpha].Equal(e.now) {
			t.Errorf("XDG_STATE_HOME=%q: history under ~/.local/state holds %v, want %s", value, h, alpha)
		}
		if h := savedHistory(t, filepath.Join(e.home, "relative", "state")); h != nil {
			t.Errorf("XDG_STATE_HOME=%q: history written under the relative folder: %v", value, h)
		}
	}
}

func TestAFailedOpenWritesNothingToTheHistory(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	brokenTarget(e, "exit 1\n")
	d := e.start()

	d.press("b", "esc")

	if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); h != nil {
		t.Errorf("history written after a failed open: %v", h)
	}
	if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main -") {
		t.Errorf("want alpha still never opened:\n%s", screen)
	}
}

func TestARelaunchShowsTheLastOpenedTimeAgainstTheClock(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.now = time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	e.start().press("j", "z")

	e.now = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	screen := e.start().screen()

	if !hasExactRow(screen, "beta ~/code main 2h ago") || !hasExactRow(screen, "alpha ~/code main -") {
		t.Errorf("want beta opened 2h ago and alpha never:\n%s", screen)
	}
}

func TestAnEntryForARepoNotInTheListSurvivesAnotherOpen(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	gone := filepath.Join(e.home, "code", "gone")
	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	e.writeFile(filepath.Join(e.home, ".local", "state", "loper", "history.json"), `{"`+gone+`": "`+earlier.Format(time.RFC3339)+`"}`)
	d := e.start()

	d.press("z")

	h := savedHistory(t, filepath.Join(e.home, ".local", "state"))
	if len(h) != 2 || !h[gone].Equal(earlier) || !h[alpha].Equal(e.now) {
		t.Errorf("history holds %v, want %s at %v and %s at %v", h, gone, earlier, alpha, e.now)
	}
}

func TestACorruptHistoryFileStartsEmptyAndTheNextOpenReplacesIt(t *testing.T) {
	for _, body := range []string{"not json", "null", `{"/a": "not a time"}`, `["a list"]`} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.writeFile(filepath.Join(e.home, ".local", "state", "loper", "history.json"), body)
		d := e.start()

		if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main -") {
			t.Errorf("history %q: want alpha never opened:\n%s", body, screen)
		}
		d.press("z")

		if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main just now") {
			t.Errorf("history %q: want alpha opened just now:\n%s", body, screen)
		}
		if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); len(h) != 1 || !h[alpha].Equal(e.now) {
			t.Errorf("history %q: history holds %v, want only %s", body, h, alpha)
		}
	}
}

func TestAHistoryThatCannotBeSavedIsReportedInTheFooterAndTheOpenStillCounts(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.writeFile(filepath.Join(e.home, ".local", "state", "loper"), "a file where the folder should be")
	d := e.start()

	d.press("z")

	if got, want := footer(d), "Opened alpha in Zed, but the history could not be saved"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main just now") {
		t.Errorf("want alpha opened just now for this session:\n%s", screen)
	}
}

func TestWithCloseOnOpenTheOpenIsStillRecorded(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config("close_on_open = true\n")
	d := e.start()

	d.press("z")

	// The driver runs background work in order, so this cannot catch a save
	// that races the quit; the code rules that out by saving before it
	// reports the open.
	if !d.quit {
		t.Fatal("the tool did not quit after a successful open")
	}
	if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); !h[alpha].Equal(e.now) {
		t.Errorf("history holds %v, want %s", h, alpha)
	}
}

func TestTheHistoryFolderHoldsOnlyTheHistoryFileAfterOpens(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	d := e.start()

	d.press("z", "j", "z")

	if names := stateFolder(t, e); len(names) != 1 || names[0] != "history.json" {
		t.Errorf("state folder holds %q, want only history.json", names)
	}
}

// stateFolder returns the names in loper's state folder.
func stateFolder(t *testing.T, e *env) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.home, ".local", "state", "loper"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// userImmutable is UF_IMMUTABLE from <sys/stat.h>: no one may replace or
// change the file until the flag is cleared.
const userImmutable = 0x2

func TestAHistoryFileThatCannotBeReplacedIsLeftAsItWasWithNothingBesideIt(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	body := `{"` + alpha + `": "2026-01-02T03:04:05Z"}`
	file := filepath.Join(e.home, ".local", "state", "loper", "history.json")
	e.writeFile(file, body)
	if err := syscall.Chflags(file, userImmutable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Chflags(file, 0) }) // so the temporary folder can be removed
	d := e.start()

	d.press("z")

	if got, want := footer(d), "Opened alpha in Zed, but the history could not be saved"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	if names := stateFolder(t, e); len(names) != 1 || names[0] != "history.json" {
		t.Errorf("state folder holds %q, want only history.json", names)
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != body {
		t.Errorf("history file reads %q (%v), want it unchanged: %q", b, err, body)
	}
}

func TestLastOpenedStartsOnAStraightEdgeAfterBranchesOfDifferentLengths(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.git(alpha, "checkout", "-q", "-b", "feature/long-branch-name")
	e.repo("code/beta")
	d := e.start()

	d.press("z")

	lines := strings.Split(d.screen(), "\n")
	if got, want := cellColumn(t, lines[3], "-"), cellColumn(t, lines[2], "just now"); got != want {
		t.Errorf("last opened starts at cell %d for beta and %d for alpha:\n%s", got, want, d.screen())
	}
}

func TestAnUnreadableHistoryFileIsLeftAloneAndTheOpenWarnsThatItWasNotSaved(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	gone := filepath.Join(e.home, "code", "gone")
	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	file := filepath.Join(e.home, ".local", "state", "loper", "history.json")
	e.writeFile(file, `{"`+gone+`": "`+earlier.Format(time.RFC3339)+`"}`)
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatal(err)
	}
	d := e.start()

	d.press("z")

	if got, want := footer(d), "Opened alpha in Zed, but the history could not be saved"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
	if err := os.Chmod(file, 0o644); err != nil {
		t.Fatal(err)
	}
	if h := savedHistory(t, filepath.Join(e.home, ".local", "state")); len(h) != 1 || !h[gone].Equal(earlier) {
		t.Errorf("history holds %v, want it untouched with only %s, not %s", h, gone, alpha)
	}
}

func TestTheStateFolderIsCreatedPrivateToTheUser(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()

	d.press("z")

	info, err := os.Stat(filepath.Join(e.home, ".local", "state", "loper"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("state folder has mode %v, want %v", got, os.FileMode(0o700))
	}
}

func TestAHistoryReadThatArrivesAfterAnOpenDoesNotUndoTheOpen(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.repo("code/beta")
	e.openedBefore(map[string]time.Duration{alpha: 3 * time.Hour})
	e.start()
	d := relaunch(e) // the history read's result is held back; it was read before the open

	d.press("z") // alpha, the top row
	d.releaseLastFirst()

	if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main just now") {
		t.Errorf("want alpha opened just now, not 3h ago:\n%s", screen)
	}
}
