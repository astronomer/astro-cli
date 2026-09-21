//go:build e2e && !windows

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `astro local reset` on a docker project wipes what docker derived and leaves
// the venv alone.
//
// The standalone half is covered; this is the other, and it is not symmetry for
// its own sake. localstandalone's Clean gates on AIRFLOW_HOME precisely so that
// resetting a docker project does not take the venv with it — `astro local
// check` parses DAGs with the project's own .venv interpreter whatever mode
// Airflow runs in, so deleting it here would break a command that has nothing
// to do with standalone, from a command the user ran to clean up docker.
//
// What it removes is asserted as hard as what it spares. "The venv survived" is
// also what a reset that did nothing produces, and "the containers are gone" is
// what a plain `astro local stop` produces — so neither alone is evidence. The
// volume is the one that matters most: it holds the metadata database the user
// is trying to be rid of, and a reset that left it would look entirely
// successful while keeping the thing it was run for.
func TestResettingADockerProjectSparesTheVenv(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dkreset")
	needsDocker(t, p)

	// Stands in for what `astro local check` syncs. A real one costs a uv
	// resolution and proves the same thing about this command: reset decides by
	// the absence of standalone's own evidence, not by looking inside.
	venv := filepath.Join(p.Dir, ".venv")
	marker := filepath.Join(venv, "marker")
	if err := os.MkdirAll(venv, 0o755); err != nil {
		t.Fatalf("creating the stand-in venv this case is about: %v", err)
	}
	write(t, marker, "mine\n")

	dag := filepath.Join(p.Dir, "dags", "exampledag.py")
	dagBefore := read(t, dag)

	p.runSlow("local", "start", "--docker").requireSuccess()

	// Checked here as well as at the end, so a start that removed the venv is
	// blamed on the start. Without this the case fails after the reset, naming
	// a command that did nothing and sending the reader to Clean.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the start removed the stand-in venv, before reset ran: %v", err)
	}

	// Read while it exists, for the same reason the standalone twin reads the
	// venv first: "it is gone afterwards" means nothing about state that was
	// never there.
	project := startedDockerStack(t, p)

	p.runSlow("local", "reset", "--yes").requireSuccess()

	// What reset is for, in the order it would fail.
	if left := containersFor(t, project); len(left) != 0 {
		t.Errorf("reset left %d container(s) behind: %v", len(left), left)
	}
	if left := volumesFor(t, project); len(left) != 0 {
		t.Errorf("reset left %d volume(s) behind, holding the database it was run to remove: %v", len(left), left)
	}
	if st := p.status(); st.State != "stopped" {
		t.Errorf("state after reset = %q, want stopped", st.State)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("reset should leave no runtime record")
	}

	// And what it must not touch.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("reset on a docker project removed the venv, which `astro local check` needs "+
			"in either mode: %v", err)
	}
	if got := read(t, dag); got != dagBefore {
		t.Error("reset rewrote a DAG; it may only remove derived state")
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "pyproject.toml")); err != nil {
		t.Errorf("reset removed the manifest: %v", err)
	}
}

// `astro local reset` removes a stopped docker project's volume, with no record
// to tell it what to remove.
//
// The case above resets a project that is still up, where the record names the
// compose project. This is the other path through Clean, and it is the one a
// person actually reaches: `astro local stop` removes the record and the
// containers and keeps the metadata volume by design, so the state somebody
// resets from is usually the state with nothing left to read.
//
// With no record, Clean derives the compose project from the project path. A
// derivation that disagreed with what compose published would take down a
// project that does not exist, remove the compose file, and report success —
// while the database the reset was run for stayed on the machine. Nothing else
// would notice: the containers really are gone by then, so every other symptom
// of a working reset is already true before this command runs.
//
// The state after the stop is asserted, not assumed. If a stop began removing
// the volume, or stopped removing the record, this would go green while testing
// neither the derivation nor the removal.
func TestResettingAStoppedDockerProjectStillFindsItsVolume(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dkstopped")
	needsDocker(t, p)

	p.runSlow("local", "start", "--docker").requireSuccess()

	// Read while there is still a record to read it from. After the stop the
	// name is exactly what nothing on disk says any more, which is the case.
	project := startedDockerStack(t, p)

	p.runSlow("local", "stop").requireSuccess()

	// Waited for rather than asserted once. compose down returns when it has
	// issued the removals, so a loaded machine can still list containers in
	// Removing — and this is a precondition, so a single shot that lost that
	// race would fail the case before it ran the reset it exists to check.
	waitFor(t, "the stopped project's containers to go", func() bool {
		return len(containersFor(t, project)) == 0
	})
	if recs := stateFilesInCache(t, p, "runtime.json"); len(recs) != 0 {
		t.Fatalf("stop left %d state record(s), so reset would read one instead of deriving: %v", len(recs), recs)
	}
	// The compose file is what Clean gates on: it is the only evidence the
	// project ever ran in docker mode, and a stop that removed it would make
	// the reset below skip everything and still succeed.
	if files := stateFilesInCache(t, p, "docker-compose.yaml"); len(files) != 1 {
		t.Fatalf("want one generated compose file after stop, found %d: %v", len(files), files)
	}
	if kept := volumesFor(t, project); len(kept) == 0 {
		t.Fatalf("stop removed the volume, so there is nothing left here for reset to find; volumes for %s: %v",
			project, kept)
	}

	reset := p.runSlow("local", "reset", "--yes").requireSuccess()

	// The direct evidence, rather than inferring the derivation from a side
	// effect: reset prints the compose project it acted on, and that name comes
	// from what Clean derived. It also separates the two ways this can go wrong
	// — a name derived wrongly from no engine having answered, which exits 0
	// and says so instead of naming a project.
	if want := "removed compose project " + project; !strings.Contains(reset.Stdout, want) {
		t.Errorf("reset did not report acting on %s, so it did not derive the name from the path:\n%s",
			project, reset.output())
	}
	if left := volumesFor(t, project); len(left) != 0 {
		t.Errorf("reset left %d volume(s) behind, holding the database it was run to remove: %v\n%s",
			len(left), left, reset.output())
	}
	if files := stateFilesInCache(t, p, "docker-compose.yaml"); len(files) != 0 {
		t.Errorf("reset left the generated compose file behind: %v", files)
	}
}

// volumesFor lists the volumes compose created for a project.
//
// By the compose project label, not by name: `docker volume ls --filter name=`
// is an unanchored regex, so a project whose name is a prefix of another's
// would answer for both. docker_test.go's containersFor filters on the same
// label for the same reason.
func volumesFor(t *testing.T, project string) []string {
	t.Helper()
	names, err := dockerLines(t.Context(), "volume", "ls",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("listing volumes for %s: %v", project, err)
	}
	return names
}
