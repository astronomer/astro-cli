//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The goldens in cmd/local/testdata/schema pin the shape of every `--output
// json` payload, but they pin the Go type — they marshal a struct and
// compare. Nothing in them says a command still emits that struct. Swap the
// type a command hands to Emit and every golden stays green.
//
// This binds the two together for `astro init`, the payload most likely to
// be read by something outside this repository: run the real binary and
// check it publishes no key the contract has not pinned.
//
// Keys the command emits, not keys it must emit. A greenfield init leaves
// several `omitempty` fields out, and requiring them would make this a test
// of one fixture's content rather than of the contract. An unpinned key is
// the failure worth catching: it means a field was renamed, or added without
// anybody deciding it was public.
//
// Reading the golden across the module boundary on purpose. Restating the
// key list here would give two lists to keep in step, and the golden is not
// the implementation — it is a checked-in snapshot somebody reviewed.
func TestInitPublishesNoUnpinnedJSONKeys(t *testing.T) {
	tier(t, 0)

	pinned := pinnedKeys(t, "init")
	p := newProject(t)

	var emitted map[string]any
	p.run("init", "--name", "schema-check", "--output", "json").
		requireSuccess().
		requireJSON(&emitted)

	if len(emitted) == 0 {
		t.Fatal("init published an empty object; the case cannot mean anything")
	}
	for _, k := range sortedKeys(emitted) {
		if !pinned[k] {
			t.Errorf("`astro init --output json` published %q, which is not in the pinned contract.\n"+
				"If it is meant to be public, run `make update-schemas` and\n"+
				"commit the golden; if not, it should not be on stdout.\npinned: %v",
				k, sortedKeys(anyMap(pinned)))
		}
	}
}

// pinnedKeys reads the top-level keys of a pinned payload.
func pinnedKeys(t *testing.T, name string) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "cmd", "local", "testdata", "schema", name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the pinned %s payload: %v", name, err)
	}
	var shape map[string]any
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	keys := make(map[string]bool, len(shape))
	for k := range shape {
		keys[k] = true
	}
	return keys
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func anyMap(m map[string]bool) map[string]any {
	out := make(map[string]any, len(m))
	for k := range m {
		out[k] = nil
	}
	return out
}
