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
		{Name: "backend", Folder: "~/code", Branch: "main"},
		{Name: "dotfiles", Folder: "~", Branch: "main"},
		{Name: "aycapi", Folder: "~/code/work", Branch: "fix/login"},
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"bnd", []string{"backend"}},                    // the name
		{"wrk", []string{"aycapi"}},                     // the folder
		{"login", []string{"aycapi"}},                   // the branch
		{"zzz", nil},                                    // nothing
		{"dotmain", nil},                                // one term across two fields
		{"", []string{"backend", "dotfiles", "aycapi"}}, // an empty query matches every row, in order
		{"dot main", []string{"dotfiles"}},              // each term in its own field
		{"wrk login", []string{"aycapi"}},
		{"code main", []string{"backend"}},
		{"bnd login", nil}, // every term must match
		{"  bnd   ", []string{"backend"}},
		{"   ", []string{"backend", "dotfiles", "aycapi"}},
	} {
		if got := names(rows, matcher.Find(c.query, rows)); !slices.Equal(got, c.want) {
			t.Errorf("%q matches %q, want %q", c.query, got, c.want)
		}
	}
}

func TestAMatchInTheNameRanksAboveAnExactMatchInTheFolderOrBranch(t *testing.T) {
	rows := []matcher.Row{
		{Name: "api", Folder: "~/code/tools", Branch: "main"},
		{Name: "web", Folder: "~/code", Branch: "tools"},
		{Name: "t-o-o-l-s", Folder: "~/code", Branch: "main"}, // scattered, so it scores below the others
	}

	got := names(rows, matcher.Find("tools", rows))
	if len(got) != 3 || got[0] != "t-o-o-l-s" {
		t.Errorf("ranked %q, want t-o-o-l-s first of all three", got)
	}
}

func TestAWordBoundaryRanksAboveTheMiddleOfAWord(t *testing.T) {
	rows := []matcher.Row{
		{Name: "rapido", Folder: "~", Branch: "main"},
		{Name: "my-api", Folder: "~", Branch: "main"},
	}

	if got, want := names(rows, matcher.Find("api", rows)), []string{"my-api", "rapido"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q", got, want)
	}
}

func TestTiesKeepTheOrderOfTheRows(t *testing.T) {
	for _, field := range []string{"name", "folder", "branch"} {
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
			r := matcher.Row{Name: other, Folder: other, Branch: other}
			switch field {
			case "name":
				r.Name = text
			case "folder":
				r.Folder = text
			case "branch":
				r.Branch = text
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
		{Name: "Backend", Folder: "~", Branch: "main"},
		{Name: "backend", Folder: "~", Branch: "main"},
		{Name: "bB", Folder: "~", Branch: "main"},
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
	rows := []matcher.Row{{Name: "main-app", Folder: "~/köde/main", Branch: "mainline"}}

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
	if want := []int{0, 1, 2, 3}; !slices.Equal(m.Branch, want) {
		t.Errorf("branch positions %v, want %v", m.Branch, want)
	}
}

func TestAFieldThatDoesNotMatchHasNoPositions(t *testing.T) {
	rows := []matcher.Row{{Name: "web", Folder: "~/code", Branch: "fix/login"}}

	got := matcher.Find("login", rows)

	if len(got) != 1 || got[0].Name != nil || got[0].Folder != nil || !slices.Equal(got[0].Branch, []int{4, 5, 6, 7, 8}) {
		t.Errorf("got %+v, want only the branch's positions 4 to 8", got)
	}
}

func TestARowWithANameMatchingOneTermRanksAboveRowsMatchingOnlyInTheFolderOrBranch(t *testing.T) {
	rows := []matcher.Row{
		{Name: "web", Folder: "~/api", Branch: "main"},
		{Name: "api", Folder: "~/code", Branch: "main"},
	}

	if got, want := names(rows, matcher.Find("main api", rows)), []string{"api", "web"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q", got, want)
	}
}

func TestMoreTermsMatchedWellRankHigher(t *testing.T) {
	rows := []matcher.Row{
		{Name: "w-e-b", Folder: "~/a-p-i", Branch: "main"},
		{Name: "web", Folder: "~/api", Branch: "main"},
	}

	if got, want := names(rows, matcher.Find("web api", rows)), []string{"web", "w-e-b"}; !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q: the scores of both terms count", got, want)
	}
}

func TestThePositionsOfAFieldAreThoseOfEveryTermThatMatchedIt(t *testing.T) {
	rows := []matcher.Row{{Name: "api-web", Folder: "~/code", Branch: "main"}}

	got := matcher.Find("web api main", rows)

	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if want := []int{0, 1, 2, 4, 5, 6}; !slices.Equal(got[0].Name, want) {
		t.Errorf("name positions %v, want %v", got[0].Name, want)
	}
	if want := []int{0, 1, 2, 3}; !slices.Equal(got[0].Branch, want) {
		t.Errorf("branch positions %v, want %v", got[0].Branch, want)
	}
}

func TestEachTermIsCaseSensitiveOnlyWhenItHasACapital(t *testing.T) {
	rows := []matcher.Row{
		{Name: "Backend", Folder: "~", Branch: "MAIN"},
		{Name: "backend", Folder: "~", Branch: "main"},
	}

	if got, want := names(rows, matcher.Find("Back main", rows)), []string{"Backend"}; !slices.Equal(got, want) {
		t.Errorf("matches %q, want %q", got, want)
	}
}

func TestATermWithoutAccentsIgnoresThemAndOneWithAccentsDoesNot(t *testing.T) {
	rows := []matcher.Row{
		{Name: "~café", Folder: "~", Branch: "main"},
		{Name: "cafe", Folder: "~", Branch: "main"},
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
