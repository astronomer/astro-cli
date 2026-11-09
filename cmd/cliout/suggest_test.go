package cliout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClosest(t *testing.T) {
	cases := map[string]struct {
		name       string
		candidates []string
		want       []string
	}{
		"one edit":                   {"workspac", []string{"workspace", "deployment"}, []string{"workspace"}},
		"ignoring case":              {"WorkSpace", []string{"workspace"}, []string{"workspace"}},
		"a prefix counts as close":   {"deploy", []string{"deployment", "workspace"}, []string{"deployment"}},
		"every one at the nearest":   {"al", []string{"all", "ali", "a"}, []string{"all", "ali", "a"}},
		"the nearest wins":           {"alll", []string{"ali", "all"}, []string{"all"}},
		"one past the distance":      {"xyz", []string{"abc"}, nil},
		"an empty name matches none": {"", []string{"all"}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, Closest(c.name, c.candidates, 2))
		})
	}
}

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
		why  string
	}{
		{"output", "output", 0, "the same"},
		{"", "abc", 3, "insertions"},
		{"workspac", "workspace", 1, "a deletion"},
		{"bogs", "tags", 2, "two substitutions"},
		{"sizé", "size", 1, "an accented letter is one edit"},
		{"outptu", "output", 1, "swapped neighbors at the end"},
		{"tgas", "tags", 1, "swapped neighbors"},
		{"froce", "force", 1, "swapped neighbors mid-word"},
		{"ab", "ba", 1, "a whole word swapped"},
		{"éa", "aé", 1, "a swap is by character, not byte"},
		{"froce", "forec", 2, "two separate swaps"},
		{"ca", "abc", 3, "optimal string alignment edits no substring twice"},
	} {
		assert.Equal(t, c.want, EditDistance(c.a, c.b), "%s: %q to %q", c.why, c.a, c.b)
		assert.Equal(t, c.want, EditDistance(c.b, c.a), "%s, reversed: %q to %q", c.why, c.b, c.a)
	}
}
