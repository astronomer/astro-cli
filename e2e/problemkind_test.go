//go:build e2e

package e2e

import (
	"testing"
)

// A failed command names its failure, through the built binary.
//
// The unit tests call emitJSONError directly, which proves the resolver and
// the envelope but not that cobra's wrapper still reaches them. That is the
// way this feature breaks: nothing errors, the field is simply never there
// again. Running the real binary is also how `no_project` was found in the
// first place — every kind written before it presumed you were already in a
// project, and the failure a script meets first had no name.
//
// Tier 0: an empty directory and one command. There is nothing to start.
func TestAFailedCommandPublishesItsKind(t *testing.T) {
	tier(t, 0)

	p := newProject(t)

	var emitted map[string]any
	// No project here — newProject gives a bare directory — so this is the
	// first failure any consumer meets.
	p.run("local", "stop", "--output", "json").
		requireFailure().
		requireJSON(&emitted)

	if got := emitted["kind"]; got != "no_project" {
		t.Errorf("kind = %v, want %q\nemitted: %v", got, "no_project", emitted)
	}
	// Beside the prose, not instead of it: a person still has to be able to
	// read what happened.
	if msg, _ := emitted["error"].(string); msg == "" {
		t.Errorf("the failure published no message alongside its kind: %v", emitted)
	}

	// And every key it publishes is one somebody pinned, `kind` included.
	pinned := pinnedKeys(t, "error")
	for _, k := range sortedKeys(emitted) {
		if !pinned[k] {
			t.Errorf("a failed command published %q, which is not in the pinned contract.\n"+
				"If it is meant to be public, run `make update-schemas` and commit the golden.\npinned: %v",
				k, sortedKeys(anyMap(pinned)))
		}
	}
}
