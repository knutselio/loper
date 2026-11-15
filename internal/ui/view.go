package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/matcher"
	"github.com/knutselio/loper/internal/reltime"
)

// headings are the column headings, by column.
var headings = map[string]string{"name": "NAME", "folder": "FOLDER", "branch": "BRANCH", "last_opened": "OPENED"}

func (m Model) View() tea.View {
	now := m.deps.Now()
	// The width of each column's widest value, over all repos and the
	// heading, so the columns stay put while searching.
	full := make([]int, len(m.columns))
	for i, c := range m.columns {
		full[i] = ansi.StringWidth(headings[c])
	}
	for _, r := range m.all {
		for i, c := range m.columns {
			full[i] = max(full[i], ansi.StringWidth(m.cell(c, r, matcher.Match{}, now, math.MaxInt)))
		}
	}
	shown, widths := m.fit(full)
	rule := ansi.Truncate(" "+m.dim(strings.Repeat("─", max(m.width-2, 0)))+" ", m.width, "")
	noun := "repos"
	if len(m.repos) == 1 {
		noun = "repo"
	}
	count, warnings := fmt.Sprintf(" %s · %d %s", m.bold("loper"), len(m.repos), noun), ""
	for _, path := range m.missing {
		warnings += "  " + safe(m.shorten(path)) + " not found"
	}
	sort := "  sort: last opened "
	if m.byName {
		sort = "  sort: name "
	}
	if m.searching { // the search line replaces the count, the warnings and the sort
		count, warnings, sort = " /"+m.query, "", ""
	}
	spin := ""
	if m.refreshing && len(m.all) > 0 { // with no list yet, the spinner is in the list
		spin = "  " + spinner[m.frame] + " refreshing "
	}
	// The sort and the spinner keep their place on the right. What does not
	// fit gives way: the warnings first, then the sort, then the spinner; the
	// count, or the search line, is cut last.
	right := sort + spin
	if ansi.StringWidth(count+right) > m.width {
		warnings, right = "", spin
	}
	if ansi.StringWidth(count+right) > m.width {
		right = ""
	}
	room := m.width - ansi.StringWidth(right)
	if over := ansi.StringWidth(count) - room; m.searching && over > 0 && room >= ansi.StringWidth(" /…") {
		count = " /…" + cutStart(m.query, over+1) // the end of the query, where the typing is
	}
	left := ansi.Truncate(count, room, "…")
	if rest := room - ansi.StringWidth(left); rest >= ansi.StringWidth("  …") { // less would not read as a cut warning
		left += ansi.Truncate(warnings, rest, "…")
	}
	lines := []string{pad(left, room) + right, rule}
	var empty []string // what an empty list says instead
	switch {
	case len(m.repos) > 0:
	case m.searching && len(m.all) > 0:
		empty = []string{"No matches"}
	case !m.refreshing:
		paths := make([]string, len(m.deps.Paths))
		for i, p := range m.deps.Paths {
			paths[i] = safe(m.shorten(p))
		}
		empty = []string{"No repos found in " + strings.Join(paths, ", "), "Change paths in " + safe(m.shorten(m.deps.ConfigFile))}
	case m.loaded:
		empty = []string{spinner[m.frame] + " scanning…"}
	}
	// join lines up the shown columns, each cell from cell given its room.
	// The last_opened column is right-aligned.
	join := func(cell func(c string, room int) string) string {
		cells := make([]string, len(shown))
		for j, k := range shown {
			room := widths[k]
			if len(shown) == 1 { // the name alone has the whole row but its last cell
				room = max(m.width-4, 0)
			}
			cells[j] = cell(m.columns[k], room)
			switch {
			case m.columns[k] == "last_opened":
				cells[j] = strings.Repeat(" ", widths[k]-ansi.StringWidth(cells[j])) + cells[j]
			case j < len(cells)-1: // the last column is not padded, so a row has no trailing spaces
				cells[j] = pad(cells[j], widths[k])
			}
		}
		return strings.Join(cells, "  ")
	}
	if m.pageHeight() < m.listHeight() {
		line := join(func(c string, room int) string { return headings[c] })
		lines = append(lines, ansi.Truncate("   "+m.dim(line), max(m.width-1, 0), "…"))
	}
	for i := m.offset; i < m.offset+m.pageHeight(); i++ {
		if i >= len(m.repos) {
			line := ""
			if i < len(empty) {
				line = ansi.Truncate("   "+empty[i], max(m.width-1, 0), "…")
			}
			lines = append(lines, line)
			continue
		}
		r := m.repos[i]
		marker := "   "
		if i == m.selected {
			marker = " " + m.bold(">") + " "
		}
		var h matcher.Match
		if m.hits != nil {
			h = m.hits[i]
		}
		cells := join(func(c string, room int) string { return m.cell(c, r, h, now, room) })
		line := ansi.Truncate(marker+cells, max(m.width-1, 0), "…")
		if i == m.selected {
			line = m.bar(line, m.width)
		}
		lines = append(lines, line)
	}
	keys := m.footer()
	if m.notice != "" {
		keys = ansi.Truncate(" "+message(m.notice), m.width, "…")
	}
	if m.quitting {
		keys = ansi.Truncate(" Quitting when the open is done", m.width, "…")
	}
	switch {
	case m.failure != nil:
		lines = m.overlayFailure(lines)
	case m.add.open:
		lines = m.overlayAdd(lines)
	case m.menu:
		lines = m.overlayMenu(lines)
	case m.help:
		lines = m.overlay(lines, m.box(" Help ", m.helpLines[m.helpTop:m.helpTop+m.helpRows()], m.helpWidth))
	}
	lines = append(lines, rule, keys)
	lines = lines[:min(len(lines), m.height)] // never taller than the terminal
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// footer is the footer's line of keys: the tool's own keys, then the keyed
// targets, then the keys that come last, separated by " | ". What does not fit gives way whole items at a
// time from the end, and a … then ends the line.
func (m Model) footer() string {
	var items, last []string
	for _, s := range KeyList() {
		for _, b := range s.Bindings {
			switch {
			case b.FooterLabel == "":
			case b.FooterLast:
				last = append(last, m.key(b.FooterKey)+" "+b.FooterLabel)
			default:
				items = append(items, m.key(b.FooterKey)+" "+b.FooterLabel)
			}
		}
	}
	for _, t := range m.deps.Targets {
		if t.Key != "" {
			items = append(items, m.key(t.Key)+" "+t.Name)
		}
	}
	items = append(items, last...)
	for n := len(items); n >= 0; n-- {
		shown := items[:n]
		if n < len(items) {
			shown = append(slices.Clone(shown), "…")
		}
		if line := " " + strings.Join(shown, m.dim(" | ")); ansi.StringWidth(line) <= m.width {
			return line
		}
	}
	return ansi.Truncate(" …", m.width, "")
}

// fit returns the indexes of the columns shown and the width of each column,
// given the width of each column's widest value. While the rows would reach
// the terminal's last cell, the folder and branch columns shrink, the widest
// first, down to 12 cells; a column narrower than that keeps its width. When
// that is not enough, last_opened is dropped, then branch, then folder,
// whatever order the columns are in, and the columns left shrink again from
// their full width. The name never shrinks here. A last_opened column
// shown last is widened to the room left.
func (m Model) fit(full []int) (shown, widths []int) {
	shown = make([]int, len(m.columns))
	for i := range shown {
		shown[i] = i
	}
	for _, drop := range []string{"last_opened", "branch", "folder", ""} { // "" drops nothing: the last pass only shrinks
		widths = slices.Clone(full)
		for {
			width := 3 + 2*(len(shown)-1) // the marker, and the gaps between columns
			widest := -1                  // the folder or branch column to shrink next
			for _, i := range shown {
				width += widths[i]
				if c := m.columns[i]; (c == "folder" || c == "branch") && widths[i] > 12 && (widest < 0 || widths[i] > widths[widest]) {
					widest = i
				}
			}
			if width <= m.width-1 { // the last cell stays blank
				// A last_opened column shown last takes the room left, so it
				// ends one cell before the edge, as the header line and the
				// footer do.
				if last := shown[len(shown)-1]; m.columns[last] == "last_opened" {
					widths[last] += m.width - 1 - width
				}
				return shown, widths
			}
			if widest < 0 {
				break
			}
			widths[widest]--
		}
		shown = slices.DeleteFunc(shown, func(i int) bool { return m.columns[i] == drop })
	}
	return shown, widths
}

// cell returns the text of column c for r, as of now, with the letters
// matched by h highlighted, cut with … to room cells.
func (m Model) cell(c string, r repo, h matcher.Match, now time.Time, room int) string {
	var s string
	switch c {
	case "name":
		// Under the root-path style, the path before the name is dimmed.
		name := m.name(r)
		cut := strings.LastIndex(name, string(filepath.Separator)) + 1
		k := utf8.RuneCountInString(name[:cut])
		var rest []int // the matched positions in the name itself, counted from its start
		for _, p := range h.Name {
			if p >= k {
				rest = append(rest, p-k)
			}
		}
		path, base := m.highlight(name[:cut], h.Name), m.highlight(name[cut:], rest)
		over := ansi.StringWidth(path+base) - room
		switch {
		case over <= 0:
			return m.dim(path) + base
		case ansi.StringWidth(base) < room: // the path gives way from its start, so the name stays whole
			return m.dim("…"+cutStart(path, over+1)) + base
		}
		return ansi.Truncate(base, room, "…")
	case "folder":
		s = m.dim(m.highlight(safe(m.shorten(filepath.Dir(r.path))), h.Folder))
	case "branch":
		s = m.highlight(safe(r.branch), h.Branch)
	default: // last_opened
		s = "-"
		if at, ok := m.history[r.path]; ok {
			s = reltime.Format(at, now)
		}
	}
	return ansi.Truncate(s, room, "…")
}

// name is the text of r's name column: its name or, under the root-path
// style, its path from its root, which for a repo listed directly in paths is
// its name, as display text.
func (m Model) name(r repo) string {
	if m.deps.RowStyle == "root-path" {
		// The error is dropped: in a list the tool scanned or cached itself, a
		// repo's path is its root or a folder under it, both absolute. A cache
		// edited by hand could break that; the row then shows "" or a path
		// with "..", and the next refresh puts it right.
		if rel, _ := filepath.Rel(r.root, r.path); rel != "." {
			return safe(rel)
		}
	}
	return safe(filepath.Base(r.path))
}

// listHeight is the number of screen lines left for the list between the
// header, the footer and their rules.
func (m Model) listHeight() int {
	return max(m.height-4, 0)
}

// pageHeight is the number of rows of the list on screen at a time: the
// list's height, less the headings line, which shows only when there are
// rows and room for one under it.
func (m Model) pageHeight() int {
	if len(m.repos) > 0 && m.listHeight() > 1 {
		return m.listHeight() - 1
	}
	return m.listHeight()
}

// shorten replaces the home folder at the start of path with ~.
func (m Model) shorten(path string) string {
	if path == m.deps.Home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, m.deps.Home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
