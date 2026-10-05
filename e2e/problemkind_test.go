//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// A failed command names its failure, through the built binary.
//
// The unit tests drive cliout.Execute over a root they build, which proves the
// resolver and the envelope but not that the binary's own root still reaches
// them. That is the way this feature breaks: nothing errors, the field is
// simply never there again. Running the real binary is also how `no_project`
// was found in the first place — every kind written before it presumed you
// were already in a project, and the failure a script meets first had no name.
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

// A command invoked wrongly exits 2, in either mode, and names it `usage` in
// json mode — through the binary, because exit statuses are main's and only a
// subprocess sees them. Both trees: the v2 one, and a cloud command, whose
// flag error is reported before its pre-run reads any login.
//
// Tier 0: nothing runs; the flag parse fails first.
func TestAUsageErrorExitsTwo(t *testing.T) {
	tier(t, 0)

	p := newProject(t)

	for _, args := range [][]string{
		{"local", "status", "--bogus"},
		{"deployment", "list", "--bogus"},
		{"bogus"},
	} {
		r := p.run(args...)
		if r.ExitCode != 2 {
			t.Errorf("`astro %s` exited %d, want 2\n%s", strings.Join(args, " "), r.ExitCode, r.output())
		}
	}

	r := p.run("local", "status", "--bogus", "--output", "json")
	if r.ExitCode != 2 {
		t.Fatalf("exit %d, want 2\n%s", r.ExitCode, r.output())
	}
	var emitted map[string]any
	r.requireJSON(&emitted)
	if emitted["kind"] != "usage" || emitted["code"] != float64(2) {
		t.Errorf("want kind usage and code 2, got %v", emitted)
	}
}
