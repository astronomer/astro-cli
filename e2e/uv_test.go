//go:build e2e

package e2e

import (
	"os/exec"
	"testing"
)

// needsUV skips a case when uv is not reachable.
//
// Separate from the tier gate, which says how much this run is willing to pay.
// A run that asked for tier 1 on a machine without uv gets a skip naming the
// tool rather than a failure inside a command that cannot work — and a machine
// without uv cannot run `astro local start` either, so there is nothing here it
// could have told us.
func needsUV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv is not on PATH, so no Python environment can be built")
	}
}

// sync builds the project's own .venv, the way `astro local start` does before
// it runs anything.
//
// Used by the cases that check what happens when a project HAS its own
// interpreter. The cases that check the other half — a project with none — call
// nothing and let the CLI provision one, which is what a docker-mode project
// does in real use.
func (p *project) sync() {
	p.t.Helper()
	cmd := exec.Command("uv", "sync", "--quiet")
	cmd.Dir = p.Dir
	cmd.Env = p.env(nil)
	if out, err := cmd.CombinedOutput(); err != nil {
		p.t.Fatalf("uv sync: %v\n%s", err, out)
	}
}

// airflowProject is a scaffolded project with a real Airflow environment built
// for it, which is what every tier-1 case starts from.
func airflowProject(t *testing.T) *project {
	t.Helper()
	needsUV(t)
	p := newProject(t)
	p.run("init", "--name", "tier1").requireSuccess()
	p.sync()
	return p
}
