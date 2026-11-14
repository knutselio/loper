package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// folder checks a paths entry: an absolute path, or one that starts with ~.
type folder struct{}

func (*folder) UnmarshalText(b []byte) error {
	p := string(b)
	if p != "~" && !strings.HasPrefix(p, "~/") && !filepath.IsAbs(p) {
		return errors.New("not an absolute path")
	}
	return nil
}

// sortName checks the sort setting: one of the two sorts.
type sortName struct{}

func (*sortName) UnmarshalText(b []byte) error {
	if s := string(b); s != "last_opened" && s != "name" {
		return errors.New("not a sort")
	}
	return nil
}

// column checks a columns entry: one of the four columns.
type column struct{}

func (*column) UnmarshalText(b []byte) error {
	if !slices.Contains([]string{"name", "folder", "branch", "last_opened"}, string(b)) {
		return errors.New("not a column")
	}
	return nil
}

// rowStyle checks the row_style setting: one of the three styles.
type rowStyle struct{}

func (*rowStyle) UnmarshalText(b []byte) error {
	if !slices.Contains([]string{"name-folder", "root-path", "name"}, string(b)) {
		return errors.New("not a row style")
	}
	return nil
}

// maxAge checks the cache_max_age setting: "0", or a positive interval in
// Go's duration syntax, such as "4h" or "1h30m". Days are not a unit.
type maxAge struct{}

func (*maxAge) UnmarshalText(b []byte) error {
	_, err := parseMaxAge(string(b))
	return err
}

// parseMaxAge reads a cache_max_age value.
func parseMaxAge(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil && d < 0 {
		err = errors.New("negative interval")
	}
	return d, err
}

// command checks a target command: words that split cleanly, one of them
// holding {path}.
type command struct{}

func (*command) UnmarshalText(b []byte) error {
	words, err := split(string(b))
	if err == nil && len(words) > 0 && words[0] == "" {
		err = errors.New("has no program to run")
	}
	if err == nil && len(words) > 0 && words[0] == "cd" && len(words) != 2 {
		err = errors.New("needs one folder after cd")
	}
	if err == nil && !slices.ContainsFunc(words, func(w string) bool { return strings.Contains(w, "{path}") }) {
		err = errors.New("has no {path}")
	}
	if err != nil {
		return fmt.Errorf("target command %q %w", b, err)
	}
	return nil
}

// targetName checks a target name: not only spaces, and no control characters,
// which would break the rows or reach the terminal as escapes.
type targetName struct{}

func (*targetName) UnmarshalText(b []byte) error {
	n := string(b)
	if strings.TrimSpace(n) == "" {
		return errNoName
	}
	if strings.ContainsFunc(n, unicode.IsControl) {
		return fmt.Errorf("target name %q has a control character", n)
	}
	return nil
}

var errNoName = errors.New("a target needs a name")

// key checks a target key: one printable, non-space character that is not
// one of the tool's own keys, including those of features still to come.
type key struct{}

func (*key) UnmarshalText(b []byte) error {
	k := string(b)
	if utf8.RuneCountInString(k) != 1 {
		return fmt.Errorf("target key %q is not a single character", k)
	}
	if r, _ := utf8.DecodeRuneInString(k); !unicode.IsGraphic(r) || unicode.IsSpace(r) {
		return fmt.Errorf("target key %q is not a printable character", k)
	}
	if strings.Contains("jkgG/sr?qaed", k) {
		return fmt.Errorf("target key %q is one of loper's own keys", k)
	}
	return nil
}

// split breaks command into words the way a shell does, honouring single
// quotes, double quotes and backslashes. It fails on an unbalanced quote.
func split(command string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false // so that '' is an empty word
	var quote rune  // the open quote, or 0
	escaped := false
	for _, c := range command {
		switch {
		case escaped:
			switch {
			case c == '\n': // a line continuation, dropped
			case quote == '"' && !strings.ContainsRune("$`\"\\", c):
				word.WriteRune('\\') // inside double quotes the backslash stays
				word.WriteRune(c)
			default:
				word.WriteRune(c)
				inWord = true
			}
			escaped = false
		case c == '\\' && quote != '\'':
			escaped = true
		case c == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(c)
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("has an unbalanced quote")
	}
	if escaped {
		return nil, errors.New("ends with a lone backslash")
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}
