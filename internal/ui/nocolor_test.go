package ui_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// colours returns the parameters of the SGR sequences in s that set or
// reset a colour: foreground, background or underline colour.
func colours(s string) []string {
	var found []string
	for _, seq := range sgr.FindAllString(s, -1) {
		for _, p := range strings.Split(seq[2:len(seq)-1], ";") {
			if n, _ := strconv.Atoi(p); 30 <= n && n <= 49 || 90 <= n && n <= 107 || n == 58 || n == 59 {
				found = append(found, p)
			}
		}
	}
	return found
}

func TestWithNoColourTheStylesStayAndNoColourIsDrawn(t *testing.T) {
	screens := map[bool][]string{}
	for _, noColor := range []bool{false, true} {
		e := newEnv(t)
		e.color, e.noColor = !noColor, noColor
		e.repo("code/alpha")
		e.repo("code/alps")
		e.config(zedTarget)
		d := e.start()
		d.send(tea.WindowSizeMsg{Width: 80, Height: 24})
		list := d.screen()
		d.press("/", "a", "l")
		search := d.screen()
		d.press("esc", "?")
		help := d.screen()
		d.press("esc", "a")
		add := d.screen()
		screens[noColor] = []string{list, search, help, add}
	}

	list, search, help, add := screens[true][0], screens[true][1], screens[true][2], screens[true][3]
	for _, c := range []struct{ what, screen, seq string }{
		{"the selected row in reverse video", list, "\x1b[7m"},
		{"the folders dimmed", list, "\x1b[2m~/code"},
		{"the footer's keys bold", list, "\x1b[1mz\x1b[22m Zed"},
		{"the matched letters underlined", search, "\x1b[4ma\x1b[24m"},
		{"the help's keys bold", help, "\x1b[1mz\x1b[22m"},
		{"the add dialog's examples dimmed", add, "\x1b[2mzed {path}\x1b[22m"},
	} {
		if !strings.Contains(c.screen, c.seq) {
			t.Errorf("with no colour, want %s (%q):\n%q", c.what, c.seq, c.screen)
		}
	}
	for i, screen := range screens[true] {
		if got := colours(screen); len(got) > 0 {
			t.Errorf("with no colour, screen %d sets the colours %q:\n%q", i, got, screen)
		}
		// Apart from the colour, the screen is the one drawn with colour.
		if !slices.Equal(styledLines(screen), styledLines(screens[false][i])) {
			t.Errorf("screen %d differs from colour's in more than colour:\n%s\nwith colour:\n%s",
				i, strings.Join(styledLines(screen), "\n"), strings.Join(styledLines(screens[false][i]), "\n"))
		}
	}
	if got := colours(screens[false][0]); len(got) == 0 {
		t.Errorf("with colour, the list sets no colour:\n%q", screens[false][0])
	}
}

// styledLines is the lines of screen as styled marks them.
func styledLines(screen string) []string {
	var lines []string
	for _, line := range strings.Split(screen, "\n") {
		lines = append(lines, styled(line))
	}
	return lines
}
