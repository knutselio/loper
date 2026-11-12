// Package config loads and validates the config file and applies defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"

	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/wholefile"
	"github.com/knutselio/loper/internal/xdg"
)

// Settings is the complete configuration, with every default applied.
type Settings struct {
	File        string          // the config file, read or, when missing, where it would be
	Paths       []string        // absolute folders to scan, or repos to list directly
	Targets     []target.Target // in the order they are written, perhaps none
	CloseOnOpen bool            // quit after a successful open
	SortByName  bool            // start sorted by name rather than by last opened
	Columns     []string        // the columns shown, in order; "name" is one of them
	RowStyle    string          // how name and folder are drawn: "name-folder", "root-path" or "name"
	// CacheMaxAge is how old the cache may be before a launch scans again;
	// 0 means every launch scans.
	CacheMaxAge time.Duration
}

// file is the config file as written; a nil field was left out.
type file struct {
	Paths       []string     `toml:"paths"`
	Targets     []targetFile `toml:"targets"`
	CloseOnOpen bool         `toml:"close_on_open"`
	Sort        string       `toml:"sort"`
	CacheMaxAge string       `toml:"cache_max_age"`
	Columns     []string     `toml:"columns"`
	RowStyle    string       `toml:"row_style"`
}

type targetFile struct {
	Name    string `toml:"name"`
	Key     string `toml:"key,omitempty"` // omitempty for AddTarget, which leaves out a key not given
	Command string `toml:"command"`
}

// checked is file with each value checking itself while it is decoded, so
// that a bad value is reported on its own line. It is decoded only after file
// succeeds: the library reports a wrong type without its line when the target
// checks itself.
type checked struct {
	Paths       []folder        `toml:"paths"`
	Targets     []checkedTarget `toml:"targets"`
	CloseOnOpen bool            `toml:"close_on_open"`
	Sort        sortName        `toml:"sort"`
	CacheMaxAge maxAge          `toml:"cache_max_age"`
	Columns     []column        `toml:"columns"`
	RowStyle    rowStyle        `toml:"row_style"`
}

type checkedTarget struct {
	Name    string  `toml:"name"`
	Key     key     `toml:"key"`
	Command command `toml:"command"`
}

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
	if err == nil && !slices.ContainsFunc(words, func(w string) bool { return strings.Contains(w, "{path}") }) {
		err = errors.New("has no {path}")
	}
	if err != nil {
		return fmt.Errorf("target command %q %w", b, err)
	}
	return nil
}

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
	if strings.Contains("jkgG/sr?qa", k) {
		return fmt.Errorf("target key %q is one of loper's own keys", k)
	}
	return nil
}

// Load reads the config file at path, or at the standard location when path
// is "". A missing file at the standard location means the defaults; a
// missing file named on the command line is an error. A leading ~ in a
// setting stands for home.
func Load(path, home string) (Settings, error) {
	standard := path == ""
	path = Path(path, home)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !standard {
		return Settings{}, fmt.Errorf("config file %s does not exist", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, err
	}
	return parse(path, home, b)
}

// parse is Load for b, the text of the config file at path.
func parse(path, home string, b []byte) (Settings, error) {
	var f file
	for _, v := range []any{&f, &checked{}} {
		if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(v); err != nil {
			return Settings{}, describe(path, err)
		}
	}
	if f.Paths == nil {
		f.Paths = []string{"~/code"}
	}
	if err := checkTargets(path, b, f.Targets); err != nil {
		return Settings{}, err
	}
	if err := checkColumns(path, b, f.Columns); err != nil {
		return Settings{}, err
	}
	if f.Columns == nil {
		f.Columns = []string{"name", "folder", "branch", "last_opened"}
	}
	if f.RowStyle == "" {
		f.RowStyle = "name-folder"
	}
	s := Settings{File: path, CloseOnOpen: f.CloseOnOpen, SortByName: f.Sort == "name", Columns: f.Columns, RowStyle: f.RowStyle}
	if f.CacheMaxAge != "" {
		s.CacheMaxAge, _ = parseMaxAge(f.CacheMaxAge) // cannot fail: the value was checked while decoding
	}
	for _, t := range f.Targets {
		args, _ := split(t.Command) // cannot fail: the command was checked while decoding
		s.Targets = append(s.Targets, target.Target{Name: t.Name, Key: t.Key, Args: args})
	}
	for _, p := range f.Paths {
		s.Paths = append(s.Paths, expand(p, home))
	}
	return s, nil
}

// CheckTarget returns the [[targets]] block that AddTarget would append to
// the config file at path for a target with name, key k (which may be "")
// and command, or why the target is refused: a value the loader would refuse,
// a missing name, or a file that would not load with the block appended.
func CheckTarget(path, home, name, k, cmd string) (string, error) {
	_, _, block, _, err := withTarget(path, home, name, k, cmd)
	if err != nil {
		return "", err
	}
	return block, nil
}

// CheckFile returns why a target cannot be added to the config file at path,
// whatever the target: the file does not load, lists its targets inline, or
// is a link to a file that does not exist.
func CheckFile(path, home string) error {
	_, _, err := readForTarget(path, home)
	return err
}

// AddTarget appends the block CheckTarget returns to the config file at
// path, creating the file and its folder when missing, and returns the
// targets the file now loads to, the new one last. The file is read again
// here, and nothing is written when the result would not load. A config file
// that is a symlink stays one: the file it points to is rewritten.
func AddTarget(path, home, name, k, cmd string) ([]target.Target, error) {
	real, text, _, s, err := withTarget(path, home, name, k, cmd)
	if err != nil {
		return nil, err
	}
	if err := wholefile.Write(real, text); err != nil {
		return nil, fmt.Errorf("could not write the config file: %w", err)
	}
	return s.Targets, nil
}

// withTarget returns the file to write for the config file at path, which
// is the file it points to when it is a symlink, the text of that file with
// a block for the target appended, the block, and the settings that text
// loads to. A missing file counts as empty. The name is taken without the
// spaces around it.
func withTarget(path, home, name, k, cmd string) (string, []byte, string, Settings, error) {
	fail := func(err error) (string, []byte, string, Settings, error) { return "", nil, "", Settings{}, err }
	if name = strings.TrimSpace(name); name == "" {
		return fail(errors.New("a target needs a name"))
	}
	if k != "" {
		if err := (&key{}).UnmarshalText([]byte(k)); err != nil {
			return fail(err)
		}
	}
	if err := (&command{}).UnmarshalText([]byte(cmd)); err != nil {
		return fail(err)
	}
	real, text, err := readForTarget(path, home)
	if err != nil {
		return fail(err)
	}
	// Marshal cannot fail on a struct of strings.
	block, _ := toml.Marshal(struct {
		Targets []targetFile `toml:"targets"`
	}{[]targetFile{{Name: name, Key: k, Command: cmd}}})
	if len(text) > 0 {
		if !bytes.HasSuffix(text, []byte("\n")) {
			text = append(text, '\n')
		}
		text = append(text, '\n') // a blank line before the block
	}
	text = append(text, block...)
	s, err := parse(path, home, text)
	if err != nil {
		// Its line is in the block, past the end of the file as it is, so
		// only the reason is given.
		if _, reason, ok := strings.Cut(strings.TrimPrefix(err.Error(), path+":"), ": "); ok {
			err = errors.New(reason)
		}
		return fail(err)
	}
	return real, text, string(block), s, nil
}

// readForTarget returns the file to write for the config file at path and
// its text, or why a target cannot be added to it. A missing file counts as
// empty.
func readForTarget(path, home string) (string, []byte, error) {
	real, err := realFile(path)
	if err != nil {
		return "", nil, err
	}
	text, err := os.ReadFile(real)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	// The file must load as it is, so an error after the block is appended
	// is the block's.
	if _, err := parse(path, home, text); err != nil {
		return "", nil, err
	}
	if inlineTargets(text) {
		return "", nil, errors.New("the file lists its targets inline, as targets = [...]; add a target there by hand")
	}
	return real, text, nil
}

// realFile returns the file to write for the config file at path: the file
// it points to when it is a symlink, since a write renames over its path and
// would replace a link with a plain file, or path itself. A link to nothing is
// refused, as writing through it would do just that.
func realFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return path, nil // a missing file is created; any other error shows when it is read
	}
	real, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		dest, _ := os.Readlink(path) // cannot fail: Lstat just found a link
		return "", fmt.Errorf("%s is a link to %s, which does not exist", path, dest)
	}
	return real, err
}

// inlineTargets reports whether b, a config file that loads, lists its
// targets inline, as targets = [...], to which a [[targets]] table cannot be
// added.
func inlineTargets(b []byte) bool {
	var p unstable.Parser
	p.Reset(b)
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind != unstable.KeyValue {
			return false // the root table, where an inline list must be, has ended
		}
		if keyOf(e) == "targets" {
			return true
		}
	}
	return false
}

// checkTargets reports what the per-value checks cannot see: a target with
// no command, and a key used by two targets. b is the file at path.
func checkTargets(path string, b []byte, targets []targetFile) error {
	lines, err := targetLines(path, b)
	if err != nil {
		return err
	}
	used := map[string]int{} // key → the line of the target that has it
	for i, t := range targets {
		if t.Command == "" {
			return fmt.Errorf("%s:%d: target %q has no command; it needs one with {path}", path, lines[i].start, t.Name)
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

// Path returns the config file Load reads: path, as given to --config, or
// the standard location when path is "".
func Path(path, home string) string {
	if path != "" {
		return path
	}
	return filepath.Join(xdg.Dir("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "loper", "config.toml")
}

// expand replaces a leading ~ with the home folder.
func expand(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
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
	"close_on_open": "true or false",
	"sort":          `"last_opened" or "name"`,
	"cache_max_age": `"0" or a positive interval such as "4h" or "30m" (units h, m, s)`,
	"columns":       `a list of "name", "folder", "branch" and "last_opened", including "name"`,
	"row_style":     `"name-folder", "root-path" or "name"`,
	"paths":         `a list of folders, each absolute or starting with ~/, such as ["~/code"]`,
}
