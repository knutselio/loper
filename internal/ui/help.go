package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// helpKey handles a key press while the help is open: it scrolls, closes
// or quits, and every other key does nothing.
func (m Model) helpKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.quit()
	case "j", "down":
		m.helpTop = min(m.helpTop+1, len(m.helpLines)-m.helpRows())
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "esc", "?":
		m.help = false
	}
	return m, nil
}

// helpRows is the number of rows of the help there is room for between its
// borders, or 0 when the list is too short for the help.
func (m Model) helpRows() int {
	return max(min(len(m.helpLines), m.listHeight()-2), 0)
}

// helpContent returns the rows of the help, the keys from KeyList and those
// of the help itself, and the width of the widest.
func (m Model) helpContent() (rows []string, width int) {
	sections := append(KeyList(), Section{Title: "In this help", Bindings: []Binding{
		{Keys: "j / k, ↓ / ↑", Action: "Scroll"},
		{Keys: "esc, ?", Action: "Close the help"},
	}})
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.Bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.Keys))
		}
	}
	for i, s := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		if s.Title != "" {
			rows = append(rows, " "+s.Title+" ")
		}
		for _, b := range s.Bindings {
			rows = append(rows, " "+pad(m.key(b.Keys), keyWidth)+"  "+b.Action+" ")
		}
	}
	rows = append(rows, "", " Targets ")
	if len(m.deps.Targets) == 0 {
		rows = append(rows, " None yet. Press "+m.key("a")+" to add one. ")
	}
	for _, t := range m.deps.Targets {
		name := t.Name
		if t.Key == "" {
			name += " (through the menu)"
		}
		rows = append(rows, " "+pad(m.key(t.Key), keyWidth)+"  "+name+" ")
	}
	for _, r := range rows {
		width = max(width, ansi.StringWidth(r))
	}
	return rows, width
}
