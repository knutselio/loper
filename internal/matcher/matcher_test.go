package matcher_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/knutselio/loper/internal/matcher"
)

// names returns the names of the matching rows, in rank order.
func names(rows []matcher.Row, matches []matcher.Match) []string {
	var got []string
	for _, m := range matches {
		got = append(got, rows[m.Row].Name)
	}
	return got
}

func TestOnlyRowsWithAFieldMatchingEachTermOfTheQueryAreReturned(t *testing.T) {
	rows := []matcher.Row{
		{Name: "backend", Folder: "~/code"},
		{Name: "dotfiles", Folder: "~"},
		{Name: "aycapi", Folder: "~/code/work"},
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"bnd", []string{"backend"}},                    // the name
		{"wrk", []string{"aycapi"}},                     // the folder
		{"zzz", nil},                                    // nothing
		{"bndcode", nil},                                // one term across two fields
		{"", []string{"backend", "dotfiles", "aycapi"}}, // an empty query matches every row, in order
		{"ayc wrk", []string{"aycapi"}},                 // each term in its own field
		{"code bnd", []string{"backend"}},
		{"bnd wrk", nil}, // every term must match
		{"  bnd   ", []string{"backend"}},
		{"   ", []string{"backend", "dotfiles", "aycapi"}},
	} {
		if got := names(rows, matcher.Find(c.query, rows)); !slices.Equal(got, c.want) {
			t.Errorf("%q matches %q, want %q", c.query, got, c.want)
		}
	}
}

func TestAMatchInTheNameRanksAboveAnExactMatchInTheFolder(t *testing.T) {
	rows := []matcher.Row{
		{Name: "api", Folder: "~/code/tools"},
		{Name: "t-o-o-l-s", Folder: "~/code"}, // scattered, so it scores below the other
	}

	got := names(rows, matcher.Find("tools", rows))
	if len(got) != 2 || got[0] != "t-o-o-l-s" {
		t.Errorf("ranked %q, want t-o-o-l-s first of both", got)
	}
}

func TestAWordBoundaryRanksAboveTheMiddleOfAWord(t *testing.T) {
	rows := []matcher.Row{
		{Name: "rapido", Folder: "~"},
		{Name: "my-api", Folder: "~"},
	}

	if got, want := names(rows, matcher.Find("api", rows)), []string{"my-api", "rapido"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q", got, want)
	}
}

func TestTiesKeepTheOrderOfTheRows(t *testing.T) {
	for _, field := range []string{"name", "folder"} {
		// Even rows match exactly and odd rows scattered, so the rows tie in
		// two groups; enough of them that an unstable sort would reorder.
		var rows []matcher.Row
		var evens, odds []int
		for i := range 40 {
			text, other := "work", fmt.Sprintf("x%02d", i)
			if i%2 == 1 {
				text = "w-o-r-k"
				odds = append(odds, i)
			} else {
				evens = append(evens, i)
			}
			r := matcher.Row{Name: other, Folder: other}
			switch field {
			case "name":
				r.Name = text
			case "folder":
				r.Folder = text
			}
			rows = append(rows, r)
		}

		var got []int
		for _, m := range matcher.Find("work", rows) {
			got = append(got, m.Row)
		}
		if want := append(evens, odds...); !slices.Equal(got, want) {
			t.Errorf("equal matches in the %s ranked as rows %v, want %v", field, got, want)
		}
	}
}

func TestALowercaseQueryIgnoresCaseAndOneWithACapitalDoesNot(t *testing.T) {
	rows := []matcher.Row{
		{Name: "Backend", Folder: "~"},
		{Name: "backend", Folder: "~"},
		{Name: "bB", Folder: "~"},
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"back", []string{"Backend", "backend"}},
		{"Back", []string{"Backend"}},
		{"bB", []string{"bB"}},
		{"B", []string{"Backend", "bB"}},
	} {
		if got := names(rows, matcher.Find(c.query, rows)); !slices.Equal(got, c.want) {
			t.Errorf("%q matches %q, want %q", c.query, got, c.want)
		}
	}
}

func TestMatchedPositionsAreTheCharactersOfEachMatchingField(t *testing.T) {
	rows := []matcher.Row{{Name: "main-app", Folder: "~/köde/main"}}

	got := matcher.Find("main", rows)

	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	m := got[0]
	if want := []int{0, 1, 2, 3}; !slices.Equal(m.Name, want) {
		t.Errorf("name positions %v, want %v", m.Name, want)
	}
	if want := []int{7, 8, 9, 10}; !slices.Equal(m.Folder, want) { // characters, not bytes: ö is two bytes
		t.Errorf("folder positions %v, want %v", m.Folder, want)
	}
}

func TestAFieldThatDoesNotMatchHasNoPositions(t *testing.T) {
	rows := []matcher.Row{{Name: "web", Folder: "~/code/login"}}

	got := matcher.Find("login", rows)

	if len(got) != 1 || got[0].Name != nil || !slices.Equal(got[0].Folder, []int{7, 8, 9, 10, 11}) {
		t.Errorf("got %+v, want only the folder's positions 7 to 11", got)
	}
}

func TestARowWithANameMatchingOneTermRanksAboveRowsMatchingOnlyInTheFolder(t *testing.T) {
	rows := []matcher.Row{
		{Name: "web", Folder: "~/api/main"},
		{Name: "api", Folder: "~/code/main"},
	}

	if got, want := names(rows, matcher.Find("main api", rows)), []string{"api", "web"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q", got, want)
	}
}

func TestMoreTermsMatchedWellRankHigher(t *testing.T) {
	rows := []matcher.Row{
		{Name: "w-e-b", Folder: "~/a-p-i"},
		{Name: "web", Folder: "~/api"},
	}

	if got, want := names(rows, matcher.Find("web api", rows)), []string{"web", "w-e-b"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q: the scores of both terms count", got, want)
	}
}

func TestThePositionsOfAFieldAreThoseOfEveryTermThatMatchedIt(t *testing.T) {
	rows := []matcher.Row{{Name: "api-web", Folder: "~/code"}}

	got := matcher.Find("web api code", rows)

	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if want := []int{0, 1, 2, 4, 5, 6}; !slices.Equal(got[0].Name, want) {
		t.Errorf("name positions %v, want %v", got[0].Name, want)
	}
	if want := []int{2, 3, 4, 5}; !slices.Equal(got[0].Folder, want) {
		t.Errorf("folder positions %v, want %v", got[0].Folder, want)
	}
}

func TestEachTermIsCaseSensitiveOnlyWhenItHasACapital(t *testing.T) {
	rows := []matcher.Row{
		{Name: "Backend", Folder: "~/MAIN"},
		{Name: "backend", Folder: "~/main"},
	}

	if got, want := names(rows, matcher.Find("Back main", rows)), []string{"Backend"}; !slices.Equal(got, want) {
		t.Errorf("matches %q, want %q", got, want)
	}
}

func TestATermWithoutAccentsIgnoresThemAndOneWithAccentsDoesNot(t *testing.T) {
	rows := []matcher.Row{
		{Name: "~café", Folder: "~"},
		{Name: "cafe", Folder: "~"},
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"cafe", []string{"cafe", "~café"}},
		{"café", []string{"~café"}},
	} {
		if got := names(rows, matcher.Find(c.query, rows)); !slices.Equal(got, c.want) {
			t.Errorf("%q matches %q, want %q", c.query, got, c.want)
		}
	}
	if got := matcher.Find("cafe", rows[:1]); len(got) != 1 || !slices.Equal(got[0].Name, []int{1, 2, 3, 4}) {
		t.Errorf("cafe in ~café: got %+v, want name positions 1 to 4", got)
	}
}
