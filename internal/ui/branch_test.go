package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestARepoOnABranchShowsThatBranch(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.git(alpha, "commit", "-q", "--allow-empty", "-m", "init")
	e.git(alpha, "checkout", "-q", "-b", "feature/x")

	screen := e.start().screen()

	if !hasRow(screen, "alpha ~/code feature/x") {
		t.Errorf("screen has no row %q:\n%s", "alpha ~/code feature/x", screen)
	}
}

func TestARepoWithADetachedHeadShowsItsShortCommitHash(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	e.git(alpha, "commit", "-q", "--allow-empty", "-m", "init")
	e.git(alpha, "checkout", "-q", "--detach")
	hash := e.gitOutput(alpha, "rev-parse", "--short=7", "HEAD")

	screen := e.start().screen()

	if want := "alpha ~/code @" + hash; !hasRow(screen, want) {
		t.Errorf("screen has no row %q:\n%s", want, screen)
	}
}

func TestAFreshRepoWithNoCommitsShowsTheBranchItWillStartOn(t *testing.T) {
	e := newEnv(t)
	e.git(e.home, "init", "-q", "-b", "trunk", "code/fresh")

	screen := e.start().screen()

	if !hasRow(screen, "fresh ~/code trunk") {
		t.Errorf("screen has no row %q:\n%s", "fresh ~/code trunk", screen)
	}
}

func TestNameFolderAndBranchStartOnAStraightEdgeEvenForWideNames(t *testing.T) {
	e := newEnv(t)
	e.repo("code/caf\u00e9")
	e.repo("code/日本")
	e.repo("code/work/backend")

	screen := e.start().screen()

	var rows []string
	for _, line := range strings.Split(screen, "\n") {
		for _, name := range []string{"café", "日本", "backend"} {
			if strings.Contains(line, name) {
				rows = append(rows, line)
			}
		}
	}
	if len(rows) != 3 {
		t.Fatalf("want a row for each of the 3 repos, got:\n%s", screen)
	}
	for _, col := range []string{"~/code", " main"} {
		want := cellColumn(t, rows[0], col)
		for _, row := range rows[1:] {
			if got := cellColumn(t, row, col); got != want {
				t.Errorf("%q starts at cell %d in %q, want %d:\n%s", col, got, row, want, screen)
			}
		}
	}
}

// cellColumn returns the display cell at which s first appears in line.
func cellColumn(t *testing.T, line, s string) int {
	t.Helper()
	i := strings.Index(line, s)
	if i < 0 {
		t.Fatalf("%q not in %q", s, line)
	}
	return ansi.StringWidth(line[:i])
}

func TestARepoWithACorruptHeadShowsNoBranch(t *testing.T) {
	e := newEnv(t)
	alpha := e.repo("code/alpha")
	if err := os.WriteFile(filepath.Join(alpha, ".git", "HEAD"), []byte("not a head\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	screen := e.start().screen()

	if !hasExactRow(screen, "alpha ~/code -") {
		t.Errorf("screen has no row %q:\n%s", "alpha ~/code -", screen)
	}
}
