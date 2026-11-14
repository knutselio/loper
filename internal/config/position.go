package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// checkTargets reports what the per-value checks cannot see: a target with
// no name or no command, one in place that stays open, and a key used by two
// targets. b is the file at path.
func checkTargets(path string, b []byte, targets []targetFile) error {
	lines, err := targetLines(path, b)
	if err != nil {
		return err
	}
	used := map[string]int{} // key → the line of the target that has it
	for i, t := range targets {
		if t.Name == "" {
			return fmt.Errorf("%s:%d: %w", path, lines[i].start, errNoName)
		}
		if t.Command == "" {
			return fmt.Errorf("%s:%d: target %q has no command; it needs one with {path}", path, lines[i].start, t.Name)
		}
		if words, _ := split(t.Command); words[0] == "cd" && !t.InPlace { // cannot fail: the command was checked while decoding
			return fmt.Errorf("%s:%d: target %q runs cd, which needs in_place = true", path, lines[i].start, t.Name)
		}
		if t.InPlace && t.Close != nil && !*t.Close {
			return fmt.Errorf("%s:%d: target %q has in_place = true, which always closes loper, and close = false", path, lines[i].start, t.Name)
		}
		if first, ok := used[t.Key]; ok {
			return fmt.Errorf("%s:%d: target key %q is already used by the target on line %d", path, lines[i].key, t.Key, first)
		}
		if t.Key != "" {
			used[t.Key] = lines[i].start
		}
	}
	return nil
}

// checkColumns reports what the per-value checks cannot see: a columns
// list without name, and a column listed twice. b is the file at path; a nil
// columns was left out.
func checkColumns(path string, b []byte, columns []string) error {
	if columns == nil {
		return nil
	}
	start, lines := columnLines(b)
	for i, c := range columns {
		if slices.Contains(columns[:i], c) {
			return fmt.Errorf("%s:%d: column %q is listed twice", path, lines[i], c)
		}
	}
	if !slices.Contains(columns, "name") {
		return fmt.Errorf("%s:%d: columns must include \"name\"", path, start)
	}
	return nil
}

// columnLines returns the line of the columns setting in b, a config file
// that has already decoded cleanly, and the line of each of its entries.
func columnLines(b []byte) (int, []int) {
	var p unstable.Parser
	p.Reset(b)
	line := func(n *unstable.Node) int { return p.Shape(n.Raw).Start.Line }
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind != unstable.KeyValue || keyOf(e) != "columns" {
			continue
		}
		var items []int
		for c := e.Value().Children(); c.Next(); {
			items = append(items, line(c.Node()))
		}
		k := e.Key()
		k.Next()
		return line(k.Node()), items
	}
	return 0, nil
}

// targetLine is where a target is in the config file.
type targetLine struct {
	start int // the line the target starts on
	key   int // the line of its key, or 0 when it has none
}

// targetLines returns where each target is in b, the config file at path,
// which has already decoded cleanly, in the order they are written. Targets
// are either [[targets]] tables or inline tables in a targets list; any
// other way of writing them is an error.
func targetLines(path string, b []byte) ([]targetLine, error) {
	var p unstable.Parser
	p.Reset(b)
	line := func(n *unstable.Node) int { return p.Shape(n.Raw).Start.Line }
	var lines []targetLine
	inRoot, inTargets := true, false
	for p.NextExpression() {
		e := p.Expression()
		k := keyOf(e)
		isTargets := k == "targets" || strings.HasPrefix(k, "targets.")
		switch {
		case isTargets && e.Kind != unstable.KeyValue && (e.Kind != unstable.ArrayTable || k != "targets"),
			isTargets && inRoot && e.Kind == unstable.KeyValue && k != "targets":
			h := e.Key()
			h.Next()
			return nil, fmt.Errorf("%s:%d: write each target as a [[targets]] table", path, line(h.Node()))
		case e.Kind == unstable.Table || e.Kind == unstable.ArrayTable:
			inRoot = false
			inTargets = e.Kind == unstable.ArrayTable && keyOf(e) == "targets"
			if inTargets {
				k := e.Key()
				k.Next()
				lines = append(lines, targetLine{start: line(k.Node())})
			}
		case inTargets && keyOf(e) == "key":
			lines[len(lines)-1].key = line(e)
		case inRoot && keyOf(e) == "targets":
			for t := e.Value().Children(); t.Next(); {
				l := targetLine{start: line(t.Node())}
				for kv := t.Node().Children(); kv.Next(); {
					if keyOf(kv.Node()) == "key" {
						l.key = line(kv.Node())
					}
				}
				lines = append(lines, l)
			}
		}
	}
	return lines, nil
}

// keyOf returns the dotted key of a key-value or table node.
func keyOf(n *unstable.Node) string {
	var parts []string
	for k := n.Key(); k.Next(); {
		parts = append(parts, string(k.Node().Data))
	}
	return strings.Join(parts, ".")
}

// describe turns a TOML error into one that names the file and line.
func describe(path string, err error) error {
	var missing *toml.StrictMissingError
	if errors.As(err, &missing) {
		e := missing.Errors[0]
		line, _ := e.Position()
		return fmt.Errorf("%s:%d: unknown setting %q", path, line, strings.Join(e.Key(), "."))
	}
	var bad *toml.DecodeError
	if errors.As(err, &bad) {
		line, _ := bad.Position()
		if want, ok := wants[strings.Join(bad.Key(), ".")]; ok {
			return fmt.Errorf("%s:%d: invalid value for %q: want %s", path, line, strings.Join(bad.Key(), "."), want)
		}
		return fmt.Errorf("%s:%d: %s", path, line, strings.TrimPrefix(bad.Error(), "toml: "))
	}
	return err
}

// wants says, for each setting, what a valid value looks like.
var wants = map[string]string{
	"close_on_open":    "true or false",
	"targets.close":    "true or false",
	"targets.in_place": "true or false",
	"sort":             `"last_opened" or "name"`,
	"cache_max_age":    `"0" or a positive interval such as "4h" or "30m" (units h, m, s)`,
	"columns":          `a list of "name", "folder", "branch" and "last_opened", including "name"`,
	"row_style":        `"name-folder", "root-path" or "name"`,
	"paths":            `a list of folders, each absolute or starting with ~/, such as ["~/code"]`,
}
