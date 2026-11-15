package ui_test

import (
	"path/filepath"
	"slices"
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

	screen := startQuiet(e).screen()

	for _, row := range []string{"alpha ~/code main", "dotfiles ~ main", "/ search 2 repos"} {
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
	e.config(`paths = ["~/dots", "~/work"]` + zedTarget)
	d := startQuiet(e)
	screen := d.screen()

	for _, row := range []string{"dots ~ main", "backend ~/work main", "/ search 2 repos"} {
		if !hasRow(screen, row) {
			t.Errorf("screen has no row %q:\n%s", row, screen)
		}
	}
	d.press("j", "enter", "enter") // dots sorts after backend
	if got := e.targetArgs("zed"); len(got) != 1 || got[0] != filepath.Join(e.home, "dots") {
		t.Errorf("target called with %q, want the path as written", got)
	}
}

func TestAListedPathThatDoesNotExistIsAProblemTopRightAndTheRestStillLoads(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.config(`paths = ["~/code", "~/typo", "~/nope"]`)

	d := startQuiet(e)

	if got, want := topRight(d), "! ~/typo not found, ~/nope not found"; got != want {
		t.Errorf("top right reads %q, want %q", got, want)
	}
	if !hasRow(d.screen(), "alpha ~/code main") {
		t.Errorf("screen has no row for alpha:\n%s", d.screen())
	}
}

func TestAProblemIsRedWithColourAndKeepsItsMarkWithout(t *testing.T) {
	for _, c := range []struct {
		color bool
		want  string
	}{
		{true, " \x1b[31m! ~/typo not found\x1b[39m "},
		{false, " ! ~/typo not found "},
	} {
		e := newEnv(t)
		e.color, e.noColor = c.color, !c.color
		e.repo("code/alpha")
		e.config(`paths = ["~/code", "~/typo"]`)

		if top := strings.SplitN(startQuiet(e).screen(), "\n", 2)[0]; !strings.HasSuffix(top, c.want) {
			t.Errorf("colour %v: top row is %q, want it to end with %q", c.color, top, c.want)
		}
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

func TestOverlappingPathsListARepoOnce(t *testing.T) {
	e := newEnv(t)
	e.repo("code/alpha")
	e.repo("code/beta")
	e.config(`paths = ["~/code/beta", "~/code", "~/code/beta"]`)

	d := startQuiet(e)

	if !hasRow(d.screen(), "/ search 2 repos") {
		t.Errorf("header does not count 2 repos:\n%s", d.screen())
	}
	if got, want := order(d), []string{"alpha ~/code", "beta ~/code"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q, each once", got, want)
	}
}
