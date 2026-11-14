package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// safe turns outside text, such as a repo's name, folder or branch, into
// display text: control characters and bytes that are not UTF-8 are shown
// escaped, as \n, \r, \t, \x1b or \u009b, so none reaches the terminal. A
// backslash is shown as \\, so two different names never look the same.
// Other printable text is kept.
func safe(s string) string {
	return escape(s, `\\`)
}

// message is safe for a message, such as a loader's, that names files as
// they are but quotes other values as Go does, escaped already: a backslash
// is kept as it is, so a quoted value is not escaped twice.
func message(s string) string {
	return escape(s, `\`)
}

// escape is safe, with a backslash shown as backslash.
func escape(s, backslash string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[0])
		case r == '\\':
			b.WriteString(backslash)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < utf8.RuneSelf && unicode.IsControl(r): // C0 and DEL
			fmt.Fprintf(&b, `\x%02x`, r)
		case unicode.IsControl(r): // C1, told apart from a byte that is not UTF-8
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[:n])
		}
		s = s[n:]
	}
	return b.String()
}

// clean makes s safe to draw on one line: escape sequences and control
// characters, which would move the cursor or change the screen, are
// dropped, and a tab becomes spaces.
func clean(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
