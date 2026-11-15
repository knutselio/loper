package ui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// menuKey handles a key press while the target menu is open.
func (m Model) menuKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if len(m.deps.Targets) == 0 { // the menu says there are none, and a adds one
		switch msg.String() {
		case "q":
			return m.quit()
		case "a":
			m.menu, m.add = false, addDialog{open: true}
			return m, m.checkFile()
		case "esc":
			m.menu = false
		}
		return m, nil
	}
	switch msg.String() {
	case "q":
		return m.quit()
	case "j", "down":
		m.choice = min(m.choice+1, len(m.deps.Targets)-1)
	case "k", "up":
		m.choice = max(m.choice-1, 0)
	case "enter", "space":
		return m.open(m.deps.Targets[m.choice])
	case "e":
		t := m.deps.Targets[m.choice] // a copy of its own, which tells this dialog apart
		m.add = addDialog{open: true, old: &t, fields: [3]string{t.Name, t.Key, t.Command}, after: choiceOf(t.Close, t.InPlace)}
		return m, m.checkFile()
	case "d":
		t := m.deps.Targets[m.choice]
		m.add = addDialog{open: true, old: &t, remove: true}
		return m, m.check()
	case "esc":
		m.menu = false
	default:
		if t, ok := m.keyed(msg); ok {
			return m.open(t)
		}
	}
	return m, nil
}

// menuRows is the number of targets the menu has room to show between its
// borders, or 0 when the list is too short for the menu. With no targets
// the menu is the one row that says so.
func (m Model) menuRows() int {
	return max(min(max(len(m.deps.Targets), 1), m.listHeight()-2), 0)
}

// scrollMenu moves the menu's first shown target so the highlighted one is
// in view.
func (m Model) scrollMenu() Model {
	rows := max(m.menuRows(), 1)
	m.menuTop = min(m.menuTop, m.choice, len(m.deps.Targets)-rows)
	m.menuTop = max(m.menuTop, m.choice-rows+1)
	return m
}

// overlayMenu draws the target menu centred over the list part of lines,
// which stays visible around it, or with no targets a box that says so.
func (m Model) overlayMenu(lines []string) []string {
	if len(m.deps.Targets) == 0 {
		row := " No targets yet. Press " + m.key("a") + " to add one. "
		return m.overlay(lines, m.box("", []string{row}, ansi.StringWidth(row)))
	}
	keyWidth := 0
	for _, t := range m.deps.Targets {
		keyWidth = max(keyWidth, ansi.StringWidth(t.Key))
	}
	title := " Open " + safe(filepath.Base(m.repos[m.selected].path)) + " "
	rows := make([]string, len(m.deps.Targets))
	width := 0
	for i, t := range m.deps.Targets {
		marker, key := "  ", m.key(t.Key)
		if i == m.choice { // in the bar, without the accent
			marker, key = m.bold(">")+" ", m.bold(t.Key)
		}
		rows[i] = " " + marker + pad(key, keyWidth) + "  " + t.Name + " "
		width = max(width, ansi.StringWidth(rows[i]))
	}
	hint := " " + m.key("e") + " edit" + m.dim(" • ") + m.key("d") + " remove "
	width = m.boxWidth(title, max(width, ansi.StringWidth(hint)+2)) // the bar spans the menu
	rows[m.choice] = m.bar(ansi.Truncate(rows[m.choice], width, "…"), width)
	box := m.box(title, rows[m.menuTop:m.menuTop+m.menuRows()], width)
	box[len(box)-1] = m.edge("╰", hint, width, "╯") // the hint in the bottom border, as the title is in the top
	return m.overlay(lines, box)
}
