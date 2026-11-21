//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// `astro deploy --dag-bundle` (1.x's --dag-bundle-name, still accepted) names
// the Dag bundle a deploy ships its Dags to. Naming one on a deploy that ships
// no Dags is refused as a usage error before anything is read or contacted,
// and the 1.x spelling is refused the same way, with the rename hint.
//
// Tier 0: the refusal happens while flags are checked.
func TestDeployDagBundleRefusals(t *testing.T) {
	tier(t, 0)

	parent := manifestProjectForDeploy(t)

	for _, tc := range []struct {
		name string
		args []string
		says string
	}{
		{"--image", []string{"--image", "--dag-bundle", "nightly"}, "cannot use --dag-bundle with --image"},
		{"the 1.x spelling with --image", []string{"--image", "--dag-bundle-name", "nightly"}, "cannot use --dag-bundle with --image"},
		{"an empty name", []string{"--dag-bundle", ""}, "--dag-bundle cannot be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := parent.forT(t)
			r := p.run(append([]string{"deploy"}, tc.args...)...).requireFailure()
			if !strings.Contains(r.Stderr, tc.says) {
				t.Errorf("the refusal should say %q\n%s", tc.says, r.output())
			}
		})
	}

	t.Run("the 1.x spelling says what to type instead", func(t *testing.T) {
		p := parent.forT(t)
		r := p.run("deploy", "--image", "--dag-bundle-name", "nightly").requireFailure()
		if !strings.Contains(r.Stderr, "--dag-bundle-name") || !strings.Contains(r.Stderr, "use --dag-bundle") {
			t.Errorf("a run passing the 1.x spelling should be told to use --dag-bundle\n%s", r.output())
		}
	})
}
