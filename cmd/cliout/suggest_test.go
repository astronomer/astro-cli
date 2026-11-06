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
	assert.Equal(t, 0, EditDistance("output", "output"))
	assert.Equal(t, 2, EditDistance("outptu", "output"))
	assert.Equal(t, 1, EditDistance("sizé", "size"), "an accented letter is one edit")
	assert.Equal(t, 3, EditDistance("", "abc"))
}
