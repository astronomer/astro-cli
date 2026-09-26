//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// uv records the cutoff it resolved under in the lockfile's [options], so
	// the lock is where to see that this run's date reached it, and that an
	// unpinned run really was unpinned.
	lock, err := os.ReadFile(filepath.Join(p.Dir, "uv.lock"))
	if err != nil {
		p.t.Fatalf("reading the lockfile uv sync wrote: %v", err)
	}
	recorded := strings.Contains(string(lock), "\nexclude-newer = ")
	if cutoff := excludeNewer(); cutoff != "" && !recorded {
		p.t.Fatalf("uv.lock records no exclude-newer, so UV_EXCLUDE_NEWER=%s did not reach uv sync:\n%s", cutoff, lock)
	} else if cutoff == "" && recorded {
		p.t.Fatalf("%s=none, but uv.lock records an exclude-newer:\n%s", excludeNewerEnv, lock)
	}
}

// airflowProject is a scaffolded project with a real Airflow environment built
// for it, which is what every tier-1 case starts from.
func airflowProject(t *testing.T) *project {
	t.Helper()
	return namedAirflowProject(t, "project")
}

// namedAirflowProject is airflowProject with the directory's name chosen, for
// a case that runs two at once and needs them to differ. See newNamedProject.
func namedAirflowProject(t *testing.T, name string) *project {
	t.Helper()
	needsUV(t)
	p := newNamedProject(t, name)
	p.run("init", "--name", "tier1").requireSuccess()
	p.sync()
	return p
}
