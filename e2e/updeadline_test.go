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

// upDeadlineBound is how long a start may take in all before these cases call
// it a hang: the budget, a cold pull of both images on a slow runner, and the
// rollback's graceful down. Without the fixes they cover the start never
// returns, so what this bound catches is the bug itself.
const upDeadlineBound = 5 * time.Minute

// A docker start whose metadata database never comes up fails fast, names
// postgres, shows what it said, and takes down what it created.
//
// It used to hang. The Airflow services wait on the migration completing, so
// `compose up --detach` waits inside itself; the runtime image's entrypoint
// waits on postgres's port with no limit; and postgres under `restart:
// unless-stopped` crash-loops for good on a data directory it refuses. Now
// the migration waits on postgres's healthcheck, and compose itself gives up
// with "dependency failed to start" — in seconds, long before the timeout.
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

	// A timeout the failure must not need: the point is that compose reports
	// it, not that the deadline does.
	const budget = 3 * time.Minute
	env := map[string]string{"ASTRO_LOCAL_HEALTH_TIMEOUT": budget.String()}
	// Once untimed, so the images are pulled: a cold pull on a slow runner is
	// not what the timing below is about. It fails the same way, and leaves
	// nothing behind for the timed run to trip over.
	p.runBounded(upDeadlineBound, env, "local", "start", "--docker").requireFailure()

	began := time.Now()
	r := p.runBounded(upDeadlineBound, env, "local", "start", "--docker").requireFailure()
	if took := time.Since(began); took > time.Minute {
		t.Errorf("the start took %s to fail; compose should have reported the dead database in seconds\n%s", took, r.output())
	}

	// Compose's own verdict, which reaches the reader as the up's streamed
	// output; the error itself is the up's exit status.
	if !strings.Contains(r.Stdout+r.Stderr, "dependency failed to start: container astro-pgdead-") {
		t.Errorf("compose's report that postgres failed to start was not shown\n%s", r.output())
	}
	// Its own last words, captured before the containers went: the diagnosis.
	if !strings.Contains(r.Stdout+r.Stderr, pgRefusal) {
		t.Errorf("postgres's output was not shown\n%s", r.output())
	}
	// Not a timeout: advice to wait longer would send the reader the wrong way.
	if strings.Contains(r.Stderr, "ASTRO_LOCAL_HEALTH_TIMEOUT") {
		t.Errorf("a dead database was reported as a timeout to wait out\n%s", r.output())
	}

	assertNothingPublished(t, p, "astro-pgdead-")
}

// A docker start whose migration never finishes gives up at the health
// timeout, and takes down what it created.
//
// The case the deadline on `compose up` is a safety net for: everything the
// up waits on is healthy or running, so compose has nothing to report and
// would wait forever. The CLI stops it — SIGTERM, which the docker CLI passes
// on to the compose plugin — and rolls back.
func TestADockerStartWhoseMigrationHangsHitsTheDeadline(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "mighang")
	needsDocker(t, p)
	write(t, filepath.Join(p.Dir, "docker-compose.override.yml"), `services:
  db-migration:
    command: ["sleep", "infinity"]
`)

	r := p.runBounded(upDeadlineBound,
		map[string]string{"ASTRO_LOCAL_HEALTH_TIMEOUT": "45s"},
		"local", "start", "--docker").requireFailure()

	// Given up on, not "may still be coming up"; the knob that bounds it is
	// named, since a migration that is merely slow is helped by it.
	for _, want := range []string{"did not finish starting in time after 45s", "ASTRO_LOCAL_HEALTH_TIMEOUT"} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr does not carry %q\n%s", want, r.output())
		}
	}

	assertNothingPublished(t, p, "astro-mighang-")
}

// assertNothingPublished checks a failed start left no route, no record and
// no containers. With no record to read the compose project from, the
// containers are found by the prefix composeProjectName gives every project in
// a directory of that name.
func assertNothingPublished(t *testing.T, p *project, prefix string) {
	t.Helper()
	// Routes first: `status` prunes dead routes, which would hide a leak.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("a failed start left %d route(s) claimed: %+v", len(left), left)
	}
	if st := p.status(); st.State != "stopped" || st.Port != 0 {
		t.Errorf("status after a failed start = %+v, want stopped with no port", st)
	}
	if left := containersOfProjectsPrefixed(t, prefix); len(left) != 0 {
		t.Errorf("a failed start left %d container(s) behind: %v", len(left), left)
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
