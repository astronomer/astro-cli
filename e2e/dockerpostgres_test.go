//go:build e2e && !windows

package e2e

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The metadata database's Postgres moved from 12 to 15, and a project whose
// database 12 created keeps running on 12: Postgres 15 refuses to start on a
// data directory 12 initialized, and a plain stop keeps that directory by
// design, so every docker-mode project started before the change has one.
//
// No released CLI that wrote a 12 database can be run from here, so the case
// makes one where a real project's would be: it starts the project fresh,
// which must come up on 15, stops it, and has postgres:12.6 initialize the same
// volume over again — same name, same compose labels, the shape an older CLI
// left. The second start must read that, run 12, and come up healthy on it,
// which is the only proof that the version the CLI picked is one the data
// directory accepts.
func TestAnExistingPostgres12DatabaseKeepsItsVersion(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dkpg12")
	needsDocker(t, p)

	p.runSlow("local", "start", "--docker").requireSuccess()
	project := startedDockerStack(t, p)
	if img := postgresImageOf(t, project); !strings.HasSuffix(img, "postgres:15") {
		t.Fatalf("a new project's database runs %s, want postgres:15", img)
	}

	p.runSlow("local", "stop").requireSuccess()
	waitFor(t, "the stopped project's containers to go", func() bool {
		return len(containersFor(t, project)) == 0
	})
	volume := project + "_postgres_data"
	reinitializeOnPostgres12(t, volume)

	start := p.runSlow("local", "start", "--docker").requireSuccess()
	if img := postgresImageOf(t, project); !strings.HasSuffix(img, "postgres:12.6") {
		t.Fatalf("a database Postgres 12 created runs %s, want postgres:12.6\n%s", img, start.output())
	}
	if !strings.Contains(start.output(), "created by PostgreSQL 12") {
		t.Errorf("the start did not say why this project stays on Postgres 12:\n%s", start.output())
	}
	st := p.status()
	if st.State != "running" || !acceptsWithin(st.Port, 3*time.Minute) {
		t.Fatalf("Airflow did not come up on the Postgres 12 database: state %q, port %d\n%s", st.State, st.Port, start.output())
	}

	// The probe the version was read through is created and removed within the
	// start. One left behind would hold the volume, and the reset in cleanup
	// could not delete it.
	if left, err := dockerLines(t.Context(), "ps", "--all", "--filter", "name=^"+project+"-pgversion$", "--format", "{{.Names}}"); err != nil {
		t.Fatalf("listing probe containers: %v", err)
	} else if len(left) != 0 {
		t.Errorf("the start left its Postgres version probe behind: %v", left)
	}

	// The next start of the same volume remembers what was read: still 12,
	// and the note is not repeated, since it is said once per project.
	p.runSlow("local", "stop").requireSuccess()
	waitFor(t, "the stopped project's containers to go", func() bool {
		return len(containersFor(t, project)) == 0
	})
	again := p.runSlow("local", "start", "--docker").requireSuccess()
	if img := postgresImageOf(t, project); !strings.HasSuffix(img, "postgres:12.6") {
		t.Fatalf("the second start of the Postgres 12 database runs %s, want postgres:12.6\n%s", img, again.output())
	}
	if strings.Contains(again.output(), "created by PostgreSQL 12") {
		t.Errorf("the note about Postgres 12 was repeated; it is said once:\n%s", again.output())
	}
}

// postgresImageOf is the image the project's postgres service runs, asked of
// docker rather than read from the compose file the CLI wrote, which says what
// was asked for and not what started.
func postgresImageOf(t *testing.T, project string) string {
	t.Helper()
	ids := containersMatching(t, project, "--filter", "label=com.docker.compose.service=postgres")
	if len(ids) != 1 {
		t.Fatalf("want one postgres container for %s, found %v", project, ids)
	}
	img, err := dockerLines(t.Context(), "inspect", ids[0], "--format", "{{.Config.Image}}")
	if err != nil || len(img) != 1 {
		t.Fatalf("inspecting %s: %v %v", ids[0], img, err)
	}
	return img[0]
}

// reinitializeOnPostgres12 empties volume and has postgres:12.6 create a
// database in it, the way the compose stack an older CLI wrote would have.
//
// In place rather than a new volume, so the volume keeps the labels compose
// stamped on it: one compose did not create draws a warning, and one whose
// labels disagree with the file can draw a prompt to recreate it.
//
// The server is stopped with SIGTERM once initialization has finished, which
// is a clean shutdown, so the next start finds a data directory that needs no
// recovery. Everything it runs is --rm, and the container is removed in cleanup
// as well in case the case fails between the two.
func reinitializeOnPostgres12(t *testing.T, volume string) {
	t.Helper()
	const image = "docker.io/postgres:12.6"
	mount := volume + ":/var/lib/postgresql/data"
	name := volume + "-e2e-pg12"
	t.Cleanup(func() {
		// Gone already, unless the case failed partway.
		_ = dockerRun(t, time.Minute, "rm", "--force", name)
	})

	if err := dockerRun(t, 5*time.Minute, "run", "--rm", "--volume", mount, "--entrypoint", "find", image,
		"/var/lib/postgresql/data", "-mindepth", "1", "-delete"); err != nil {
		t.Fatalf("emptying %s: %v", volume, err)
	}
	if err := dockerRun(t, time.Minute, "run", "--detach", "--rm", "--name", name, "--volume", mount,
		"--env", "POSTGRES_USER=postgres", "--env", "POSTGRES_PASSWORD=postgres", image); err != nil {
		t.Fatalf("starting postgres 12 on %s: %v", volume, err)
	}
	waitFor(t, "postgres 12 to initialize "+volume, func() bool {
		out, err := exec.CommandContext(t.Context(), "docker", "logs", name).CombinedOutput()
		return err == nil && strings.Contains(string(out), "PostgreSQL init process complete")
	})
	if err := dockerRun(t, time.Minute, "stop", name); err != nil {
		t.Fatalf("stopping postgres 12: %v", err)
	}

	got, err := dockerLines(t.Context(), "run", "--rm", "--volume", mount, "--entrypoint", "cat", image,
		"/var/lib/postgresql/data/PG_VERSION")
	if err != nil || len(got) != 1 || got[0] != "12" {
		t.Fatalf("%s does not hold a Postgres 12 database: %v %v", volume, got, err)
	}
}

// dockerRun runs one docker command to completion under its own bound, for
// the ones dockerLines' listing timeout is too short for: a run may pull.
func dockerRun(t *testing.T, timeout time.Duration, args ...string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Logf("docker %s:\n%s", strings.Join(args, " "), out)
	}
	return err
}
