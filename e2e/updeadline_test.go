//go:build e2e && !windows

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pgRefusal is what the stand-in postgres prints before it exits: the line a
// real postgres prints when its data directory was initialized by a newer
// major version, which is how this hang was first met.
const pgRefusal = "FATAL:  database files are incompatible with server"

// upDeadlineBudget is short so the case is too. Long enough that nothing about
// creating the containers races it: they are created, and their images pulled,
// before the clock starts.
const upDeadlineBudget = "45s"

// upDeadlineBound is how long the start may take in all before this case calls
// it a hang: the budget, a cold pull of both images on a slow runner, and the
// rollback's graceful down. Without the deadline the start never returns, so
// what this bound catches is the bug itself.
const upDeadlineBound = 5 * time.Minute

// A docker start whose metadata database never comes up gives up at the health
// timeout, says which service was failing and what it said, and takes down
// what it created.
//
// It used to hang. The Airflow services wait on the migration completing, so
// `compose up --detach` waits inside itself; the runtime image's entrypoint
// waits on postgres's port with no limit; and postgres under `restart:
// unless-stopped` crash-loops for good on a data directory it refuses. The
// health timeout only started after the up returned, so it never started.
//
// The stand-in postgres exits at once rather than refusing a real data
// directory: what makes the hang is a database that never listens, and this is
// the cheapest one, costing no image the stack does not already pull.
func TestADockerStartWhoseDatabaseNeverComesUpFailsInTime(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "pgdead")
	needsDocker(t, p)
	write(t, filepath.Join(p.Dir, "docker-compose.override.yml"), `services:
  postgres:
    command: ["sh", "-c", "echo '`+pgRefusal+`' >&2; exit 1"]
`)

	r := p.runBounded(upDeadlineBound,
		map[string]string{"ASTRO_LOCAL_HEALTH_TIMEOUT": upDeadlineBudget},
		"local", "start", "--docker").requireFailure()

	// What gave up and when; which service was failing; and its own last
	// words, which are the diagnosis — the containers that held them are gone
	// by now.
	for _, want := range []string{
		"gave up after " + upDeadlineBudget,
		"Last output from postgres",
		pgRefusal,
	} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr does not carry %q\n%s", want, r.output())
		}
	}
	// And not as a health timeout. A crash loop is not slow: advice to wait
	// longer, which the CLI adds to a timeout, would send the reader the wrong
	// way.
	if strings.Contains(r.Stderr, "ASTRO_LOCAL_HEALTH_TIMEOUT") {
		t.Errorf("a failing service was reported as a timeout to wait out\n%s", r.output())
	}

	// Nothing published: the record and route are written after the up.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("a timed-out up left %d route(s) claimed: %+v", len(left), left)
	}
	if st := p.status(); st.State != "stopped" || st.Port != 0 {
		t.Errorf("status after a timed-out up = %+v, want stopped with no port", st)
	}

	// And the containers are gone, asked of docker. With no record to read the
	// compose project from, they are found by the prefix composeProjectName
	// gives every project in a directory of this name.
	if left := containersOfProjectsPrefixed(t, "astro-pgdead-"); len(left) != 0 {
		t.Errorf("a timed-out up left %d container(s) behind: %v", len(left), left)
	}
}

// containersOfProjectsPrefixed lists every container, running or not, whose
// compose project name starts with prefix.
func containersOfProjectsPrefixed(t *testing.T, prefix string) []string {
	t.Helper()
	lines, err := dockerLines(t.Context(), "ps", "-a",
		"--filter", "label=com.docker.compose.project",
		"--format", `{{.Label "com.docker.compose.project"}} {{.Names}}`)
	if err != nil {
		t.Fatalf("listing compose containers: %v", err)
	}
	var found []string
	for _, l := range lines {
		if project, name, _ := strings.Cut(l, " "); strings.HasPrefix(project, prefix) {
			found = append(found, name)
		}
	}
	return found
}
