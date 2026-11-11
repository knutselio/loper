package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestARepoSeveralLevelsDeepIsListed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/a/b/c/deep")

	screen := e.start().screen()

	if !hasRow(screen, "deep ~/code/a/b/c main") {
		t.Errorf("screen has no row for deep:\n%s", screen)
	}
}

func TestARepoNestedInsideAnotherRepoIsNotListed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/outer")
	e.repo("code/outer/vendor/inner")

	screen := e.start().screen()

	if !hasRow(screen, "outer ~/code main") {
		t.Errorf("screen has no row for outer:\n%s", screen)
	}
	if strings.Contains(screen, "inner") {
		t.Errorf("nested repo is listed:\n%s", screen)
	}
}

func TestARepoInsideAHiddenFolderIsNotListed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/.cache/hidden")

	screen := e.start().screen()

	if !hasRow(screen, "alpha ~/code main") {
		t.Errorf("screen has no row for alpha:\n%s", screen)
	}
	if strings.Contains(screen, "hidden") {
		t.Errorf("repo in a hidden folder is listed:\n%s", screen)
	}
}

func TestARepoReachableOnlyThroughASymlinkIsNotListed(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	linked := e.repo("elsewhere/linked")
	e.symlink(linked, "code/linked")
	e.symlink(filepath.Dir(linked), "code/folder-link")

	screen := e.start().screen()

	if !hasRow(screen, "alpha ~/code main") {
		t.Errorf("screen has no row for alpha:\n%s", screen)
	}
	if strings.Contains(screen, "linked") {
		t.Errorf("repo behind a symlink is listed:\n%s", screen)
	}
}

func TestAFolderWhoseGitIsASymlinkIsNotListed(t *testing.T) {
	e := newEnv(t)
	real := e.repo("elsewhere/real")
	if err := os.MkdirAll(filepath.Join(e.home, "code", "fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.symlink(filepath.Join(real, ".git"), "code/fake/.git")

	screen := e.start().screen()

	if strings.Contains(screen, "fake") {
		t.Errorf("folder with a symlinked .git is listed:\n%s", screen)
	}
}

func TestALinkedWorktreeIsNotListedButItsPrimaryRepoIs(t *testing.T) {
	e := newEnv(t)
	primary := e.repo("code/primary")
	e.git(primary, "commit", "-q", "--allow-empty", "-m", "init")
	e.git(primary, "worktree", "add", "-q", "-b", "feature", filepath.Join(e.home, "code", "feature-tree"))

	screen := e.start().screen()

	if !hasRow(screen, "primary ~/code main") {
		t.Errorf("screen has no row for primary:\n%s", screen)
	}
	if strings.Contains(screen, "feature-tree") {
		t.Errorf("linked worktree is listed:\n%s", screen)
	}
}

func TestARepoInAFolderWithSpacesIsListedAndOpensWithItsPath(t *testing.T) {
	e := newEnv(t)
	spaced := e.repo("code/side projects/my repo")
	d := e.start()

	if got := d.selected(); got != "my repo ~/code/side projects main" {
		t.Errorf("selected %q, want the repo with spaces", got)
	}
	d.press("enter")
	if got := e.targetArgs("zed"); len(got) != 1 || got[0] != spaced {
		t.Errorf("target called with %q, want [%q]", got, spaced)
	}
}
