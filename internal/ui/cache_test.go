package ui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// savedCache is the cache file as written.
type savedCache struct {
	ScannedAt time.Time `json:"scanned_at"`
	Paths     []string  `json:"paths"`
	Repos     []struct {
		Path   string `json:"path"`
		Branch string `json:"branch"`
	} `json:"repos"`
}

// readCache returns the cache file in the cache folder, or nil when there
// is no file.
func readCache(t *testing.T, cacheHome string) *savedCache {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cacheHome, "loper", "repos.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var c savedCache
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("cache file is not JSON: %v\n%s", err, b)
	}
	return &c
}

func TestALaunchWritesTheReposTheirBranchesTheScanTimeAndThePathsToTheCache(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	beta := e.repo("projects/beta")
	e.git(beta, "checkout", "-q", "-b", "feature")
	e.config(`paths = ["~/code", "~/projects"]`)

	e.start()

	c := readCache(t, filepath.Join(e.home, ".cache"))
	if c == nil {
		t.Fatal("no cache file under XDG_CACHE_HOME")
	}
	var repos []string
	for _, r := range c.Repos {
		repos = append(repos, r.Path+" "+r.Branch)
	}
	if want := []string{alpha + " main", beta + " feature"}; !slices.Equal(repos, want) {
		t.Errorf("cache holds repos %q, want %q", repos, want)
	}
	if !c.ScannedAt.Equal(e.now) {
		t.Errorf("cache scanned at %v, want %v", c.ScannedAt, e.now)
	}
	if want := []string{filepath.Join(e.home, "code"), filepath.Join(e.home, "projects")}; !slices.Equal(c.Paths, want) {
		t.Errorf("cache scanned from %q, want %q", c.Paths, want)
	}
}

// relaunch launches the program again on the cache the last launch wrote
// and hands it the cached list. The refresh has run and rewritten the cache
// by then; only its result is held back.
func relaunch(e *env) *driver {
	e.t.Helper()
	d := e.launch()
	d.next() // the cache read
	return d
}

func TestARepoCreatedAfterTheCacheWasWrittenAppearsOnceTheRefreshFinishes(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.start()
	e.repo("code/beta")

	d := relaunch(e)

	if screen := d.screen(); !hasRow(screen, "alpha ~/code main") || hasRow(screen, "beta") {
		t.Errorf("cached list: want alpha and no beta:\n%s", screen)
	}
	d.release()
	if screen := d.screen(); !hasRow(screen, "alpha ~/code main") || !hasRow(screen, "beta ~/code main") {
		t.Errorf("refreshed list: want alpha and beta:\n%s", screen)
	}
}

func TestABranchChangedAfterTheCacheWasWrittenShowsOnceTheRefreshFinishes(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.start()
	e.git(alpha, "checkout", "-q", "-b", "feature")

	d := relaunch(e)

	if screen := d.screen(); !hasRow(screen, "alpha ~/code main") {
		t.Errorf("cached list: want alpha on main:\n%s", screen)
	}
	d.release()
	if screen := d.screen(); !hasRow(screen, "alpha ~/code feature") {
		t.Errorf("refreshed list: want alpha on feature:\n%s", screen)
	}
}

func TestARepoDeletedAfterTheCacheWasWrittenIsGoneOnceTheRefreshFinishes(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.start()
	if err := os.RemoveAll(beta); err != nil {
		t.Fatal(err)
	}

	d := relaunch(e)

	if screen := d.screen(); !hasRow(screen, "beta ~/code main") {
		t.Errorf("cached list: want beta still there:\n%s", screen)
	}
	d.release()
	if screen := d.screen(); hasRow(screen, "beta") || header(d) != "repos 1 sort: last opened" {
		t.Errorf("refreshed list: want beta gone and one repo:\n%s", screen)
	}
}

func TestTheHeaderShowsTheSpinnerAndRefreshingWhileTheRefreshRunsAndNotAfter(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.start()

	d := relaunch(e)

	if got, want := header(d), "repos 1 sort: last opened ⠋ refreshing"; got != want {
		t.Errorf("while refreshing: header reads %q, want %q", got, want)
	}
	d.tick()
	if got, want := header(d), "repos 1 sort: last opened ⠙ refreshing"; got != want {
		t.Errorf("after a tick: header reads %q, want %q", got, want)
	}
	d.release()
	lines := strings.Split(d.screen(), "\n")
	if !strings.HasSuffix(lines[0], " sort: last opened ") {
		t.Errorf("after the refresh: header reads %q, want it to end with the sort", lines[0])
	}
	d.tick() // the tick that was on its way when the refresh finished
	if d.tick() {
		t.Error("the spinner still ticks after the refresh finished")
	}
}

func TestTheHeaderWithTheSpinnerFitsTheWidthAndKeepsTheSortAndSpinnerOnTheRight(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.start()
	d := relaunch(e)

	for _, width := range []int{80, 50, 20, 5} {
		d.send(tea.WindowSizeMsg{Width: width, Height: 24})
		head := strings.SplitN(d.screen(), "\n", 2)[0]
		if w := ansi.StringWidth(head); w > width {
			t.Errorf("width %d: header is %d cells wide: %q", width, w, head)
		}
		if width >= 50 && (!strings.HasPrefix(head, " repos  1 ") || !strings.HasSuffix(head, " sort: last opened   ⠋ refreshing ")) {
			t.Errorf("width %d: header %q does not have the count on the left and the sort and spinner on the right", width, head)
		}
	}
}

func TestWithNoCacheTheScreenShowsScanningUntilTheRefreshFinishes(t *testing.T) {
	for _, body := range []string{"", "not json", "null", `{"repos": "not a list"}`} { // "" is no file
		e := newEnv(t)
		e.repo("code/alpha")
		if body != "" {
			e.writeFile(filepath.Join(e.home, ".cache", "loper", "repos.json"), body)
		}

		d := relaunch(e)

		if screen := d.screen(); !strings.Contains(screen, "\n   ⠋ scanning…") || hasRow(screen, "alpha") {
			t.Errorf("cache %q: before the refresh's result: want the spinner and scanning… and no repos:\n%s", body, screen)
		}
		if got, want := header(d), "repos 0 sort: last opened"; got != want {
			t.Errorf("cache %q: before the refresh: header reads %q, want %q, the spinner being in the list", body, got, want)
		}
		d.release()
		if screen := d.screen(); strings.Contains(screen, "scanning…") || !hasRow(screen, "alpha ~/code main") {
			t.Errorf("cache %q: after the refresh's result: want alpha and no scanning…:\n%s", body, screen)
		}
	}
}

func TestALaunchWithACacheNeverShowsScanning(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.start()

	d := e.launch()
	if screen := d.screen(); strings.Contains(screen, "scanning…") {
		t.Errorf("before the cache is read: want no scanning…:\n%s", screen)
	}
	d.next() // the cache read
	if screen := d.screen(); strings.Contains(screen, "scanning…") {
		t.Errorf("with the cached list: want no scanning…:\n%s", screen)
	}
}

func TestWithNoReposFoundTheScreenNamesTheScannedPathsAndTheConfigFile(t *testing.T) {
	e := newEnv(t)
	for _, dir := range []string{"code", "projects"} {
		if err := os.MkdirAll(filepath.Join(e.home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.config(`paths = ["~/code", "~/projects"]`)

	screen := e.start().screen()

	for _, want := range []string{"No repos found in ~/code, ~/projects", "~/.config/loper/config.toml"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen does not say %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "scanning…") {
		t.Errorf("screen still says scanning…:\n%s", screen)
	}
}

// alphaBetaGamma makes three repos, alpha, beta and gamma, and launches
// once so the cache holds them.
func alphaBetaGamma(e *env) map[string]string {
	e.t.Helper()
	repos := map[string]string{}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		repos[name] = e.repo("code/" + name)
	}
	e.start()
	return repos
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func TestWhenTheSelectedRepoDisappearsTheSelectionMovesToTheNearestRow(t *testing.T) {
	for _, c := range []struct {
		keys []string
		gone string
		want string
	}{
		{[]string{"j"}, "beta", "gamma"}, // the row that takes its place
		{[]string{"G"}, "gamma", "beta"}, // the last row: the one above
		{[]string{"g"}, "alpha", "beta"}, // the first row
	} {
		e := newEnv(t)
		repos := alphaBetaGamma(e)
		remove(t, repos[c.gone])
		d := relaunch(e)
		d.press(c.keys...)

		d.release()

		if got := d.selected(); !startsWith(got, c.want+" ~/code") {
			t.Errorf("%s selected and gone: selected %q, want %s", c.gone, got, c.want)
		}
	}
}

func TestWhenAnotherRepoDisappearsOrAppearsTheSelectionStaysOnTheSameRepo(t *testing.T) {
	for _, c := range []struct {
		gone, added string
		order       []string
	}{
		{"alpha", "", []string{"beta ~/code", "gamma ~/code"}},                             // beta moves up a row
		{"", "aaa", []string{"aaa ~/code", "alpha ~/code", "beta ~/code", "gamma ~/code"}}, // beta moves down a row
	} {
		e := newEnv(t)
		repos := alphaBetaGamma(e)
		if c.gone != "" {
			remove(t, repos[c.gone])
		}
		if c.added != "" {
			e.repo("code/" + c.added) // sorts first
		}
		d := relaunch(e)
		d.press("j") // beta

		d.release()

		if got := order(d); !slices.Equal(got, c.order) {
			t.Errorf("%q gone, %q added: rows %q, want %q", c.gone, c.added, got, c.order)
		}
		if got := d.selected(); !startsWith(got, "beta ~/code") {
			t.Errorf("%q gone, %q added: selected %q, want beta still", c.gone, c.added, got)
		}
	}
}

func TestUntilAKeyIsPressedTheTopRowStaysSelectedWhenTheRefreshChangesTheList(t *testing.T) {
	e := newEnv(t)
	alphaBetaGamma(e)
	e.repo("code/aaa") // sorts first
	d := relaunch(e)

	d.release()

	if got := d.selected(); !startsWith(got, "aaa ~/code") {
		t.Errorf("selected %q, want the top row, aaa", got)
	}
}

func TestTheTargetMenuClosesWhenItsRepoDisappearsAndStaysWhenAnotherDoes(t *testing.T) {
	for _, c := range []struct {
		gone     []string
		menu     string // the menu's title, or "" when it is closed
		selected string
	}{
		{[]string{"beta"}, "", "gamma"},
		{[]string{"alpha"}, "Open beta", "beta"},
		{[]string{"alpha", "beta", "gamma"}, "", ""},
	} {
		e := newEnv(t)
		repos := alphaBetaGamma(e)
		for _, name := range c.gone {
			remove(t, repos[name])
		}
		d := relaunch(e)
		d.press("j", "enter") // the menu for beta

		d.release()

		screen := d.screen()
		if c.menu == "" && boxRows(d) != nil || c.menu != "" && !strings.Contains(screen, c.menu) {
			t.Errorf("%v gone: want menu %q:\n%s", c.gone, c.menu, screen)
		}
		if c.selected == "" {
			if !strings.Contains(screen, "No repos found") {
				t.Errorf("%v gone: want no repos found:\n%s", c.gone, screen)
			}
			d.press("enter", "z")
			if got := e.targetArgs("zed"); got != nil {
				t.Errorf("%v gone: zed called with %q on an empty list", c.gone, got)
			}
		} else if got := d.selected(); !startsWith(got, c.selected+" ~/code") {
			t.Errorf("%v gone: selected %q, want %s", c.gone, got, c.selected)
		}
	}
}

func TestTheErrorDialogStaysWhenTheSelectedRepoDisappears(t *testing.T) {
	e := newEnv(t)
	repos := alphaBetaGamma(e)
	brokenTarget(e, "exit 1\n")
	remove(t, repos["beta"]) // the target fails either way: it cannot start in a folder that is gone
	d := relaunch(e)
	d.hold = false    // the open's result comes at once; the refresh stays held
	d.press("j", "b") // a failed open of beta
	if screen := d.screen(); !strings.Contains(screen, "Could not open beta in Broken") {
		t.Fatalf("want the dialog for the failed open:\n%s", screen)
	}

	d.release()

	if screen := d.screen(); !strings.Contains(screen, "Could not open beta in Broken") {
		t.Errorf("want the dialog for the failed open still shown:\n%s", screen)
	}
	d.press("esc")
	if got := d.selected(); !startsWith(got, "gamma ~/code") {
		t.Errorf("selected %q, want gamma", got)
	}
}

func TestTheCacheIsWrittenUnderDotCacheWhenXDGCacheHomeIsUnsetOrRelative(t *testing.T) {
	for _, value := range []string{"", "relative/cache"} {
		e := newEnv(t)
		e.repo("code/alpha")
		t.Setenv("XDG_CACHE_HOME", value)
		t.Chdir(e.home) // so a relative folder that is not ignored lands in the temporary home

		e.start()

		if readCache(t, filepath.Join(e.home, ".cache")) == nil {
			t.Errorf("XDG_CACHE_HOME=%q: no cache under ~/.cache", value)
		}
		if readCache(t, filepath.Join(e.home, "relative", "cache")) != nil {
			t.Errorf("XDG_CACHE_HOME=%q: cache written under the relative folder", value)
		}
	}
}

func TestTheCacheIsReadFromXDGCacheHomeWhenItIsSet(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(e.home, "xdg-cache"))
	e.start()
	e.repo("code/beta")

	d := relaunch(e)

	if screen := d.screen(); !hasRow(screen, "alpha ~/code main") {
		t.Errorf("want the cached list from XDG_CACHE_HOME:\n%s", screen)
	}
	if readCache(t, filepath.Join(e.home, ".cache")) != nil {
		t.Error("cache also written under ~/.cache")
	}
}

func TestTheCacheFolderIsPrivateToTheUserAndHoldsOnlyTheCacheFile(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")

	e.start()
	e.start()

	dir := filepath.Join(e.home, ".cache", "loper")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("cache folder has mode %v, want %v", got, os.FileMode(0o700))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "repos.json" {
		t.Errorf("cache folder holds %v, want only repos.json", entries)
	}
}

func TestACacheThatCannotBeWrittenLeavesTheRefreshedListOnScreen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.writeFile(filepath.Join(e.home, ".cache", "loper"), "a file where the folder should be")

	d := e.start()

	if screen := d.screen(); !hasExactRow(screen, "alpha ~/code main -") || header(d) != "repos 1 sort: last opened" {
		t.Errorf("want the refreshed list:\n%s", screen)
	}
	if got, want := footer(d), "enter open z Zed s sort r refresh q quit"; got != want {
		t.Errorf("footer reads %q, want %q", got, want)
	}
}

func TestAKeyPressedWhileScanningLeavesTheTopRowSelectedOnceTheListArrives(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.openedBefore(map[string]time.Duration{beta: time.Hour})
	d := relaunch(e) // no cache: scanning…

	d.press("j")
	d.releaseLastFirst() // the scan, then the history, which moves beta to the top

	if got := d.selected(); !startsWith(got, "beta ~/code") {
		t.Errorf("selected %q, want the top row, beta", got)
	}
}
