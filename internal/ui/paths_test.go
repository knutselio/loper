package ui_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestASecondFolderInPathsAddsItsReposToTheList(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("projects/beta")
	e.config(`paths = ["~/code", "~/projects"]`)

	screen := e.start().screen()

	for _, row := range []string{"alpha ~/code main", "beta ~/projects main"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q:\n%s", row, screen)
		}
	}
}

func TestTheConfigIsReadFromXDGConfigHomeWhenItIsSet(t *testing.T) {
	e := newEnv(t)
	e.repo("projects/beta")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.home, "xdg"))
	e.writeFile(filepath.Join(e.home, "xdg", "loper", "config.toml"), `paths = ["~/projects"]`)

	screen := e.start().screen()

	if !hasRow(screen, "beta ~/projects main") {
		t.Errorf("screen has no row for beta:\n%s", screen)
	}
}

func TestTheConfigIsReadFromDotConfigWhenXDGConfigHomeIsUnset(t *testing.T) {
	e := newEnv(t)
	e.repo("projects/beta")
	t.Setenv("XDG_CONFIG_HOME", "")
	e.writeFile(filepath.Join(e.home, ".config", "loper", "config.toml"), `paths = ["~/projects"]`)

	screen := e.start().screen()

	if !hasRow(screen, "beta ~/projects main") {
		t.Errorf("screen has no row for beta:\n%s", screen)
	}
}

func TestARelativeXDGConfigHomeIsIgnored(t *testing.T) {
	e := newEnv(t)
	e.repo("projects/beta")
	t.Setenv("XDG_CONFIG_HOME", "relative/xdg")
	t.Chdir(e.home)
	e.writeFile(filepath.Join(e.home, "relative", "xdg", "loper", "config.toml"), `paths = ["~/nowhere"]`)
	e.writeFile(filepath.Join(e.home, ".config", "loper", "config.toml"), `paths = ["~/projects"]`)

	screen := e.start().screen()

	if !hasRow(screen, "beta ~/projects main") {
		t.Errorf("screen has no row for beta:\n%s", screen)
	}
}

func TestAPathsEntryThatIsARepoAppearsAsOneRow(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("dotfiles")
	e.config(`paths = ["~/code", "~/dotfiles"]`)

	screen := e.start().screen()

	for _, row := range []string{"alpha ~/code main", "dotfiles ~ main", "repos 2"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q:\n%s", row, screen)
		}
	}
}

func TestAHiddenFolderListedDirectlyIsIncluded(t *testing.T) {
	e := newEnv(t)
	e.repo(".config/nvim")
	e.repo(".local/share/tool")
	e.config(`paths = ["~/.config/nvim", "~/.local"]`)

	screen := e.start().screen()

	if !hasRow(screen, "nvim ~/.config main") {
		t.Errorf("screen has no row for nvim:\n%s", screen)
	}
	if !hasRow(screen, "tool ~/.local/share main") {
		t.Errorf("screen has no row for tool:\n%s", screen)
	}
}

func TestASymlinkListedDirectlyIsFollowedAndShowsThePathAsWritten(t *testing.T) {
	e := newEnv(t)
	e.repo("elsewhere/dotfiles")
	e.repo("elsewhere/work/backend")
	e.repo("elsewhere/hidden-behind-link")
	e.symlink(filepath.Join(e.home, "elsewhere", "dotfiles"), "dots")
	e.symlink(filepath.Join(e.home, "elsewhere", "work"), "work")
	e.symlink(filepath.Join(e.home, "elsewhere", "hidden-behind-link"), "elsewhere/work/inner-link")
	e.config(`paths = ["~/dots", "~/work"]`)
	d := e.start()
	screen := d.screen()

	for _, row := range []string{"dots ~ main", "backend ~/work main", "repos 2"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q:\n%s", row, screen)
		}
	}
	d.press("enter", "enter")
	if got := e.targetArgs("zed"); len(got) != 1 || got[0] != filepath.Join(e.home, "dots") {
		t.Errorf("target called with %q, want the path as written", got)
	}
}

func TestAListedPathThatDoesNotExistIsAWarningInTheHeaderAndTheRestStillLoads(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/typo"]`)

	screen := e.start().screen()

	header := strings.SplitN(screen, "\n", 2)[0]
	if !strings.Contains(header, "~/typo not found") {
		t.Errorf("header %q does not warn about ~/typo", header)
	}
	if !hasRow(screen, "alpha ~/code main") {
		t.Errorf("screen has no row for alpha:\n%s", screen)
	}
}

func TestABareTildeInPathsIsTheHomeFolder(t *testing.T) {
	e := newEnv(t)
	e.repo("dotfiles")
	e.config(`paths = ["~"]`)

	screen := e.start().screen()

	if !hasRow(screen, "dotfiles ~ main") {
		t.Errorf("screen has no row for dotfiles:\n%s", screen)
	}
}

func TestOverlappingPathsListARepoOnceAtItsFirstOccurrence(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.config(`paths = ["~/code/beta", "~/code", "~/code/beta"]`)

	screen := e.start().screen()

	if !hasRow(screen, "repos 2") {
		t.Errorf("header does not count 2 repos:\n%s", screen)
	}
	lines := strings.Split(screen, "\n")
	if !hasRow(lines[2], "beta ~/code main") || !hasRow(lines[3], "alpha ~/code main") {
		t.Errorf("want beta then alpha, each once:\n%s", screen)
	}
}
