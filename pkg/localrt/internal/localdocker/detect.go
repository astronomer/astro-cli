package localdocker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/container"
)

// Multi-engine detection, ported from Astro Desktop:
// a project's containers may run under docker, OrbStack, or podman, and the
// engine that started them is not necessarily the one detection would pick
// today. Attach-side operations (logs, exec, status, stop) therefore probe
// the preferred engine first, then the other, and use whichever actually
// has containers for the project directory.

// engineConn is one reachable container engine: the binary to shell out to
// and the env needed to reach its daemon (podman machine socket; nil for
// docker/OrbStack).
type engineConn struct {
	bin string
	env []string
}

const (
	workingDirLabel = "com.docker.compose.project.working_dir"
	// composeProjectLabel carries the project name the working_dir label's
	// containers were brought up under. A constant for the same reason its
	// sibling is one: misspelled inline, it yields an empty name for every
	// container an engine reports, and an empty name reads as "nothing
	// running" rather than as an error — so the typo would be invisible.
	composeProjectLabel = "com.docker.compose.project"
)

// probeTimeout bounds one engine probe.
//
// Every probe shells out, and the engine on the other end is not always
// answering: a podman whose machine is down takes the better part of a second
// to say so, and a docker socket that accepts the connection and then goes
// quiet never says anything at all. Until this was here the probe inherited
// context.Background() from its callers — a listing had nothing bounding it,
// and `astro use` on a machine with a stalled engine hung rather than
// reporting what it could not reach.
//
// Generous on purpose: a probe is one `ps` against a local daemon, so a
// healthy engine answers in milliseconds and only a sick one gets near this.
// Cutting a slow-but-alive engine off reports a project stopped when it is
// not, which is why the start path asks whether an engine answered at all
// (StatusOfReached) rather than trusting the verdict.
//
// A var for the same reason rollbackTimeout and logCaptureTimeout are: the
// property worth testing is what happens when the deadline fires, and a test
// that has to wait five seconds to find out is a test nobody runs.
var probeTimeout = 5 * time.Second

// Engine binaries probed for a project's containers.
const (
	binDocker = "docker"
	binPodman = "podman"
)

// resolvePreferredEngine picks the engine for starting a project: the
// container.binary pin when it names one, otherwise pkg/container's
// resolution ($PATH search, OrbStack detection), plus its connection env
// from connFor.
func resolvePreferredEngine(binary string, connFor func(bin string) engineConn) (engineConn, error) {
	eng, err := container.Resolve(container.Config{Binary: binary})
	if err != nil {
		return engineConn{}, err
	}
	return connFor(eng.Binary()), nil
}

// connFor builds the connection for an engine binary. Connection-env
// lookup is best-effort: without it the commands still reach a default
// docker daemon, and a down podman machine simply yields no containers.
func connFor(bin string) engineConn {
	mgr, err := container.NewManager(container.Config{Binary: bin}, nil)
	if err != nil {
		return engineConn{bin: bin}
	}
	env, _ := mgr.ConnectionEnv() //nolint:errcheck // best-effort, per the comment above
	return engineConn{bin: bin, env: env}
}

// findProject locates the engine that runs a project's containers and the
// compose project name they carry, by asking each candidate engine for a
// running container labeled with the project's working dir: the preferred
// engine first, then the other of docker/podman — so the common case is
// one lookup, and the other engine's connection (for podman, a machine
// inspect) is only resolved when the first probe comes up empty. An empty
// name means no engine has running containers for this directory.
func (e *Engine) findProject(ctx context.Context, projectPath string) (conn engineConn, composeProject string) {
	conn, name, _ := e.probeEngines(ctx, projectPath)
	return conn, name
}

// probeEngines asks the preferred engine, then the other of docker/podman,
// for the project's running compose name — so the common case is one lookup
// and the second engine's connection is resolved only when the first comes up
// empty. It returns the first match with its engine, and whether at least one
// engine answered without error, which lets a caller tell a clean "not found"
// from "no engine reachable".
func (e *Engine) probeEngines(ctx context.Context, projectPath string) (conn engineConn, composeProject string, reached bool) {
	pref, otherBin := e.engineOrder(ctx, projectPath)
	prefName, prefErr := e.probe(ctx, pref, projectPath)
	if prefName != "" {
		return pref, prefName, true
	}
	oconn := e.otherConn(ctx, otherBin)
	otherName, otherErr := e.probe(ctx, oconn, projectPath)
	if otherName != "" {
		return oconn, otherName, true
	}
	// Reached when at least one engine answered cleanly with no match; a
	// not-installed second engine erroring is normal.
	return engineConn{}, "", prefErr == nil || otherErr == nil
}

// engineOrder is the engine a probe asks first and the name of the one it
// falls through to. Both the per-project probe and the whole-machine sweep
// consult the engines in this order, and they used to spell the rule out
// separately — so a change to it (a third engine, a different fallback) could
// be made in one and missed in the other, leaving the two paths disagreeing
// about which engine owns a project.
//
// Only the preferred engine's connection is resolved here. The other's costs a
// `podman machine ls` and `inspect`, and there is no reason to pay for them
// until the first engine has come up short.
//
// projectPath is the project whose container.binary decides the preferred
// engine; "" (the whole-machine sweep) asks for the global setting.
func (e *Engine) engineOrder(ctx context.Context, projectPath string) (pref engineConn, otherBin string) {
	pref = bounded(ctx, engineConn{bin: binDocker}, func() engineConn {
		conn, err := e.preferred(projectPath)
		if err != nil {
			return engineConn{bin: binDocker}
		}
		return conn
	})
	otherBin = binPodman
	if pref.bin == binPodman {
		otherBin = binDocker
	}
	return pref, otherBin
}

// otherConn resolves the fall-through engine's connection under the probe
// deadline.
func (e *Engine) otherConn(ctx context.Context, bin string) engineConn {
	return bounded(ctx, engineConn{bin: bin}, func() engineConn { return e.connFor(bin) })
}

// bounded runs f under probeTimeout and returns fallback if it overruns.
//
// It exists because resolving an engine connection is the one step on the
// probe path that cannot be handed a deadline: connFor reaches
// container.Manager.ConnectionEnv, which shells out to `podman machine ls`
// and `inspect` through exec.Commands that take no context at all. Bounding
// only the `ps` that follows left the hang exactly where it was found — on a
// machine whose podman is wedged, the CLI never reached a probe to time out.
//
// The abandoned goroutine outlives the call, which is the price of work that
// cannot be canceled: the send is buffered so it never blocks, and what it
// holds is one subprocess this process was already waiting on. The fallback is
// a bare connection, which still reaches a default daemon — the same thing
// connFor returns when the manager cannot be built.
func bounded[T any](ctx context.Context, fallback T, f func() T) T {
	done := make(chan T, 1)
	go func() { done <- f() }()
	timer := time.NewTimer(probeTimeout)
	defer timer.Stop()
	select {
	case v := <-done:
		return v
	case <-timer.C:
		return fallback
	case <-ctx.Done():
		return fallback
	}
}

// probe asks one engine for the compose project name of a running
// container whose working_dir label matches projectPath. It returns the
// engine's error so a caller that must distinguish "no such project" from
// "engine unreachable" can — findProject treats both as an empty name.
func (e *Engine) probe(ctx context.Context, conn engineConn, projectPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := e.cmd.Output(
		ctx, conn.env, conn.bin, "ps",
		"--filter", "label="+workingDirLabel+"="+projectPath,
		"--format", `{{.Label "`+composeProjectLabel+`"}}`,
	)
	if err != nil {
		return "", err
	}
	name, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return name, nil
}

// projectContainers counts the containers compose knows about for one project
// name, stopped ones included.
//
// Scoped, and --all, on purpose. `down --project-name X` can only remove
// containers labeled with X, so X is the only scope whose contents decide
// whether a teardown is safe. Asking the working-dir label instead — as an
// earlier version of the failed-start cleanup did — got it wrong in both
// directions: it missed stopped containers of our own, which the guard then
// destroyed, and it reported foreign projects in the same directory that the
// teardown could never have touched, which made it refuse to clean up after
// itself.
func (e *Engine) projectContainers(ctx context.Context, conn engineConn, name string) (int, error) {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "compose", "-p", name, "ps", "-aq")
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	return len(strings.Split(trimmed, "\n")), nil
}

// ContainersGone reports whether projectPath's containers are confirmed
// absent from every reachable engine. It errors when no engine could be
// reached, so a caller that deletes state (astro local list --clean) refuses
// to drop a docker record whose engine is only momentarily down — unlike
// findProject, which cannot tell that apart from a stopped project.
func (e *Engine) ContainersGone(ctx context.Context, projectPath string) (bool, error) {
	_, name, reached := e.probeEngines(ctx, projectPath)
	if name != "" {
		return false, nil
	}
	if !reached {
		return false, fmt.Errorf("no container engine reachable to confirm %s is stopped", projectPath)
	}
	return true, nil
}

// runningProjects maps every running compose project's working directory to
// its compose project name, across the engines that answer.
//
// This is probe's question asked once for the whole machine instead of once
// per project, and it exists because the per-project spelling does not scale:
// a listing walks N records, each one costing a `ps` against the preferred
// engine and — when that came up empty, which is the normal case for a record
// that is not running — a second against the other engine. On one machine with
// 26 docker records and a podman whose machine was down, that was 52 probes
// and 18 seconds for a command that prints a table. Asked this way it is one
// call, or two when something is still unaccounted for.
//
// The second engine is probed only when a path is still unmatched, which is
// the same fall-through rule findProject follows: the common case stays a
// single call, and the other engine's connection (for podman, a machine
// inspect) is resolved only when there is a reason to.
//
// An engine that errors contributes nothing rather than failing the lookup —
// podman not being installed is the ordinary case, not a broken machine. What
// it does report is reached: whether any engine answered at all, which is the
// difference between "nothing is running" and "nobody was home to ask". A
// caller that deletes state on the answer has to be able to tell those apart,
// and a map alone cannot.
//
// Every directory keeps ALL the project names found under it, not one. A
// directory is not a key: a person running their own `docker compose up` in
// their project, or an older astro stack still up under a previous name, puts
// two projects in one directory, and keeping a single winner meant the
// record's own project could lose its slot and its live Airflow be reported
// stopped.
func (e *Engine) runningProjects(ctx context.Context, paths []string) (found map[string][]string, reached bool) {
	pref, otherBin := e.engineOrder(ctx, "")
	found, reached = e.projectsOn(ctx, pref)
	if allFound(found, paths) {
		return found, reached
	}
	more, otherReached := e.projectsOn(ctx, e.otherConn(ctx, otherBin))
	for dir, names := range more {
		found[dir] = append(found[dir], names...)
	}
	return found, reached || otherReached
}

// allFound reports whether every path already has an answer, which is when
// there is nothing left for a second engine to tell us.
func allFound(found map[string][]string, paths []string) bool {
	for _, p := range paths {
		if len(found[p]) == 0 {
			return false
		}
	}
	return true
}

// projectsOn asks one engine for all of its running compose projects, keyed by
// working directory. It filters on the label's presence rather than its value,
// which is what makes one call answer for every project at once.
//
// reached is false when the engine did not answer — not installed, daemon
// down, or the probe deadline caught it. Indistinguishable from each other and
// deliberately so; what a caller needs is that the answer is not evidence.
func (e *Engine) projectsOn(ctx context.Context, conn engineConn) (projects map[string][]string, reached bool) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := e.cmd.Output(
		ctx, conn.env, conn.bin, "ps",
		"--filter", "label="+workingDirLabel,
		"--format", `{{.Label "`+workingDirLabel+`"}}`+"\t"+`{{.Label "`+composeProjectLabel+`"}}`,
	)
	projects = map[string][]string{}
	if err != nil {
		return projects, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dir, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || dir == "" || name == "" {
			continue
		}
		// One compose project has several containers, so the same pair
		// arrives once per container. Recorded once: the names are what a
		// record is matched against, and a duplicate changes no answer while
		// making a listing of them read as two projects.
		if !slices.Contains(projects[dir], name) {
			projects[dir] = append(projects[dir], name)
		}
	}
	return projects, true
}

// ContainersGoneAll is ContainersGone for many projects at once, and for the
// same reason StatusOfAll exists: the --clean sweep asked per record, so the
// probe storm this file removed from the listing survived in the command that
// immediately follows it. On the machine that prompted all this, a --clean of
// 26 stale records still took 18 seconds after the listing itself was down to
// one call.
//
// It errors rather than reporting absence when no engine could be reached,
// which is the whole reason the batch had to carry reachability: --clean
// deletes records and their routes, and a momentary daemon outage must not be
// read as "every project is stopped".
func (e *Engine) ContainersGoneAll(ctx context.Context, paths []string) (map[string]bool, error) {
	gone := make(map[string]bool, len(paths))
	if len(paths) == 0 {
		return gone, nil
	}
	found, reached := e.runningProjects(ctx, paths)
	if !reached {
		return nil, fmt.Errorf("no container engine reachable to confirm %d project(s) are stopped", len(paths))
	}
	for _, p := range paths {
		gone[p] = len(found[p]) == 0
	}
	return gone, nil
}
