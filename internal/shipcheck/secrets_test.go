package shipcheck

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A warning about the gitignored files an image carries names where they
// are, most files first, so the place to add to an ignore file is what the
// reader sees, not a list of thousands of paths.
func TestPlacesOfNamesTopLevelPlaces(t *testing.T) {
	var paths []string
	for i := range 5509 {
		paths = append(paths, fmt.Sprintf(".claude/worktrees/agent-%d/README.md", i))
	}
	paths = append(paths, ".astro/config.local.lock")
	assert.Equal(t, ".claude/ and .astro/", placesOf(paths))

	assert.Equal(t, "notes.txt", placesOf([]string{"notes.txt"}), "a file at the root is its own place")
	assert.Equal(t, "a/, b/ and c.txt", placesOf([]string{"b/1", "a/1", "a/2", "c.txt", "b/2", "a/3"}),
		"most files first, then by name")
	assert.Equal(t, "a/, b/, c/ and 1 more place", placesOf([]string{"a/1", "a/2", "a/3", "b/1", "b/2", "c/1", "d/1"}))
	assert.Equal(t, "a/, b/, c/ and 2 more places", placesOf([]string{"a/1", "b/1", "c/1", "d/1", "e/1"}))
}

func TestCountFiles(t *testing.T) {
	for n, want := range map[int]string{
		1:       "1 file",
		2:       "2 files",
		999:     "999 files",
		5510:    "5,510 files",
		1234567: "1,234,567 files",
	} {
		assert.Equal(t, want, countFiles(n))
	}
}
