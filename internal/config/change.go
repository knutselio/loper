package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"

	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/wholefile"
)

// CheckTarget returns, for c, Old's lines in the config file at path and the
// lines WriteTarget would write in their place, which for a new target are
// appended, or why c is refused: a value the loader would refuse, a missing
// name, a file that would not load with the change, or an Old that is not in
// the file, or more than once.
func CheckTarget(path, home string, c target.Change) (before, after string, err error) {
	r, err := rewrite(path, home, c)
	return r.before, r.after, err
}

// CheckFile returns why the targets of the config file at path cannot be
// changed, whatever the change: the file does not load, lists its targets
// inline, or is a link to a file that does not exist.
func CheckFile(path, home string) error {
	_, _, err := readForTarget(path, home)
	return err
}

// WriteTarget writes c to the config file at path, creating the file and
// its folder when missing, and returns the targets the file now loads to and
// the index of the target added or changed, or of where the removed one was.
// The file is read again here, and nothing is written when CheckTarget would
// refuse c. A config file that is a symlink stays one: the file it points to
// is rewritten.
func WriteTarget(path, home string, c target.Change) ([]target.Target, int, error) {
	r, err := rewrite(path, home, c)
	if err != nil {
		return nil, 0, err
	}
	if err := wholefile.Write(r.real, r.text); err != nil {
		return nil, 0, fmt.Errorf("could not write the config file: %w", err)
	}
	return r.settings.Targets, r.at, nil
}

// rewritten is the config file with a change made.
type rewritten struct {
	real     string // the file to write: the file a symlink points to, or the file itself
	text     []byte
	before   string // the changed or removed target's lines as they were, or "" when one is added
	after    string // the lines in their place, or appended, or "" when one is removed
	settings Settings
	at       int // the index of the target added or changed, or of where the removed one was
}

// rewrite makes c in the text of the config file at path. A missing file
// counts as empty. A name is taken without the spaces around it. A target
// added goes at the end, after a blank line. A target changed or removed is
// found by its name, key, command, close and in_place, and only the lines
// of its table change, from its [[targets]] line to the line of its last
// value, so comments directly above it stay. A removed table takes the blank lines
// after it along, or, at the end of the file, those before it, so no run of
// blank lines is left.
func rewrite(path, home string, c target.Change) (rewritten, error) {
	var block []byte
	t := targetFile{Name: strings.TrimSpace(c.Name), Key: c.Key, Command: c.Command, Close: c.Close, InPlace: c.InPlace}
	if !c.Remove {
		if err := (&targetName{}).UnmarshalText([]byte(t.Name)); err != nil {
			return rewritten{}, err
		}
		if t.Key != "" {
			if err := (&key{}).UnmarshalText([]byte(t.Key)); err != nil {
				return rewritten{}, err
			}
		}
		if err := (&command{}).UnmarshalText([]byte(t.Command)); err != nil {
			return rewritten{}, err
		}
		// Marshal cannot fail on a struct of strings and booleans.
		block, _ = toml.Marshal(struct {
			Targets []targetFile `toml:"targets"`
		}{[]targetFile{t}})
	}
	real, text, err := readForTarget(path, home)
	if err != nil {
		return rewritten{}, err
	}
	r := rewritten{real: real, after: string(block)}
	if c.Old == nil {
		if len(text) > 0 {
			if !bytes.HasSuffix(text, []byte("\n")) {
				text = append(text, '\n')
			}
			text = append(text, '\n') // a blank line before the block
		}
		r.text = append(text, block...)
		if r.settings, err = parseChanged(path, home, r.text); err != nil {
			return rewritten{}, err
		}
		r.at = len(r.settings.Targets) - 1
		return r, nil
	}
	var was file
	_ = toml.Unmarshal(text, &was) // cannot fail: readForTarget loaded the text
	r.at, err = find(was.Targets, *c.Old)
	if err != nil {
		return rewritten{}, err
	}
	start, end := tableLines(text, r.at)
	r.before = string(text[start:end])
	if c.Lines != "" && r.before != c.Lines {
		return rewritten{}, fmt.Errorf("the lines of target %q changed in the file since they were shown; look at them again", c.Old.Name)
	}
	want := slices.Clone(was.Targets)
	if c.Remove {
		want = slices.Delete(want, r.at, r.at+1)
		start, end = withBlankLines(text, start, end)
		block = nil
	} else {
		want[r.at] = t
		// Checked here, as the loader would name the line of the edited
		// target for a key used by a target after it.
		lines, _ := targetLines(path, text) // cannot fail: readForTarget loaded the text
		for i, other := range was.Targets {
			if i != r.at && t.Key != "" && other.Key == t.Key {
				return rewritten{}, fmt.Errorf("target key %q is already used by the target on line %d", t.Key, lines[i].start)
			}
		}
		if !bytes.HasSuffix(text[:end], []byte("\n")) { // the last line of the file, without a line break
			block = bytes.TrimSuffix(block, []byte("\n"))
		}
		// The line ending is that of the header's line, which always has one.
		if i := start + bytes.IndexByte(text[start:end], '\n'); i > start && text[i-1] == '\r' {
			block = bytes.ReplaceAll(block, []byte("\n"), []byte("\r\n"))
		}
		r.after = string(block)
	}
	r.text = slices.Concat(text[:start], block, text[end:])
	if r.settings, err = parseChanged(path, home, r.text); err != nil {
		return rewritten{}, err
	}
	// The guard: whatever the lines found, the file must load to the same
	// settings and targets but for the one changed.
	var is file
	_ = toml.Unmarshal(r.text, &is) // cannot fail: it just loaded
	if !slices.EqualFunc(is.Targets, want, same) || !reflect.DeepEqual(withoutTargets(is), withoutTargets(was)) {
		return rewritten{}, fmt.Errorf("the change would alter more of the file than target %q; change it there by hand", c.Old.Name)
	}
	return r, nil
}

// parseChanged is parse for text, the config file at path with a change
// made. The line of an error is left out, as it is in the changed text and
// not in the file as it is.
func parseChanged(path, home string, text []byte) (Settings, error) {
	s, err := parse(path, home, text)
	if err != nil {
		if _, reason, ok := strings.Cut(strings.TrimPrefix(err.Error(), path+":"), ": "); ok {
			err = errors.New(reason)
		}
	}
	return s, err
}

// find returns the index in targets of the one old was loaded from, or why
// there is not exactly one.
func find(targets []targetFile, old target.Target) (int, error) {
	at := -1
	for i, t := range targets {
		if !same(t, targetFile{Name: old.Name, Key: old.Key, Command: old.Command, Close: old.Close, InPlace: old.InPlace}) {
			continue
		}
		if at >= 0 {
			return 0, fmt.Errorf("the file has more than one target written as %q; change it there by hand", old.Name)
		}
		at = i
	}
	if at < 0 {
		return 0, fmt.Errorf("target %q is no longer in the file", old.Name)
	}
	return at, nil
}

// tableLines returns where the i-th [[targets]] table of b, a config file
// that loads and has no inline targets, is: from the start of the line of
// its header to the end of the line of its last value, line break included.
// The parser reads the values, so a multi-line string is never taken for a
// header.
func tableLines(b []byte, i int) (start, end int) {
	var p unstable.Parser
	p.Reset(b)
	n := -1
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind == unstable.KeyValue {
			if n == i {
				end = int(e.Raw.Offset + e.Raw.Length)
			}
			continue
		}
		if n == i {
			break // the next table ends it
		}
		if e.Kind == unstable.ArrayTable && keyOf(e) == "targets" {
			n++
			k := e.Key()
			k.Next()
			start = int(k.Node().Raw.Offset)
		}
	}
	start = bytes.LastIndexByte(b[:start], '\n') + 1
	if j := bytes.IndexByte(b[end:], '\n'); j >= 0 {
		return start, end + j + 1
	}
	return start, len(b)
}

// withBlankLines widens start and end, the lines of a table in b, to take
// the blank lines after it or, when only blank lines follow, those before it.
func withBlankLines(b []byte, start, end int) (int, int) {
	blank := func(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }
	for end < len(b) {
		next := len(b)
		if j := bytes.IndexByte(b[end:], '\n'); j >= 0 {
			next = end + j + 1
		}
		if !blank(b[end:next]) {
			return start, end
		}
		end = next
	}
	for start > 0 {
		prev := bytes.LastIndexByte(b[:start-1], '\n') + 1
		if !blank(b[prev:start]) {
			break
		}
		start = prev
	}
	return start, end
}

// same reports whether a and b are the same target, close and in_place
// included.
func same(a, b targetFile) bool {
	return reflect.DeepEqual(a, b)
}

// withoutTargets returns f without its targets, to compare the rest.
func withoutTargets(f file) file {
	f.Targets = nil
	return f
}

// readForTarget returns the file to write for the config file at path and
// its text, or why its targets cannot be changed. A missing file counts as
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
	// The write renames a new file over it, which a file the owner cannot
	// write would not stop.
	if info, err := os.Stat(real); err == nil && info.Mode().Perm()&0o200 == 0 {
		return "", nil, fmt.Errorf("%s is read-only; make it writable to change its targets here", real)
	}
	// The file must load as it is, so an error once the change is made is
	// the change's.
	if _, err := parse(path, home, text); err != nil {
		return "", nil, err
	}
	if inlineTargets(text) {
		return "", nil, errors.New("the file lists its targets inline, as targets = [...]; edit them there by hand")
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
