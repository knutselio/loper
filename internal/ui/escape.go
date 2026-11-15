package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Safe turns outside text, such as a repo's name, folder or branch, into
// display text: control characters and bytes that are not UTF-8 are shown
// escaped, as \n, \r, \t, \x1b or \u009b, so none reaches the terminal, and
// so are the invisible characters, as \u200b or \u202e. A backslash is shown
// as \\, so two different names never look the same. Other printable text is
// kept.
func Safe(s string) string {
	return escape(s, `\\`)
}

// Message is Safe for a message, such as a loader's, that names files as
// they are but quotes other values as Go does, escaped already: a backslash
// is kept as it is, so a quoted value is not escaped twice.
func Message(s string) string {
	return escape(s, `\`)
}

// escape is Safe, with a backslash shown as backslash.
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
		case unicode.IsControl(r) || invisible(r): // C1, told apart from a byte that is not UTF-8, and the invisible ones
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[:n])
		}
		s = s[n:]
	}
	return b.String()
}

// invisible reports whether r is a zero-width character or a text-direction
// control, which would hide text or reorder it.
func invisible(r rune) bool {
	return r >= 0x200b && r <= 0x200f || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 || r == 0x2060 || r == 0xfeff
}

// clean makes s safe to draw on one line: escape sequences and control
// characters, which would move the cursor or change the screen, and the
// invisible characters are dropped, and a tab becomes spaces.
func clean(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || invisible(r) {
			return -1
		}
		return r
	}, s)
}
