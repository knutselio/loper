package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const reverse = "\x1b[7m"

// barError says why line is not a bar width cells wide: in reverse video
// from its first cell to its last, with no reset part-way, and with the
// inner style, a parameter such as 2 for dim or 4 for underline, inside it.
// It returns "" for a bar.
func barError(line string, width int, inner string) string {
	if !strings.HasPrefix(line, reverse) {
		return "it does not start in reverse video"
	}
	if w := ansi.StringWidth(line); w != width {
		return fmt.Sprintf("it is %d cells wide, not %d", w, width)
	}
	seen := false
	for _, loc := range sgr.FindAllStringIndex(line, -1) {
		for _, p := range strings.Split(line[loc[0]+2:loc[1]-1], ";") {
			seen = seen || p == inner
			if rest := ansi.Strip(line[loc[1]:]); (p == "27" || p == "0" || p == "") && rest != "" {
				return fmt.Sprintf("reverse video ends before %q", rest)
			}
		}
	}
	if !seen {
		return "it has no " + inner + " inside"
	}
	return ""
}

func TestWithColourTheSelectedRowIsABarAcrossTheScreenThatMovesWithTheSelection(t *testing.T) {
	for _, c := range []struct {
		name, config string
		width        int
		keys         []string
		inner        string // a style inside the bar that must not end it
	}{
		{"a dimmed folder", "", 80, nil, "2"},
		{"a dimmed path", `row_style = "root-path"`, 80, nil, "2"},
		{"underlined matches", "", 80, []string{"/", "a", "l"}, "4"},
		{"a branch cut with …", "", 36, nil, "2"},
	} {
		e := newEnv(t)
		e.color = true
		alpha := e.repo("code/work/alpha")
		e.git(alpha, "checkout", "-q", "-b", "a-branch-too-long-for-the-width")
		e.repo("code/work/alps")
		e.config(c.config)
		d := e.start()
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})
		d.press(c.keys...)

		lines := strings.Split(d.screen(), "\n")
		if c.width < 80 && !strings.Contains(lines[3], "a-branch-too…") {
			t.Errorf("%s: the selected row %q has no branch cut with …", c.name, lines[3])
		}
		if why := barError(lines[3], c.width, c.inner); why != "" {
			t.Errorf("%s: the selected row %q is not a bar: %s", c.name, lines[3], why)
		}
		if strings.Contains(lines[4], reverse) {
			t.Errorf("%s: the row %q under the selection is in reverse video", c.name, lines[4])
		}

		d.press("down")
		lines = strings.Split(d.screen(), "\n")
		if why := barError(lines[4], c.width, c.inner); why != "" {
			t.Errorf("%s: after down, the selected row %q is not a bar: %s", c.name, lines[4], why)
		}
		if strings.Contains(lines[3], reverse) {
			t.Errorf("%s: after down, the row %q above the selection is still in reverse video", c.name, lines[3])
		}
	}
}

func TestWithColourTheBarOverWideCharactersIsContinuousAndAsWideAsTheScreen(t *testing.T) {
	e := newEnv(t)
	e.color = true
	e.repo("code/日本語のリポジトリ") // 20 cells in 10 characters
	e.repo("code/日本語")
	d := e.start()

	for _, width := range []int{80, 40, 13, 12} { // at 13 and 12 the name alone is cut, before or across a wide character
		d.send(tea.WindowSizeMsg{Width: width, Height: 24})
		if line := strings.Split(d.screen(), "\n")[3]; barError(line, width, "1") != "" {
			t.Errorf("width %d: the selected row %q is not a bar: %s", width, line, barError(line, width, "1"))
		}
	}
}

func TestWithColourTheHighlightedTargetIsABarAcrossTheMenu(t *testing.T) {
	for _, c := range []struct {
		name   string
		config func(e *env)
		width  int
		keys   []string
		target string // the highlighted target
	}{
		{"rows wider than the title", threeTargets, 80, []string{"enter", "j"}, "Menu only"},
		{"the title wider than the rows", func(*env) {}, 80, []string{"enter"}, "Zed"},
		{"a narrow screen", func(*env) {}, 14, []string{"enter"}, "Zed"},
	} {
		e := newEnv(t)
		e.color = true
		e.repo("code/alpha")
		c.config(e)
		d := e.start()
		d.send(tea.WindowSizeMsg{Width: c.width, Height: 24})

		d.press(c.keys...)

		var top, bar string
		for _, line := range strings.Split(d.screen(), "\n") {
			if strings.Contains(line, "╭") {
				top = ansi.Strip(line)
			}
			if strings.Contains(line, "│") && strings.Contains(line, c.target) {
				bar = line
			}
		}
		width := ansi.StringWidth(top[strings.Index(top, "╭")+len("╭") : strings.Index(top, "╮")])
		i, j := strings.Index(bar, reverse), strings.LastIndex(bar, "│")
		if i < 0 || j < i {
			t.Errorf("%s: the highlighted target %q is not in reverse video", c.name, bar)
			continue
		}
		inside := strings.TrimSuffix(bar[i:j], "\x1b[2m") // up to the dimmed right border
		if why := barError(inside, width, "1"); why != "" {
			t.Errorf("%s: the highlighted target %q is not a bar across the menu: %s", c.name, inside, why)
		}
		if strings.Contains(inside, "…") {
			t.Errorf("%s: the highlighted target %q is cut, though it fits", c.name, inside)
		}
	}
}
