package manifest

import (
	"errors"
	"fmt"
	"testing"
)

// One manifest, parsed many times, reports its problems in one order.
//
// Links are decoded out of a map, so the order they are found in is random on
// every run, and Load sorts to hide that. Sorting by key alone very nearly
// settles it — no two rules currently address the same key — but sort.Slice is
// not stable, so the day a second rule lands on some key the pair starts
// coming back in either order: error text that differs run to run, and a
// silent flake in every table here that asserts an ordered list.
//
// Repeated rather than run once because a single run of a randomized order
// proves nothing; 200 parses of 24 links is enough that an unstable pair shows
// up rather than hides.
func TestProblemOrderIsStable(t *testing.T) {
	content := "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\n"
	for i := range 24 {
		content += fmt.Sprintf("\n[tool.astro.deployments.l%02d]\ntarget = \"mwaa\"\ndeployment = \"d\"\n", i)
	}
	path := write(t, content)

	var first []Problem
	for range 200 {
		_, err := Load(path)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("want a *ValidationError, got %T: %v", err, err)
		}
		if first == nil {
			first = ve.Problems
			if len(first) < 2 {
				t.Fatalf("fixture reports %d problems, too few to order", len(first))
			}
			continue
		}
		for i := range ve.Problems {
			if ve.Problems[i] != first[i] {
				t.Fatalf("problem %d came back as %+v, first run had %+v", i, ve.Problems[i], first[i])
			}
		}
	}
}

// Two problems under one key still come back in a fixed order.
//
// TestProblemOrderIsStable can only exercise the keys the rules happen to
// produce, and today none of them collide, so it would keep passing if the
// tiebreak were deleted. This drives the comparison directly with a pair that
// differs only in code — the case the tiebreak exists for and the one no
// fixture can currently reach.
func TestProblemsUnderOneKeyOrderByCode(t *testing.T) {
	p := &parser{problems: []Problem{
		{Code: CodeURLNotHTTP, Key: "same.key", Reason: "b"},
		{Code: CodeEmptyString, Key: "same.key", Reason: "a"},
		{Code: CodeRequired, Key: "aaa.key", Reason: "c"},
	}}
	p.sortProblems()

	want := []ProblemCode{CodeRequired, CodeEmptyString, CodeURLNotHTTP}
	for i, code := range want {
		if p.problems[i].Code != code {
			t.Errorf("problem %d = %q, want %q", i, p.problems[i].Code, code)
		}
	}
}
