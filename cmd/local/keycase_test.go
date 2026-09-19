package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A published key starting with a capital is a Go field name that reached
// the wire because somebody forgot a tag.
//
// It is never a choice. encoding/json falls back to the field's own name, so
// `Section string` publishes "Section" while the tagged field beside it
// publishes "source_note" — which is how start-missing-env came to mix the
// two, in the payload whose own doc says it is structured for a coding agent
// to act on. Nothing noticed until the shapes were pinned and somebody read
// the file.
//
// This checks the class rather than that one instance, because the next
// untagged field is the same bug and there is no reason to find it by eye
// twice.
func TestNoPublishedKeyStartsWithACapital(t *testing.T) {
	var offenders []string
	forEachPinnedKey(t, func(payload, key string) {
		if key[:1] == strings.ToUpper(key[:1]) && key[:1] != strings.ToLower(key[:1]) {
			offenders = append(offenders, payload+": "+key)
		}
	})
	sort.Strings(offenders)

	assert.Empty(t, offenders,
		"these keys are Go field names that reached the wire untagged.\n"+
			"Add a json tag — snake_case, which is what 75 of the 79 keys here\n"+
			"already use — and run `make update-schemas`.")
}

// forEachPinnedKey walks every key of every golden, at every depth.
func forEachPinnedKey(t *testing.T, fn func(payload, key string)) {
	t.Helper()
	dir := filepath.Join("testdata", "schema")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var walk func(payload string, v any)
	walk = func(payload string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, inner := range x {
				fn(payload, k)
				walk(payload, inner)
			}
		case []any:
			for _, inner := range x {
				walk(payload, inner)
			}
		}
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, rerr)
		var shape any
		require.NoError(t, json.Unmarshal(raw, &shape))
		walk(e.Name(), shape)
	}
}
