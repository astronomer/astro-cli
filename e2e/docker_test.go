//go:build e2e && !windows

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// needsDocker skips when no engine will answer, or when compose will not
// resolve under the environment these cases actually use.
//
// Separate from the tier gate, which says how much this run is willing to pay:
// a run that asked for tier 3 on a machine with no engine gets a skip naming
// what is missing, rather than a failure inside a compose call.
//
// Both halves are asked, and the second is asked with p.env(). `docker info`
// answering says the daemon is up and nothing about whether the CLI — running
// with a redirected HOME and the DOCKER_CONFIG the harness sets — can find the
// compose plugin. Probing only the daemon would skip for the failure that
// cannot happen here and run into the one that can, which is the failure this
// whole harness change exists to remove.
func needsDocker(t *testing.T, p *project) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		t.Skipf("no docker engine answered, so nothing here can run: %v\n%s", err, out)
	}

	compose := exec.CommandContext(ctx, "docker", "compose", "version")
	compose.Env = p.env(nil)
	if out, err := compose.CombinedOutput(); err != nil {
		t.Skipf("docker compose does not resolve under this suite's environment: %v\n%s", err, out)
	}
}

// dockerProject is a scaffolded project that will be started in docker mode,
// with everything that start creates guaranteed not to outlive the run.
//
// `stop --clean` rather than a plain stop, because a plain stop keeps the
// metadata database by design — which is right for a person's own project and
// wrong here. Every case builds under a fresh temp directory, so every case
// gets its own compose project, its own 44 MB postgres volume and, where a
// Dockerfile is declared, its own 1.34 GB image. Thirteen volumes and four
// images accumulated while this file was being written. e2e/doc.go calls a
// case that leaks into the developer's own state worse than no case, and
// docker's storage is the one place the three env levers cannot reach.
//
// `reset --yes` rather than `stop --clean`, which is the same list of things
// and one command short of reaching them here: stop refuses outright when no
// runtime is recorded — "no local Airflow is recorded for this project" — and
// a case that has already stopped, as the lifecycle one does because stopping
// is what it asserts, has no record left by the time cleanup runs. It left the
// 44 MB volume every time. Reset derives the compose project from the path and
// so works either way, which is the case its own doc says it was written for.
//
// runSlow because a compose down gives five containers ten seconds each before
// it removes them, which a loaded machine can take past the ordinary bound —
// and a cleanup that times out fails the case rather than the thing it was
// cleaning up after.
func dockerProject(t *testing.T, name string) *project {
	t.Helper()
	p := newNamedProject(t, name)
	p.run("init", "--name", name).requireSuccess()
	// A case here can leave containers, a metadata volume and a built image.
	// The image is not only the declared-Dockerfile cases: imagebuild runs the
	// base as-is only when the project adds no dependencies AND no OS packages,
	// so declaring either one tags astro-local/<project> too.
	// Checked, because an unchecked cleanup is how the last leak went unnoticed
	// for a whole revision: `stop --clean` had been refusing on every
	// already-stopped case and nothing said so. Reset returns early and
	// succeeds when there is no compose file, which is the skipped-docker case.
	t.Cleanup(func() { p.runSlow("local", "reset", "--yes").requireSuccess() })
	return p
}

// composeProject is the name compose published this project under, read out of
// the state record while the project is still running.
//
// runtime.json, which is the record — state.json beside it is userstate's
// per-user preferences and holds only a port reservation, which is what the
// first version of this read and why it found no compose project at all.
//
// The recorded name rather than one rebuilt from the hostname. Rebuilding it
// was wrong twice: the real name is astro-<sanitized base>-<6 hex of the path
// hash>, so a project whose hostname carries a dot — every project inside a
// linked git worktree, where the hostname is <worktree>.<repo>.localhost —
// produced a prefix matching no container at all. And the record is what
// `stop` removes, so the question had to be asked before the answer was
// needed anyway.
func composeProject(t *testing.T, p *project) string {
	t.Helper()
	matches := stateFilesInCache(t, p, "runtime.json")
	if len(matches) != 1 {
		t.Fatalf("expected one state record under %s, found %d", p.cache, len(matches))
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[0], err)
	}
	var rec struct {
		ComposeProject string `json:"composeProject"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("decoding %s: %v", matches[0], err)
	}
	if rec.ComposeProject == "" {
		t.Fatalf("no compose project recorded in %s:\n%s", matches[0], raw)
	}
	return rec.ComposeProject
}

// stateFilesInCache lists files of one name across every project state
// directory in this cache.
//
// Named for the whole cache rather than for p, because that is its scope: the
// state directory is named by a hash of the project path, and p.sibling shares
// a cache deliberately, so a case with two projects sees both. Callers that
// want "this project's one file" get that from the count being 1.
//
// Read rather than globbed. The directory name is a hash this suite must not
// reimplement — deriving it here would agree with the code under test whether
// or not it is right — and a glob would also take the cache path as pattern
// syntax, so a temp directory carrying a bracket would match the wrong set or
// fail to parse.
func stateFilesInCache(t *testing.T, p *project, name string) []string {
	t.Helper()
	root := filepath.Join(p.cache, "astro", "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No project has written state yet, which is an answer.
			return nil
		}
		t.Fatalf("reading %s: %v", root, err)
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), name)
		if _, err := os.Stat(path); err == nil {
			found = append(found, path)
		}
	}
	return found
}

// startedDockerStack asserts that a docker-mode start produced a running stack,
// and returns the compose project it published under.
//
// Both halves are read while the project is up, because every later assertion
// in these cases is that something is GONE, and that is only a change if it was
// there to begin with.
//
// Running containers rather than all of them: containersFor counts an exited
// one, so a stack that came up and fell over satisfies "there are containers"
// while being the opposite of the start this is checking for.
func startedDockerStack(t *testing.T, p *project) string {
	t.Helper()
	project := composeProject(t, p)
	if up := runningContainersFor(t, project); len(up) == 0 {
		t.Fatalf("no running containers for compose project %s, so the start did not bring a stack up", project)
	}
	if vols := volumesFor(t, project); len(vols) == 0 {
		t.Fatalf("no volumes for compose project %s, so nothing here can show one being removed", project)
	}
	return project
}

// containersFor is every container compose has for this project, running or
// not.
//
// Asked of docker rather than of the CLI, because what the CLI believes is
// what the record says, and the record is the thing these cases doubt.
//
// Two details the product's own container lookup already learned. Scoped by
// the compose project LABEL rather than by a name filter, because docker's
// name filter is an unanchored regex over every container on the machine —
// any unrelated container whose name contains this project's would be counted
// as a leak. And --all, because a stop that degraded to `compose stop` leaves
// exited containers, which is exactly the regression the lifecycle case says
// it is watching for and which plain `ps` cannot see.
func containersFor(t *testing.T, project string) []string {
	t.Helper()
	return containersMatching(t, project, "--all")
}

// runningContainersFor is the subset that is actually up.
//
// The distinction matters where the claim is that containers are still coming
// up: containersFor counts an exited one, so a stack that crashed on its way
// up satisfies "they were left running" while being the opposite of it.
func runningContainersFor(t *testing.T, project string) []string {
	t.Helper()
	return containersMatching(t, project, "--filter", "status=running")
}

func containersMatching(t *testing.T, project string, extra ...string) []string {
	t.Helper()
	args := append([]string{"ps"}, extra...)
	args = append(args,
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.Names}}")
	names, err := dockerLines(t.Context(), args...)
	if err != nil {
		t.Fatalf("listing containers for %s: %v", project, err)
	}
	return names
}

// Docker mode comes up, is reachable, says it is docker, and goes away again.
//
// The tier-2 lifecycle case for the other engine. What differs is worth
// stating: the runtime is a set of containers rather than a process, so
// `status` reports no pid, and what `stop` reaps is compose's project rather
// than a process group. A stop that cleared the record and left the containers
// running would satisfy every assertion that only asked the CLI, which is why
// the last one asks docker.
func TestDockerStartRunsAirflowAndStopTakesItDown(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dockerlife")
	needsDocker(t, p)
	p.runSlow("local", "start", "--docker").requireSuccess()

	st := p.status()
	if st.State != "running" {
		t.Fatalf("state = %q, want running", st.State)
	}
	if st.Mode != "docker" {
		t.Errorf("mode = %q, want docker", st.Mode)
	}
	if st.Port <= 0 {
		t.Fatalf("a running project should record its port, got %d", st.Port)
	}
	// No pid: the runtime is containers, and a number here would name a
	// process this engine does not have.
	if st.PID != 0 {
		t.Errorf("pid = %d, want 0 for a container runtime", st.PID)
	}

	// The port is the claim worth checking hardest, as in tier 2: a record
	// saying running while nothing answers is what this tier exists for.
	if !acceptsWithin(st.Port, 3*time.Minute) {
		t.Fatalf("port %d never answered", st.Port)
	}

	row, ok := lineWith(p.run("local", "list").requireSuccess().Stdout, p.Dir)
	if !ok {
		t.Fatal("a running docker project should appear in `astro local list`")
	}
	if !strings.Contains(row, "docker") {
		t.Errorf("the list row does not say docker: %q", row)
	}

	project := composeProject(t, p)
	if running := containersFor(t, project); len(running) == 0 {
		t.Fatalf("no containers for compose project %s, so the record describes nothing", project)
	}

	p.runSlow("local", "stop").requireSuccess()

	if stopped := p.status(); stopped.State != "stopped" {
		t.Errorf("state after stop = %q, want stopped", stopped.State)
	}
	// The half a record cannot tell you: a stop that removed the record and
	// left the containers up is the failure that looks fine from the CLI.
	if left := containersFor(t, project); len(left) != 0 {
		t.Errorf("stop left %d container(s) behind: %v", len(left), left)
	}
	// Waited for rather than asserted once: compose down returns when it has
	// issued the removals, and dockerd tears the published port's forwarding
	// down after that. runtime_test.go calls the released port the one
	// genuinely asynchronous thing in this suite, and uses the same wait.
	waitFor(t, "the port to be released", func() bool {
		return portClosed(t, st.Port)
	})
}

// A project's own Dockerfile is what docker mode builds.
//
// This did not work at all. The engine leaves BaseImage empty when a Dockerfile
// is declared — the file carries its own FROM — and the composition root's
// adapter dropped Dockerfile and Context on the way to the builder, so the
// builder had neither a base nor a file, built nothing, and returned an empty
// tag. The compose file went out with `image:` blank and the start died on
// "services.db-migration.image must be a string": a compose validation error
// for a feature that had never run once.
//
// So the assertion is not that a build happened but that THIS file did: a RUN
// line writes a marker and the running container is asked for it. An image
// built from the runtime base without the project's Dockerfile passes every
// other check here.
func TestDockerModeBuildsTheProjectsOwnDockerfile(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "ownfile")
	needsDocker(t, p)
	// FROM the runtime image, which is where a project bringing its own file
	// starts. Its ONBUILD triggers expect these two, and a project that omits
	// them fails on a missing requirements.txt rather than on anything this
	// case is about.
	write(t, filepath.Join(p.Dir, "requirements.txt"), "")
	write(t, filepath.Join(p.Dir, "packages.txt"), "")
	write(t, filepath.Join(p.Dir, "Dockerfile"),
		"FROM "+runtimeImageFor(t, p)+"\n"+
			"RUN echo built-from-my-dockerfile > /tmp/astro-marker\n")
	declareDockerfile(t, p)

	p.runSlow("local", "start", "--docker").requireSuccess()
	if st := p.status(); st.State != "running" {
		t.Fatalf("state = %q, want running", st.State)
	}

	got := p.runSlow("local", "run", "--", "cat", "/tmp/astro-marker").requireSuccess()
	if !strings.Contains(got.Stdout, "built-from-my-dockerfile") {
		t.Errorf("the RUN line in the project's Dockerfile did not take effect\n%s", got.output())
	}
}

// runtimeImageFor is the base image the CLI itself would resolve for this
// project, derived from the pin the scaffold wrote.
//
// Derived rather than written down, because a literal here is a second pin of
// the same thing: when the scaffold's Airflow moves and this does not, the
// case pulls a SECOND 1.34 GB image — undercutting the only argument for
// putting this tier on the nightly — and eventually fails on a tag the
// registry has retired, naming nothing about the feature under test.
func runtimeImageFor(t *testing.T, p *project) string {
	t.Helper()
	raw := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	m := regexp.MustCompile(`apache-airflow==(\d+\.\d+)\.\*`).FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("no apache-airflow series pin in the scaffolded manifest:\n%s", raw)
	}
	version := m[1]
	// The same repo imagebuild.RuntimeImage builds its tag from.
	return "astrocrpublic.azurecr.io/runtime:" + version
}

// A Dockerfile that cannot build says so, and leaves nothing behind.
//
// The build runs before any container is created, so what this asserts is that
// a failed build is reported as one — naming the Dockerfile rather than
// surfacing as a compose error — and that the project is left clean. Taking
// containers back down is the neighboring path and needs a failure after
// compose up, which today means the health timeout.
func TestAFailedDockerBuildSaysSoAndLeavesNothingBehind(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "badfile")
	needsDocker(t, p)
	// The runtime base, not a small unrelated one. Docker mode refuses a
	// declared Dockerfile that does not build on an Astro Runtime image, and it
	// refuses before the build — so a project on `alpine` never reaches the
	// build this case is about, and passes on a refusal message that happens to
	// contain the word "Dockerfile".
	write(t, filepath.Join(p.Dir, "Dockerfile"),
		"FROM "+runtimeImageFor(t, p)+"\nRUN exit 1\n")
	declareDockerfile(t, p)

	r := p.runSlow("local", "start", "--docker").requireFailure()

	// The Dockerfile is named. Before the adapter was fixed this same project
	// failed with "services.db-migration.image must be a string", which names
	// nothing the reader wrote.
	if !strings.Contains(r.Stderr, "Dockerfile") {
		t.Errorf("the error does not name the Dockerfile that failed\n%s", r.output())
	}
	// And it failed at the BUILD. The base refusal also names a Dockerfile, so
	// a project this case could not build for some other reason would satisfy
	// the check above while never reaching the build — which is what happened
	// when this case still said `FROM alpine`.
	if strings.Contains(r.Stderr, "must build on an Astro Runtime image") {
		t.Errorf("this case was refused before the build, so it is not testing a failed build\n%s", r.output())
	}

	// The same four things the standalone twin checks, for the same reason it
	// gives: a partial record is the failure worth catching, and each of these
	// can survive on its own. The route especially — ports are drawn before
	// the build runs, so a future reordering that registered the hostname
	// alongside them would leave this green with only the list checked.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("a failed build left %d route(s) claimed: %+v", len(left), left)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("a failed build left a record in `astro local list --all`")
	}
	if st := p.status(); st.State != "stopped" || st.PID != 0 || st.Port != 0 {
		t.Errorf("status after a failed build = %+v, want stopped with no pid or port", st)
	}
}
