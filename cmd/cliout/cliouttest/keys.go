package cliouttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// snakeCase is a published key: lower case, digits and underscores, starting
// with a letter.
var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// KeyProblems lists every key, at every depth, of every golden under dir that
// is not snake_case, as "golden.json: key". A key with a capital in it is
// usually a Go field name that reached the wire because somebody forgot a
// tag: encoding/json falls back to the field's own name.
//
// legacy names the keys a tree published before the rule, as "golden.json:
// key", with the reason each stays; changing one now would break whoever
// reads it. An entry that no longer matches a key is reported too, so the
// list cannot outlive what it excuses. hasCases is whether the tree pins
// anything: a missing directory is read as goldenEntries reads it.
func KeyProblems(t testing.TB, dir string, hasCases bool, legacy map[string]string) []string {
	t.Helper()
	entries := goldenEntries(t, dir, hasCases)

	seen := map[string]bool{}
	var walk func(golden string, v any)
	walk = func(golden string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, inner := range x {
				if !snakeCase.MatchString(k) {
					seen[golden+": "+k] = true
				}
				walk(golden, inner)
			}
		case []any:
			for _, inner := range x {
				walk(golden, inner)
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

	var out []string
	for key := range seen {
		if legacy[key] == "" {
			out = append(out, key+" is not snake_case; give the field a json tag that is, and run `make update-schemas`")
		}
	}
	for key := range legacy {
		if !seen[key] {
			out = append(out, key+" is excused as a legacy key, and no golden publishes it any more; drop the entry")
		}
	}
	sort.Strings(out)
	return out
}
