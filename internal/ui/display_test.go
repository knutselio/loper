package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// hostile is outside text that would change the screen if it were drawn as
// it is, and how it shows instead.
var hostile = []struct{ kind, text, shows string }{
	{"SGR", "\x1b[1;31mred", `\x1b[1;31mred`},
	{"OSC 8 hyperlink", "\x1b]8;;x\x1b\\link\x1b]8;;\x1b\\", `\x1b]8;;x\x1b\\link\x1b]8;;\x1b\\`},
	{"DCS", "\x1bPpay\x1b\\", `\x1bPpay\x1b\\`},
	{"APC", "\x1b_pay\x1b\\", `\x1b_pay\x1b\\`},
	{"carriage return", "ab\rcd", `ab\rcd`},
	{"line feed", "ab\ncd", `ab\ncd`},
	{"tab", "ab\tcd", `ab\tcd`},
}

// rawControls returns the control characters other than line breaks in
// screen, leaving out the tool's own styles when colour is on.
func rawControls(screen string, color bool) []rune {
	if color {
		screen = sgr.ReplaceAllString(screen, "")
	}
	var found []rune
	for _, r := range screen {
		if r != '\n' && unicode.IsControl(r) {
			found = append(found, r)
		}
	}
	return found
}

func TestOutsideTextIsDrawnEscapedOnOneRowInEveryStyleAndWidth(t *testing.T) {
	for _, field := range []string{"name", "folder", "branch"} {
		for _, h := range hostile {
			t.Run(field+", "+h.kind, func(t *testing.T) {
				e := newEnv(t)
				e.repo("code/plain")
				switch field {
				case "name":
					e.repo("code/" + h.text)
				case "folder":
					e.repo("code/" + h.text + "/alpha")
				case "branch":
					alpha := e.repo("code/alpha")
					e.writeFile(filepath.Join(alpha, ".git", "HEAD"), "ref: refs/heads/"+h.text+"\n")
				}
				for _, style := range []string{"name-folder", "root-path", "name"} {
					for _, color := range []bool{false, true} {
						for _, width := range []int{80, 40} {
							t.Run(fmt.Sprintf("%s, colour %v, width %d", style, color, width), func(t *testing.T) {
								e.config(`row_style = "` + style + `"`)
								e.color = color
								d := e.start()
								d.send(tea.WindowSizeMsg{Width: width, Height: 24})
								screen := d.screen()
								if got := rawControls(screen, color); len(got) > 0 {
									t.Fatalf("the screen holds control characters %q:\n%s", got, screen)
								}
								if strings.Contains(screen, h.text) {
									t.Fatalf("the screen holds %q unescaped:\n%s", h.text, screen)
								}
								lines := strings.Split(ansi.Strip(screen), "\n")
								if len(lines) != 24 {
									t.Fatalf("the screen has %d lines, want 24:\n%s", len(lines), screen)
								}
								if !strings.HasPrefix(lines[3], " > ") {
									t.Errorf("the first row has no selection marker:\n%s", screen)
								}
								if field != "folder" || style != "name" { // the name style shows no folder
									if width == 80 && !strings.Contains(lines[3], h.shows) {
										t.Errorf("the first row does not show %q:\n%s", h.shows, screen)
									}
								}
								for _, l := range lines {
									if w := ansi.StringWidth(l); w > width {
										t.Errorf("a line is %d cells wide, wider than %d: %q", w, width, l)
									}
								}
								// Each column after the name starts where its heading does,
								// in both rows. The escaped text is ASCII, so a rune is a cell.
								heading := []rune(lines[2])
								for _, word := range []string{"FOLDER", "BRANCH", "OPENED"} {
									s := strings.Index(lines[2], word)
									if s < 0 {
										continue
									}
									s = len([]rune(lines[2][:s]))
									for _, row := range lines[3:5] {
										r := []rune(row)
										if len(r) <= s || r[s-1] != ' ' || r[s] == ' ' {
											t.Errorf("%s does not start at cell %d in %q:\n%s", word, s, row, string(heading))
										}
									}
								}
							})
						}
					}
				}
			})
		}
	}
}

func TestTwoReposWhoseNamesDifferOnlyInAControlCharacterLookDifferent(t *testing.T) {
	e := newEnv(t)
	e.repo("code/ab")
	e.repo("code/a\x01b")
	e.repo("code/a\x02b")

	got := order(e.start())

	want := []string{`a\x01b ~/code`, `a\x02b ~/code`, "ab ~/code"}
	if !slices.Equal(got, want) {
		t.Errorf("rows read %q, want %q", got, want)
	}
}

func TestOpeningARepoWithControlCharactersPassesAndRecordsItsRealPath(t *testing.T) {
	e := newEnv(t)
	odd := e.repo("code/\x1b[31mred\nx")
	e.config(zedTarget)
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 200, Height: 24})

	d.press("z")

	if got := e.record("zed.args"); got != odd+"\n" {
		t.Errorf("zed called with %q, want %q", got, odd+"\n")
	}
	if _, ok := savedHistory(t, os.Getenv("XDG_STATE_HOME"))[odd]; !ok {
		t.Errorf("history has no entry for %q", odd)
	}
	if !strings.Contains(d.screen(), `Opened \x1b[31mred\nx in Zed`) {
		t.Errorf("no escaped notice:\n%s", d.screen())
	}
	if got := rawControls(d.screen(), false); len(got) > 0 {
		t.Errorf("the screen holds control characters %q:\n%s", got, d.screen())
	}
}

func TestTheMenuAndTheErrorDialogShowARepoWithControlCharactersEscaped(t *testing.T) {
	e := newEnv(t)
	e.repo("code/\x1b[31mred\rx")
	script := brokenTarget(e, "exit 1\n")
	d := e.start()
	d.send(tea.WindowSizeMsg{Width: 400, Height: 24})

	d.press("enter")
	if !strings.Contains(d.screen(), `Open \x1b[31mred\rx`) {
		t.Errorf("the menu's title is not escaped:\n%s", d.screen())
	}
	if got := rawControls(d.screen(), false); len(got) > 0 {
		t.Errorf("the menu holds control characters %q:\n%s", got, d.screen())
	}
	d.press("esc", "b")

	if !strings.Contains(d.screen(), `Could not open \x1b[31mred\rx in Broken`) {
		t.Errorf("the dialog's title is not escaped:\n%s", d.screen())
	}
	if got, want := boxRows(d)[0], "$ "+script+" --flag '"+e.home+`/code/\x1b[31mred\rx'`; got != want {
		t.Errorf("the dialog's command line reads %q, want %q", got, want)
	}
	if got := rawControls(d.screen(), false); len(got) > 0 {
		t.Errorf("the dialog holds control characters %q:\n%s", got, d.screen())
	}
}

func TestNoReposFoundShowsAPathWithControlCharactersEscaped(t *testing.T) {
	e := newEnv(t)
	e.config(`paths = ["~/no\u001b[31mpe"]`)

	screen := e.start().screen()

	if !strings.Contains(screen, `No repos found in ~/no\x1b[31mpe`) {
		t.Errorf("no escaped path:\n%s", screen)
	}
	if got := rawControls(screen, false); len(got) > 0 {
		t.Errorf("the screen holds control characters %q:\n%s", got, screen)
	}
}

func TestSearchFindsARepoWithControlCharactersAndHighlightsWhatIsDrawn(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/\x1b[31mred")
	e.repo("code/plain")
	d := e.start()

	d.press("/")
	d.typeText("red")

	if got, want := styledRows(d), []string{`[>] \x1b[31m[r][e][d] {~/code} main -`}; !slices.Equal(got, want) {
		t.Errorf("rows read %q, want %q", got, want)
	}
}

func TestC1DelAndBytesThatAreNotUTF8AreShownEscapedEachInItsOwnForm(t *testing.T) {
	e := newEnv(t)
	e.repo("code/n\u009bm\x7fo")
	alpha := e.repo("code/alpha")
	// The file system takes no name that is not UTF-8, but HEAD holds any byte.
	e.writeFile(filepath.Join(alpha, ".git", "HEAD"), "ref: refs/heads/b\u009br\x7fa\xffn\n")

	screen := e.start().screen()

	if got := rawControls(screen, false); len(got) > 0 {
		t.Errorf("the screen holds control characters %q:\n%s", got, screen)
	}
	if !utf8.ValidString(screen) {
		t.Errorf("the screen holds bytes that are not UTF-8: %q", screen)
	}
	for _, want := range []string{`alpha ~/code b\u009br\x7fa\xffn`, `n\u009bm\x7fo ~/code main`} {
		if !hasRow(screen, want) {
			t.Errorf("screen has no row %q:\n%s", want, screen)
		}
	}
}

func TestANameWithABackslashLooksDifferentFromOneWithTheControlCharacterItSpells(t *testing.T) {
	e := newEnv(t)
	e.repo(`code/a\x01b`)
	e.repo("code/a\x01b")

	got := order(e.start())

	want := []string{`a\\x01b ~/code`, `a\x01b ~/code`}
	if !slices.Equal(got, want) {
		t.Errorf("rows read %q, want %q", got, want)
	}
}

func TestSearchHighlightsWhatIsDrawnInAFolderAndABranchWithControlCharacters(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/q\x01zz/alpha")
	beta := e.repo("code/beta")
	e.writeFile(filepath.Join(beta, ".git", "HEAD"), "ref: refs/heads/w\x01vv\n")
	d := e.start()

	for _, c := range []struct{ query, want string }{
		{"qzz", `[>] alpha {~/code/[q]\x01[z][z]} main -`},
		{"wvv", `[>] beta {~/code} [w]\x01[v][v] -`},
	} {
		d.press("/")
		d.typeText(c.query)
		if got := styledRows(d); !slices.Equal(got, []string{c.want}) {
			t.Errorf("searching %q: rows read %q, want %q", c.query, got, []string{c.want})
		}
		d.press("esc")
	}
}
