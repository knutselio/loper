package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/target"
)

// addDialog is the dialog that adds a target to the config file, or edits or
// removes one: its fields, then the confirmation of the lines it writes. A
// removal has no fields.
type addDialog struct {
	open bool
	// old is the target edited or removed, as the menu listed it, or nil
	// while adding one. Each dialog has its own, so a result for an earlier
	// dialog is told apart by it.
	old        *target.Target
	remove     bool
	refusal    string    // why the config file cannot take the change, shown instead of the fields
	fields     [3]string // the name, the key and the command, as typed
	after      int       // the After open choice after them, an index into choices
	field      int       // the field being typed in, or len(fields) for the choice
	confirming bool      // the confirmation shows
	before     string    // old's lines in the file, while the confirmation shows them
	block      string    // the lines to write, while the confirmation shows them
	ready      bool      // the confirmation's first moment, in which enter is ignored, has passed
	writing    bool      // the change is being written, so keys wait for the result
	problem    string    // why the target was refused or not written
}

// change is the change to the config file that the dialog makes.
func (a addDialog) change() target.Change {
	var quit *bool // default and take over: no close line
	inPlace := choices[a.after] == "take over this window"
	if a.after > 0 && !inPlace {
		c := choices[a.after] == "close loper"
		quit = &c
	}
	return target.Change{Old: a.old, Lines: a.before, Remove: a.remove, Name: a.fields[0], Key: a.fields[1], Command: a.fields[2], Close: quit, InPlace: inPlace}
}

// title is the dialog's title.
func (a addDialog) title() string {
	switch {
	case a.remove:
		return " Remove a target "
	case a.old != nil:
		return " Edit a target "
	}
	return " Add a target "
}

// choiceOf returns the index in choices of what quit and inPlace, a
// target's close and in_place, say.
func choiceOf(quit *bool, inPlace bool) int {
	switch {
	case inPlace:
		return slices.Index(choices, "take over this window")
	case quit == nil:
		return 0
	case *quit:
		return slices.Index(choices, "close loper")
	}
	return slices.Index(choices, "stay open")
}

// fileCheckedMsg carries why the config file cannot take the change of the
// dialog for old, or nil when it can.
type fileCheckedMsg struct {
	old *target.Target
	err error
}

// readyMsg ends the first moment of the confirmation it counts.
type readyMsg int

// checkedMsg carries, for the dialog for old with fields and after, the
// lines its change replaces and those it writes, or why the change is refused.
type checkedMsg struct {
	old           *target.Target
	fields        [3]string
	after         int
	before, block string
	err           error
}

// changedMsg carries the targets once the dialog's change was written, and
// the index of the target added or changed, or of where the removed one was,
// or why it was not written.
type changedMsg struct {
	targets []target.Target
	at      int
	err     error
}

// examples are what the add dialog shows, dimmed, in its empty fields: the
// name, the key and the command of a target.
var examples = [3]string{"Zed", "z", "zed {path}"}

// choices are the add dialog's choices of what happens after an open: what
// close_on_open says, stay open, close loper, or run in its place.
var choices = []string{"default", "stay open", "close loper", "take over this window"}

// addKey handles a key press while the dialog is open. In the fields,
// printable characters are text, so list keys and target keys are too.
func (m Model) addKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	a := &m.add
	switch {
	case a.writing:
	case a.refusal != "":
		switch msg.String() {
		case "q":
			if m.opening { // closed, so the quit goes ahead once the open is done
				m.add = addDialog{}
			}
			return m.quit()
		case "esc":
			m.add = addDialog{}
		}
	case a.confirming:
		switch msg.String() {
		case "enter":
			if a.ready && m.addFits() { // never what was not there to read
				a.writing = true
				return m, m.write()
			}
		case "esc":
			if a.remove {
				m.add = addDialog{}
				break
			}
			a.confirming, a.before, a.block = false, "", ""
		}
	case a.remove: // its lines are still being read
		if msg.String() == "esc" {
			m.add = addDialog{}
		}
	default:
		last := len(a.fields) // the choice, after the text fields
		switch msg.String() {
		case "tab", "down":
			a.field = min(a.field+1, last)
		case "shift+tab", "up":
			a.field = max(a.field-1, 0)
		case "enter":
			if a.field < last {
				a.field++
				break
			}
			if o := a.old; o != nil && strings.TrimSpace(a.fields[0]) == o.Name && a.fields[1] == o.Key && a.fields[2] == o.Command && a.after == choiceOf(o.Close, o.InPlace) {
				m.add = addDialog{} // an edit that changes nothing writes nothing
				break
			}
			return m, m.check()
		case "esc":
			m.add = addDialog{}
		case "backspace":
			if a.field < last {
				if f := []rune(a.fields[a.field]); len(f) > 0 {
					a.fields[a.field] = string(f[:len(f)-1])
				}
			}
		default:
			if a.field < last {
				a.fields[a.field] += msg.Text
				break
			}
			switch msg.String() { // the choice takes no text
			case "right", "space":
				a.after = (a.after + 1) % len(choices)
			case "left":
				a.after = (a.after + len(choices) - 1) % len(choices)
			}
		}
	}
	return m, nil
}

// pauseEnter starts the moment, after the confirmation shows, in which enter
// is ignored.
func (m Model) pauseEnter() (Model, tea.Cmd) {
	m.add.ready = false
	m.confirms++
	n := readyMsg(m.confirms)
	return m, m.deps.Tick(m.deps.EnterPause, func(time.Time) tea.Msg { return n })
}

// checkFile checks in the background whether the config file can take the
// dialog's change, as its fields open: a target added, or the target edited
// as it is.
func (m Model) checkFile() tea.Cmd {
	a := m.add
	return func() tea.Msg {
		if a.old == nil {
			return fileCheckedMsg{err: m.deps.CheckFile(m.deps.ConfigFile, m.deps.Home)}
		}
		_, _, err := m.deps.CheckTarget(m.deps.ConfigFile, m.deps.Home, a.change())
		return fileCheckedMsg{old: a.old, err: err}
	}
}

// check checks the dialog's change against the config file in the
// background.
func (m Model) check() tea.Cmd {
	a := m.add
	return func() tea.Msg {
		before, block, err := m.deps.CheckTarget(m.deps.ConfigFile, m.deps.Home, a.change())
		return checkedMsg{old: a.old, fields: a.fields, after: a.after, before: before, block: block, err: err}
	}
}

// write makes the dialog's change to the config file in the background.
func (m Model) write() tea.Cmd {
	c := m.add.change()
	return func() tea.Msg {
		targets, at, err := m.deps.WriteTarget(m.deps.ConfigFile, m.deps.Home, c)
		return changedMsg{targets: targets, at: at, err: err}
	}
}

// overlayAdd draws the dialog centred over the list part of lines or,
// when the screen is too small for it, a notice in its place: on the middle
// line of the list, or on the header when there is no list.
func (m Model) overlayAdd(lines []string) []string {
	if !m.addFits() {
		row := 0
		if m.listHeight() > 0 {
			row = 2 + (m.listHeight()-1)/2
		}
		lines[row] = ansi.Truncate(" Window too small for the dialog", m.width, "…")
		return lines
	}
	rows, width := m.addRows()
	return m.overlay(lines, m.box(m.add.title(), rows, width))
}

// addFits reports whether the dialog fits on the screen whole, with
// room in its fields for a value.
func (m Model) addFits() bool {
	rows, width := m.addRows()
	return len(rows) <= m.listHeight()-2 && width >= 17 // 17: the margins, the marker, the label and a cell of value
}

// addRows returns the dialog's rows and its width inside the border: why
// the config file cannot take the change, its fields, with the end of a
// value too long for its field, or the lines it will write, replace or
// remove and the file they are in. Text too long for the dialog wraps.
func (m Model) addRows() ([]string, int) {
	a := m.add
	width := m.boxWidth(a.title(), 60)
	wrap := func(s string) []string {
		var rows []string
		for _, l := range strings.Split(ansi.Wrap(clean(s), max(width-2, 1), ""), "\n") {
			rows = append(rows, " "+l+" ")
		}
		return rows
	}
	lines := func(block string) []string {
		var rows []string
		for _, l := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
			rows = append(rows, wrap(l)...)
		}
		return rows
	}
	file := safe(m.shorten(m.deps.ConfigFile))
	var rows []string
	switch {
	case a.refusal != "":
		rows = append(wrap(message(m.shorten(a.refusal))), "", " "+m.key("esc")+" close ") // a loader message starts with the file
	case a.confirming && a.remove:
		rows = slices.Concat(wrap("Remove these lines from "+file+":"), lines(short(a.before)),
			[]string{"", " " + m.key("enter") + " remove  " + m.key("esc") + " keep "})
	case a.confirming && a.old != nil:
		rows = slices.Concat(wrap("Replace these lines in "+file+":"), lines(short(a.before)), []string{""},
			wrap("with these (comments inside the block are not kept):"), lines(a.block),
			[]string{"", " " + m.key("enter") + " write  " + m.key("esc") + " back "})
	case a.confirming:
		rows = slices.Concat(wrap("Append to "+file), []string{""}, lines(a.block),
			[]string{"", " " + m.key("enter") + " write  " + m.key("esc") + " back "})
	case a.remove: // its lines are still being read
		rows = []string{" " + m.key("esc") + " keep "}
	default:
		after := choices[a.after]
		if a.after == 0 {
			after += " (" + choices[choiceOf(&m.deps.CloseOnOpen, false)] + ")"
		}
		values := append(a.fields[:], "‹ "+after+" ›") // drawn as a choice, changed with ← and →
		for i, label := range []string{"Name", "Key", "Command", "After open"} {
			marker := "  "
			if i == a.field {
				marker = m.bold(">") + " "
			}
			// A command as loaded may hold a line break or a tab.
			value := clean(values[i])
			if over := ansi.StringWidth(value) - (width - 16); over > 0 { // 16: the margins, the marker and the label
				value = "…" + cutStart(value, over+1)
			}
			if value == "" && m.deps.Styles { // undimmed the example would read as typed
				value = m.dim(examples[i])
			}
			rows = append(rows, " "+marker+pad(label, 10)+"  "+value+" ")
		}
		if a.problem != "" {
			rows = append(append(rows, ""), wrap(message(m.shorten(a.problem)))...) // a loader message starts with the file
		}
		rows = append(rows, "", " "+m.key("enter")+" next  "+m.key("tab")+" move  "+m.key("esc")+" cancel ")
	}
	return rows, width
}

// short returns block, lines as they are in the config file, cut to its
// first two lines and its last when it is longer than the block the tool
// writes, so a confirmation fits whenever one for such a block would.
func short(block string) string {
	ls := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	if len(ls) <= 5 { // the block the tool writes, with a close line
		return block
	}
	return strings.Join([]string{ls[0], ls[1], fmt.Sprintf("… %d more lines", len(ls)-3), ls[len(ls)-1]}, "\n")
}
