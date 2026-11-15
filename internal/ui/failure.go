package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/target"
)

// failedMsg reports that a repo did not open in a target.
type failedMsg struct {
	repo, target string
	target.Failure
}

// errorDialog is the error dialog's content, worked out once when the
// failure arrives rather than on every render.
type errorDialog struct {
	title   string
	command string   // the command line that ran
	output  []string // the lines it printed
	cut     bool     // whether lines it printed before output were dropped
	reason  string   // why it failed, without its margins
}

func (m Model) newErrorDialog(msg failedMsg) *errorDialog {
	d := &errorDialog{
		title:   " Could not open " + Safe(msg.repo) + " in " + msg.target + " ",
		command: " $ " + Safe(m.commandLine(msg.Words)) + " ",
		cut:     msg.Cut,
		reason:  Message(msg.Reason),
	}
	if out := strings.TrimRight(msg.Output, "\n"); out != "" {
		for _, l := range strings.Split(out, "\n") {
			d.output = append(d.output, " "+clean(l)+" ")
		}
	}
	return d
}

// commandLine is the command line of words, quoted as a shell would need
// it, with a word in the home folder drawn from ~, as shorten draws it, and
// the rest of it quoted after the ~.
func (m Model) commandLine(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = target.Quote(w)
		if s := m.shorten(w); s == "~" {
			quoted[i] = s
		} else if rest, ok := strings.CutPrefix(s, "~/"); ok && s != w {
			quoted[i] = "~/" + target.Quote(rest)
		}
	}
	return strings.Join(quoted, " ")
}

// overlayFailure draws the error dialog centred over the list part of
// lines: the command line that ran, then the last lines of what it printed
// that fit, then why it failed, cut at its start so its cause shows. When
// lines are left out, a dim line above the rest says how many, if there is
// room for it.
func (m Model) overlayFailure(lines []string) []string {
	space := m.listHeight() - 2 // the rows between the borders
	if space < 2 {
		return lines // no room for the command and why it failed: the dialog shows again once there is
	}
	f := m.failure
	output := f.output
	var notice []string
	if len(output) > space-2 || f.cut && space > 2 {
		output = output[len(output)-min(len(output), max(space-3, 0)):]
		n := len(f.output) - len(output)
		switch {
		case space == 2:
		case f.cut && n == 0:
			notice = []string{m.dim(" … earlier output left out ")}
		case f.cut:
			notice = []string{m.dim(fmt.Sprintf(" … more than %d earlier lines left out ", n))}
		default:
			notice = []string{m.dim(fmt.Sprintf(" … %d earlier lines left out ", n))} // at least 2
		}
	}
	rows := slices.Concat([]string{f.command}, notice, output)
	width := ansi.StringWidth(" " + f.reason + " ")
	for _, r := range rows {
		width = max(width, ansi.StringWidth(r))
	}
	width = m.fitWidth(f.title, width, m.width) // as wide as its lines, up to the screen
	reason := f.reason
	if over := ansi.StringWidth(reason) - (width - 2); over > 0 { // 2: the margins
		reason = "…" + cutStart(reason, over+1)
	}
	return m.overlay(lines, m.drawBox(f.title, append(rows, " "+reason+" "), width))
}
