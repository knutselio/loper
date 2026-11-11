package ui_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestColumnsShowsOnlyTheListedColumnsInTheirOrder(t *testing.T) {
	for _, c := range []struct{ columns, want string }{
		{`["name", "branch"]`, "alpha main"},
		{`["branch", "name"]`, "main alpha"},
		{`["last_opened", "folder", "name"]`, "1h ago ~/code alpha"},
	} {
		e := newEnv(t)
		alpha := e.repo("code/alpha")
		e.openedBefore(map[string]time.Duration{alpha: time.Hour})
		e.config("columns = " + c.columns)

		d := e.start()

		if screen := d.screen(); !hasExactRow(screen, c.want) {
			t.Errorf("columns = %s: want the row %q:\n%s", c.columns, c.want, screen)
		}
	}
}

func TestEachColumnButTheLastIsPaddedToItsWidestValueInDisplayCells(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.git(alpha, "checkout", "-q", "-b", "a-longer-branch")
	e.repo("code/日本語") // six cells wide in three characters
	e.config(`columns = ["folder", "name", "branch"]`)

	lines := strings.Split(e.start().screen(), "\n")

	want := []string{
		" > ~/code  alpha   a-longer-branch",
		"   ~/code  日本語  main", // no trailing spaces after the last column
	}
	if got := lines[2:4]; !slices.Equal(got, want) {
		t.Errorf("rows read %q, want %q", got, want)
	}
}

func TestAColumnAddedBetweenLaunchesShowsFromTheCachedList(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.config(`columns = ["name"]`)
	e.start()
	e.git(alpha, "checkout", "-q", "-b", "feature") // only the refresh sees it
	e.config(`columns = ["name", "branch"]`)

	d := relaunch(e)

	if screen := d.screen(); !hasExactRow(screen, "alpha main") {
		t.Errorf("cached list: want alpha with its cached branch:\n%s", screen)
	}
	d.release()
	if screen := d.screen(); !hasExactRow(screen, "alpha feature") {
		t.Errorf("refreshed list: want alpha on feature:\n%s", screen)
	}
}

func TestRowStyleNameShowsTheNameAndNoFolderColumn(t *testing.T) {
	for _, columns := range []string{"", `columns = ["name", "folder", "branch"]`} { // the default lists the folder
		e := newEnv(t)
		e.repo("code/alpha")
		e.config("row_style = \"name\"\n" + columns)

		d := e.start()

		if screen := d.screen(); !hasRow(screen, "alpha main") || strings.Contains(screen, "~/code") {
			t.Errorf("%q: want alpha on main and no folder:\n%s", columns, screen)
		}
	}
}

// rootPathTree makes repos at several depths under ~/code, one listed
// directly and one under a symlinked root, with paths in that order.
func rootPathTree(e *env) {
	e.repo("code/alpha")
	e.repo("code/work/mono/backend")
	e.repo("code/work/web")
	e.repo("dotfiles")
	e.repo("elsewhere/tools/lint")
	e.symlink(filepath.Join(e.home, "elsewhere"), "linked")
	e.config("row_style = \"root-path\"\npaths = [\"~/code\", \"~/dotfiles\", \"~/linked\"]")
}

// rootPathRows are the rows of rootPathTree, sorted by name.
var rootPathRows = []string{
	"alpha main -",
	"dotfiles main -",
	"tools/lint main -",
	"work/mono/backend main -",
	"work/web main -",
}

func TestRowStyleRootPathShowsThePathFromTheScanRootAndNoFolderColumn(t *testing.T) {
	e := newEnv(t)
	rootPathTree(e)

	d := e.start()

	if got := rows(d); !slices.Equal(got, rootPathRows) {
		t.Errorf("rows %q, want %q", got, rootPathRows)
	}
	d.press("G", "enter", "enter") // work/web
	if got := e.targetArgs("zed"); len(got) != 1 || got[0] != filepath.Join(e.home, "code", "work", "web") {
		t.Errorf("zed called with %q, want work/web's path", got)
	}
}

func TestRowStyleRootPathShowsTheSameRowsFromTheCacheAsAfterTheRefresh(t *testing.T) {
	for _, maxAge := range []string{"", `cache_max_age = "4h"`} { // a full refresh, then a light one
		e := newEnv(t)
		rootPathTree(e)
		e.config("row_style = \"name\"\npaths = [\"~/code\", \"~/dotfiles\", \"~/linked\"]")
		e.start() // caches the list under another style
		e.config("row_style = \"root-path\"\npaths = [\"~/code\", \"~/dotfiles\", \"~/linked\"]\n" + maxAge)

		d := relaunch(e)

		if got := rows(d); !slices.Equal(got, rootPathRows) {
			t.Errorf("%q: cached rows %q, want %q", maxAge, got, rootPathRows)
		}
		d.release()
		if got := rows(d); !slices.Equal(got, rootPathRows) {
			t.Errorf("%q: refreshed rows %q, want %q", maxAge, got, rootPathRows)
		}
		d = relaunch(e) // on the cache the refresh wrote
		if got := rows(d); !slices.Equal(got, rootPathRows) {
			t.Errorf("%q: rows cached by the refresh %q, want %q", maxAge, got, rootPathRows)
		}
	}
}

// rows returns each row of the list, top to bottom, with its spacing
// collapsed and without the selection marker.
func rows(d *driver) []string {
	lines := strings.Split(d.screen(), "\n")
	var rows []string
	for _, line := range lines[2 : len(lines)-2] {
		if row := strings.Join(strings.Fields(strings.TrimPrefix(line, " > ")), " "); row != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

func TestUnderRootPathBothSortsOrderByTheDisplayedPathIgnoringCase(t *testing.T) {
	e := newEnv(t)
	e.repo("code/zeta")
	e.repo("code/Work/a") // first by name alone, and first if case counted
	e.repo("code/web/b")  // first by path, ignoring case
	opened := e.repo("code/zz")
	e.openedBefore(map[string]time.Duration{opened: time.Hour})
	e.config(`row_style = "root-path"`)
	d := e.start()

	if got, want := rows(d), []string{"zz main 1h ago", "web/b main -", "Work/a main -", "zeta main -"}; !slices.Equal(got, want) {
		t.Errorf("by last opened: rows %q, want %q", got, want)
	}
	d.press("s")
	if got, want := rows(d), []string{"web/b main -", "Work/a main -", "zeta main -", "zz main 1h ago"}; !slices.Equal(got, want) {
		t.Errorf("by name: rows %q, want %q", got, want)
	}
}

// styled returns s with each run of dimmed text in braces, each run of
// otherwise styled text in brackets, and every other escape sequence
// removed, so a test sees what is dimmed and what is highlighted. A run
// with no text, which the screen does not show, is left out.
func styled(s string) string {
	dim, other := false, false
	on := map[string]bool{} // the attributes and colours other than dim that are on
	s = sgr.ReplaceAllStringFunc(s, func(seq string) string {
		d := dim
		for _, p := range strings.Split(seq[2:len(seq)-1], ";") {
			switch {
			case p == "" || p == "0":
				d = false
				clear(on)
			case p == "2":
				d = true
			case p == "22": // ends bold as well as dim
				d = false
				delete(on, "1")
			case p == "39":
				delete(on, "foreground")
			case p == "49":
				delete(on, "background")
			case len(p) == 2 && p[0] == '2': // 24 and 27 end underline and reverse
				delete(on, p[1:])
			case len(p) == 2 && (p[0] == '3' || p[0] == '9'):
				on["foreground"] = true
			case len(p) == 2 && p[0] == '4' || len(p) == 3:
				on["background"] = true
			default:
				on[p] = true
			}
		}
		o := len(on) > 0
		var b strings.Builder
		if other && !o {
			b.WriteString("]")
		}
		if dim && !d {
			b.WriteString("}")
		}
		if !dim && d {
			b.WriteString("{")
		}
		if !other && o {
			b.WriteString("[")
		}
		dim, other = d, o
		return b.String()
	})
	return strings.NewReplacer("[]", "", "{}", "").Replace(s)
}

// styledRows returns each row of the list as styled shows it, top to bottom,
// with its spacing collapsed.
func styledRows(d *driver) []string {
	lines := strings.Split(d.screen(), "\n")
	var rows []string
	for _, line := range lines[2 : len(lines)-2] {
		if row := strings.Join(strings.Fields(styled(line)), " "); row != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

func TestTheFolderAndThePathBeforeTheNameAreDimmed(t *testing.T) {
	for _, c := range []struct {
		style string
		want  []string
	}{
		{"name-folder", []string{"[>] backend {~/code/work/mono} main -", "dotfiles {~} main -"}},
		{"root-path", []string{"[>] dotfiles main -", "{work/mono/}backend main -"}},
		{"name", []string{"[>] backend main -", "dotfiles main -"}},
	} {
		e := newEnv(t)
		e.color = true
		e.repo("code/work/mono/backend")
		e.repo("dotfiles")
		e.config("row_style = \"" + c.style + "\"\npaths = [\"~/code\", \"~/dotfiles\"]")

		d := e.start()

		if got := styledRows(d); !slices.Equal(got, c.want) {
			t.Errorf("%s: rows read %q, want %q", c.style, got, c.want)
		}
	}
}

func TestMatchedLettersAreHighlightedInTheCellsOfEachRowStyle(t *testing.T) {
	for _, c := range []struct {
		style, query string
		want         []string
	}{
		{"name-folder", "wrk", []string{"[>] backend {~/code/[w]o[r][k]/mono} main -"}},
		{"root-path", "monob", []string{"[>] {work/[m][o][n][o]/}[b]ackend main -"}},
		{"root-path", "kend", []string{"[>] {work/mono/}bac[k][e][n][d] main -"}},
		{"name", "back", []string{"[>] [b][a][c][k]end main -"}},
	} {
		e := newEnv(t)
		e.color = true
		e.repo("code/work/mono/backend")
		e.repo("code/alpha")
		e.config("row_style = \"" + c.style + "\"")
		d := e.start()

		d.press("/")
		d.typeText(c.query)

		if got := styledRows(d); !slices.Equal(got, c.want) {
			t.Errorf("%s, %q: rows read %q, want %q", c.style, c.query, got, c.want)
		}
	}
}

func TestTheSearchMatchesTheFolderAndBranchEvenWhenTheyAreNotShown(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	beta := e.repo("projects/beta")
	e.git(beta, "checkout", "-q", "-b", "feature")
	e.config("row_style = \"name\"\ncolumns = [\"name\"]\npaths = [\"~/code\", \"~/projects\"]")

	for _, query := range []string{"proj", "feat"} {
		d := e.start()
		d.press("/")
		d.typeText(query)

		if got, want := rows(d), []string{"beta"}; !slices.Equal(got, want) {
			t.Errorf("%q: rows %q, want %q", query, got, want)
		}
	}
}

func TestUnderRootPathARepoAlsoListedDirectlyShowsItsBareNameInEitherOrder(t *testing.T) {
	for _, paths := range []string{`["~/code", "~/code/work/backend"]`, `["~/code/work/backend", "~/code"]`} {
		e := newEnv(t)
		e.repo("code/alpha")
		e.repo("code/work/backend")
		e.config("row_style = \"root-path\"\npaths = " + paths)

		d := e.start()

		if got, want := rows(d), []string{"alpha main -", "backend main -"}; !slices.Equal(got, want) {
			t.Errorf("paths = %s: rows %q, want %q", paths, got, want)
		}
	}
}
