package ui_test

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cached returns the branch of each repo in the cache file, by path.
func cached(t *testing.T, e *env) map[string]string {
	t.Helper()
	repos := map[string]string{}
	if c := readCache(t, filepath.Join(e.home, ".cache")); c != nil {
		for _, r := range c.Repos {
			repos[r.Path] = r.Branch
		}
	}
	return repos
}

func TestRPicksUpARepoCreatedWhileTheToolWasOpen(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	beta := e.repo("code/beta")

	d.hold = true // the refresh's result stays on its way
	d.press("r")

	if got, want := header(d), "loper · 1 repo sort: last opened ⠋ refreshing"; got != want {
		t.Errorf("while refreshing: header reads %q, want %q", got, want)
	}
	d.release()
	if screen := d.screen(); !hasRow(screen, "beta ~/code main") {
		t.Errorf("want beta after the refresh:\n%s", screen)
	}
	if _, ok := cached(t, e)[beta]; !ok {
		t.Error("the cache does not hold beta")
	}
}

func TestRWhileARefreshIsRunningDoesNotStartASecondOne(t *testing.T) {
	for _, running := range []string{"launch", "light launch", "r"} {
		e := newEnv(t)
		e.repo("code/alpha")
		if running == "light launch" {
			e.config(`cache_max_age = "4h"`)
		}
		d := e.start()
		if running != "r" {
			e.now = e.now.Add(time.Hour) // a light refresh with a max age, a full one without
			d = relaunch(e)              // the launch refresh has run; its result is on its way
		} else {
			d.hold = true
			d.press("r")
		}
		beta := e.repo("code/beta")

		d.press("r")

		if _, ok := cached(t, e)[beta]; ok {
			t.Errorf("%s refresh running: r rewrote the cache with beta", running)
		}
		d.release()
		if screen := d.screen(); hasRow(screen, "beta") {
			t.Errorf("%s refresh running: r found beta:\n%s", running, screen)
		}
	}
}

func TestRBeforeTheCachedListArrivesDoesNotStartASecondRefresh(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.launch() // the cache read is on its way; the launch refresh starts when it arrives

	d.press("r")

	if readCache(t, filepath.Join(e.home, ".cache")) != nil {
		t.Error("r wrote the cache before the launch refresh started")
	}
}

func TestRWhileTheLastSpinnerTickIsOnItsWayKeepsOneSpinner(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start() // the refresh has finished, with a tick on its way

	d.hold = true
	d.press("r")
	d.tick()

	if got, want := header(d), "loper · 1 repo sort: last opened ⠙ refreshing"; got != want {
		t.Errorf("after a tick: header reads %q, want %q", got, want)
	}
	d.release()
	d.tick() // the tick that was on its way when the refresh finished
	if d.tick() {
		t.Error("the spinner still ticks after the refresh finished: two spinners ran")
	}
}

func TestRAfterTheSpinnerStoppedStartsItAgain(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	d := e.start()
	d.tick() // the last tick of the launch refresh's spinner

	d.hold = true
	d.press("r")

	if !d.tick() {
		t.Fatal("the spinner does not tick while r's refresh runs")
	}
	if got, want := header(d), "loper · 1 repo sort: last opened ⠙ refreshing"; got != want {
		t.Errorf("after a tick: header reads %q, want %q", got, want)
	}
}

// scannedAt returns when the cache file says its repos were scanned.
func scannedAt(t *testing.T, e *env) time.Time {
	t.Helper()
	c := readCache(t, filepath.Join(e.home, ".cache"))
	if c == nil {
		t.Fatal("no cache file")
	}
	return c.ScannedAt
}

func TestALaunchOnACacheYoungerThanTheMaxAgeRereadsTheCachedReposWithoutScanning(t *testing.T) {
	e := newEnv(t)
	e.config(`cache_max_age = "4h"`)
	alpha := e.repo("code/alpha")
	beta := e.repo("code/beta")
	e.start()
	scanned := e.now
	e.repo("code/gamma")
	e.git(alpha, "checkout", "-q", "-b", "feature")
	remove(t, beta)
	e.now = e.now.Add(time.Hour)

	d := e.start()

	screen := d.screen()
	if !hasRow(screen, "alpha ~/code feature") {
		t.Errorf("want alpha on its new branch:\n%s", screen)
	}
	if hasRow(screen, "beta") || hasRow(screen, "gamma") {
		t.Errorf("want beta dropped and gamma not found:\n%s", screen)
	}
	if got, want := cached(t, e), map[string]string{alpha: "feature"}; !maps.Equal(got, want) {
		t.Errorf("cache holds %q, want %q", got, want)
	}
	if got := scannedAt(t, e); !got.Equal(scanned) {
		t.Errorf("cache scanned at %v, want the time of the last scan, %v", got, scanned)
	}
}

func TestACacheRereadWithoutScanningStillExpiresAtTheMaxAgeFromItsScan(t *testing.T) {
	e := newEnv(t)
	e.config(`cache_max_age = "4h"`)
	e.repo("code/alpha")
	start := e.now
	e.start()
	e.repo("code/gamma")

	e.now = start.Add(3 * time.Hour)
	if screen := e.start().screen(); hasRow(screen, "gamma") {
		t.Errorf("3h after the scan: want gamma not found:\n%s", screen)
	}
	e.now = start.Add(5 * time.Hour)
	if screen := e.start().screen(); !hasRow(screen, "gamma ~/code main") {
		t.Errorf("5h after the scan: want gamma found:\n%s", screen)
	}
}

func TestALaunchOnACacheOlderThanTheMaxAgeScans(t *testing.T) {
	e := newEnv(t)
	e.config(`cache_max_age = "4h"`)
	e.repo("code/alpha")
	e.start()
	e.repo("code/gamma")
	e.now = e.now.Add(5 * time.Hour)

	d := e.start()

	if screen := d.screen(); !hasRow(screen, "gamma ~/code main") {
		t.Errorf("want gamma found:\n%s", screen)
	}
	if got := scannedAt(t, e); !got.Equal(e.now) {
		t.Errorf("cache scanned at %v, want now, %v", got, e.now)
	}
}

func TestALaunchOnACacheScannedFromOtherPathsScansWhateverItsAge(t *testing.T) {
	for _, c := range []struct{ before, after string }{
		{`["~/code"]`, `["~/code", "~/projects"]`},
		{`["~/projects", "~/code"]`, `["~/code", "~/projects"]`}, // another order counts as a different list
	} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.repo("projects/beta")
		e.config("cache_max_age = \"4h\"\npaths = " + c.before)
		e.start()
		e.repo("code/gamma")
		e.config("cache_max_age = \"4h\"\npaths = " + c.after)
		e.now = e.now.Add(time.Hour)

		screen := e.start().screen()

		for _, want := range []string{"alpha ~/code", "beta ~/projects", "gamma ~/code"} {
			if !hasRow(screen, want) {
				t.Errorf("paths %s, then %s: want %s:\n%s", c.before, c.after, want, screen)
			}
		}
	}
}

func TestByDefaultEveryLaunchScans(t *testing.T) {
	for _, body := range []string{"", `cache_max_age = "0"`} {
		e := newEnv(t)
		e.config(body)
		e.repo("code/alpha")
		e.start()
		e.repo("code/gamma") // the clock has not moved: the cache is as fresh as can be

		if screen := e.start().screen(); !hasRow(screen, "gamma ~/code main") {
			t.Errorf("%q: want gamma found:\n%s", body, screen)
		}
	}
}

func TestRScansEvenWhenTheCacheIsYoungerThanTheMaxAge(t *testing.T) {
	e := newEnv(t)
	e.config(`cache_max_age = "4h"`)
	e.repo("code/alpha")
	e.start()
	e.repo("code/gamma")
	e.now = e.now.Add(time.Hour)
	d := e.start()
	if screen := d.screen(); hasRow(screen, "gamma") {
		t.Fatalf("launch: want gamma not found yet:\n%s", screen)
	}

	d.press("r")

	if screen := d.screen(); !hasRow(screen, "gamma ~/code main") {
		t.Errorf("want gamma found:\n%s", screen)
	}
	if got := scannedAt(t, e); !got.Equal(e.now) {
		t.Errorf("cache scanned at %v, want now, %v", got, e.now)
	}
}

func TestAMissingPathIsReportedOnEveryLaunchWithoutScanning(t *testing.T) {
	e := newEnv(t)
	e.config("cache_max_age = \"4h\"\npaths = [\"~/code\", \"~/typo\"]")
	e.repo("code/alpha")
	e.start()
	e.repo("code/gamma")
	e.now = e.now.Add(time.Hour)

	d := e.start()

	if screen := d.screen(); hasRow(screen, "gamma") {
		t.Errorf("want a launch without a scan, so no gamma:\n%s", screen)
	}
	if got := header(d); !strings.Contains(got, "~/typo not found") {
		t.Errorf("header reads %q, want the warning for ~/typo", got)
	}
}

func TestACacheScannedLaterThanNowScans(t *testing.T) {
	e := newEnv(t)
	e.config(`cache_max_age = "4h"`)
	e.repo("code/alpha")
	e.start()
	e.repo("code/gamma")
	e.now = e.now.Add(-time.Hour) // the clock was turned back

	d := e.start()

	if screen := d.screen(); !hasRow(screen, "gamma ~/code main") {
		t.Errorf("want gamma found:\n%s", screen)
	}
	if got := scannedAt(t, e); !got.Equal(e.now) {
		t.Errorf("cache scanned at %v, want now, %v", got, e.now)
	}
}
