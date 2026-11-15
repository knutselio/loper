package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The styles, from the terminal's own palette and attributes only, so the
// tool follows the terminal's theme. matchOn and matchOff start and end the
// highlight of a matched letter, dimOn and dimOff a dimmed part (a folder,
// the path before a name, the rules and the borders of a box), boldOn and
// boldOff the marker of the selected row or target, keyOn and keyOff the
// keys of the footer, the help and the target menu, in the accent colour,
// and problemOn and problemOff a problem, in red. barOn and barOff draw the
// selected row or target as a bar, in reverse video; the styles inside the
// bar end only their own attribute, so none of them ends the bar.
var (
	matchOn    = ansi.Style{}.Underline(true).String()
	matchOff   = ansi.Style{}.NoUnderline().String()
	dimOn      = ansi.Style{}.Faint().String()
	dimOff     = ansi.Style{}.Normal().String()
	boldOn     = ansi.Style{}.Bold().String()
	boldOff    = ansi.Style{}.Normal().String()
	keyOn      = ansi.Style{}.Bold().ForegroundColor(ansi.Cyan).String()
	keyOff     = ansi.Style{}.Normal().DefaultForegroundColor().String()
	problemOn  = ansi.Style{}.ForegroundColor(ansi.Red).String()
	problemOff = ansi.Style{}.DefaultForegroundColor().String()
	barOn      = ansi.Style{}.Reverse(true).String()
	barOff     = ansi.Style{}.NoReverse().String()
)

// dim dims s, unless the styles are off.
func (m Model) dim(s string) string {
	if !m.deps.Styles || s == "" {
		return s
	}
	return dimOn + s + dimOff
}

// bold styles s as the marker of what is selected, unless the styles are
// off.
func (m Model) bold(s string) string {
	if !m.deps.Styles || s == "" {
		return s
	}
	return boldOn + s + boldOff
}

// bar draws s, padded to width cells, as a bar, unless the styles are off.
func (m Model) bar(s string, width int) string {
	if !m.deps.Styles {
		return s
	}
	return barOn + pad(s, width) + barOff
}

// key styles s as a key: bold, and in the accent colour unless colour is off.
func (m Model) key(s string) string {
	if !m.deps.Color || s == "" {
		return m.bold(s)
	}
	return keyOn + s + keyOff
}

// problem draws s as a problem: with "! " in front, which says it also
// without colour, and in red unless colour is off.
func (m Model) problem(s string) string {
	if !m.deps.Color {
		return "! " + s
	}
	return problemOn + "! " + s + problemOff
}

// highlight marks the characters of s at the positions pos, which are in
// ascending order and may go past the end of s, unless the styles are off.
func (m Model) highlight(s string, pos []int) string {
	if !m.deps.Styles || len(pos) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if len(pos) > 0 && pos[0] == i {
			b.WriteString(matchOn + string(r) + matchOff)
			pos = pos[1:]
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// box draws rows in a rounded border with title in its top edge. width is
// what the rows want inside the border; the box grows to fit the title and
// shrinks to fit the screen, cutting rows that are too long.
func (m Model) box(title string, rows []string, width int) []string {
	width = m.boxWidth(title, width)
	box := []string{m.edge("╭", title, width, "╮")}
	side := m.dim("│")
	for _, r := range rows {
		box = append(box, side+pad(ansi.Truncate(r, width, "…"), width)+side)
	}
	return append(box, m.dim("╰"+strings.Repeat("─", width)+"╯"))
}

// edge draws the top or bottom border of a box width wide inside, from the
// corner left to the corner right, with text in it after one stroke of the
// border, if there is room, and cut to fit.
func (m Model) edge(left, text string, width int, right string) string {
	stroke := min(width, 1)
	text = ansi.Truncate(text, width-stroke, "")
	return m.dim(left+strings.Repeat("─", stroke)) + text + m.dim(strings.Repeat("─", width-stroke-ansi.StringWidth(text))+right)
}

// boxWidth is the width inside the border of a box with title whose rows
// want width: room for a stroke of the border either side of the title, and
// no more than the screen has inside the two side borders.
func (m Model) boxWidth(title string, width int) int {
	return max(min(max(width, ansi.StringWidth(title)+2), m.width-2), 0)
}

// overlay places box centred over the list part of lines, which stays
// visible around it. The box must fit in the list's height.
func (m Model) overlay(lines, box []string) []string {
	width := ansi.StringWidth(box[0]) - 2
	top := 2 + max(m.listHeight()-len(box), 0)/2
	left := max(m.width-width-2, 0) / 2
	for i, b := range box {
		line := lines[top+i]
		// A wide character cut by either edge of the box becomes a space, so
		// the rest of the row stays on its cells.
		before := pad(ansi.Truncate(line, left, ""), left)
		cut := left + width + 2
		after := ansi.TruncateLeft(line, cut, "")
		if ansi.StringWidth(after) > ansi.StringWidth(line)-cut {
			after = " " + ansi.TruncateLeft(after, 2, "")
		}
		// No style leaks across either edge of the box: Truncate keeps the
		// codes after the cut, so the row's styles are closed before the
		// box, and TruncateLeft keeps those before it, so the rest of the row
		// is styled as it was.
		lines[top+i] = ansi.Truncate(before+b+after, m.width, "")
	}
	return lines
}

// cutStart removes the first n cells of s, and the rest of a wide character
// the cut would split.
func cutStart(s string, n int) string {
	if cut := ansi.TruncateLeft(s, n, ""); ansi.StringWidth(cut) <= ansi.StringWidth(s)-n {
		return cut
	}
	return ansi.TruncateLeft(s, n+1, "")
}

// pad fills s with spaces to width display cells.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}
