//go:build e2e && !windows

package e2e

import (
	"os"
	"os/exec"
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

	project := composeProject(t, p)
	if running := containersFor(t, project); len(running) == 0 {
		t.Fatalf("no containers for compose project %s, so there is nothing for reset to remove", project)
	}
	// Read while it exists, for the same reason the standalone twin reads the
	// venv first: "it is gone afterwards" means nothing about state that was
	// never there.
	volumes := volumesFor(t, project)
	if len(volumes) == 0 {
		t.Fatalf("no volumes for compose project %s; this case cannot show that reset removes one", project)
	}

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

// volumesFor lists the volumes compose created for a project.
//
// By the compose project label, not by name: `docker volume ls --filter name=`
// is an unanchored regex, so a project whose name is a prefix of another's
// would answer for both. docker_test.go's containersFor filters on the same
// label for the same reason.
func volumesFor(t *testing.T, project string) []string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "docker", "volume", "ls",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.Name}}").Output()
	if err != nil {
		t.Fatalf("listing volumes for %s: %v", project, err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}
