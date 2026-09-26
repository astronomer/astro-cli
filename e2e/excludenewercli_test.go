//go:build e2e && !windows

package e2e

import (
	"strings"
	"testing"
)

// The uv the CLI runs resolves under the UV_EXCLUDE_NEWER its environment
// carries, which is how pinnedExcludeNewer reaches it.
//
// The harness's own uv sync is checked by sync, through the lockfile. This is
// the other half, and the one that can break without any e2e change: pkg/uv
// builds each child's environment itself, and a CLI that turned on
// uv.Options.HermeticEnv would strip the variable, leaving every CLI-side
// resolution on live PyPI while the suite still looked pinned.
//
// A cutoff older than every Airflow 3 release makes the scaffold's
// apache-airflow==3.3.* unresolvable, and uv's hint names the cutoff it
// filtered by. That date appears in the output only if the CLI's uv saw it.
// Like TestAFailedStartLeavesNothingBehind, it dies in provisioning, so it
// costs a resolution rather than an Airflow.
func TestTheCLIsUVHonorsTheResolutionDate(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "cutoff").requireSuccess()

	const before = "2020-01-01T00:00:00Z"
	r := p.runWith(map[string]string{"UV_EXCLUDE_NEWER": before}, "local", "start").requireFailure()
	if !strings.Contains(r.output(), before) {
		t.Errorf("the start's output never names the cutoff %s, so the CLI's uv did not resolve under it\n%s", before, r.output())
	}
}
