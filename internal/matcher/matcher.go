// Package matcher ranks rows against a fuzzy search query.
package matcher

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// fzf's matcher scores word boundaries from tables that Init fills; without
// it every boundary bonus is zero.
func init() { algo.Init("default") }

// Row is the text of a row's searchable fields, as the list shows them.
type Row struct{ Name, Folder, Branch string }

// Match is a row that matches the query: its index among the rows given and,
// for each field that matches, the positions of the matched characters,
// counted in characters (runes) and in ascending order.
type Match struct {
	Row                  int
	Name, Folder, Branch []int
}

// Find returns the rows that match every space-separated term of query in
// one of their fields, best match first. A row with a term matching its name
// ranks above the rows matching only in the folder or branch, then the sum
// of each term's best score decides, and equal matches keep the order of
// rows. A term ignores case unless it contains a capital letter, and ignores
// accents unless it contains one.
func Find(query string, rows []Row) []Match {
	var terms []term
	for _, t := range strings.Fields(query) {
		terms = append(terms, newTerm(t))
	}
	type ranked struct {
		Match
		inName bool
		score  int
	}
	var found []ranked
	for i, row := range rows {
		r := ranked{Match: Match{Row: i}}
		fields := []struct {
			text string
			pos  *[]int
		}{{row.Name, &r.Name}, {row.Folder, &r.Folder}, {row.Branch, &r.Branch}}
		matched := true
		for _, t := range terms {
			best, ok := 0, false
			for f, field := range fields {
				pos, score, hit := t.find(field.text)
				if !hit {
					continue
				}
				*field.pos = slices.Compact(slices.Sorted(slices.Values(append(*field.pos, pos...))))
				r.inName = r.inName || f == 0
				best, ok = max(best, score), true
			}
			if !ok {
				matched = false
				break
			}
			r.score += best
		}
		if matched {
			found = append(found, r)
		}
	}
	slices.SortStableFunc(found, func(a, b ranked) int {
		if a.inName != b.inName {
			if a.inName {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.score, a.score)
	})
	matches := make([]Match, len(found))
	for i, r := range found {
		matches[i] = r.Match
	}
	return matches
}

// term is one space-separated part of a query, prepared as fzf expects: in
// lowercase when it ignores case.
type term struct {
	pattern              []rune
	sensitive, normalize bool
}

func newTerm(text string) term {
	lower := []rune(strings.ToLower(text))
	t := term{
		pattern:   []rune(text),
		sensitive: strings.ContainsFunc(text, unicode.IsUpper),
		// Like fzf, a term ignores accents only when it has none itself.
		normalize: slices.Equal(algo.NormalizeRunes(lower), lower),
	}
	if !t.sensitive {
		t.pattern = lower
	}
	return t
}

// find matches the term in text and returns the matched positions in
// ascending order and the match's score.
func (t term) find(text string) ([]int, int, bool) {
	chars := util.ToChars([]byte(text))
	res, pos := algo.FuzzyMatchV2(t.sensitive, t.normalize, true, &chars, t.pattern, true, nil)
	if res.Start < 0 {
		return nil, 0, false
	}
	return slices.Sorted(slices.Values(*pos)), res.Score, true
}
