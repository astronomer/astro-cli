package localdocker

import (
	"context"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/pkg/container"
)

// Multi-engine detection, ported from Astro Desktop's runtime/docker_logs.go:
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

const workingDirLabel = "com.docker.compose.project.working_dir"

// Engine binaries probed for a project's containers.
const (
	binDocker = "docker"
	binPodman = "podman"
)

// resolvePreferredEngine picks the engine for starting a project: pkg/
// container's resolution ($PATH search, OrbStack detection) plus its
// connection env.
func resolvePreferredEngine() (engineConn, error) {
	eng, err := container.Resolve(container.Config{})
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
	env, _ := mgr.ConnectionEnv()
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
	pref, err := e.preferred()
	if err != nil {
		pref = engineConn{bin: binDocker}
	}
	prefName, prefErr := e.probe(ctx, pref, projectPath)
	if prefName != "" {
		return pref, prefName, true
	}
	other := binPodman
	if pref.bin == binPodman {
		other = binDocker
	}
	oconn := e.connFor(other)
	otherName, otherErr := e.probe(ctx, oconn, projectPath)
	if otherName != "" {
		return oconn, otherName, true
	}
	// Reached when at least one engine answered cleanly with no match; a
	// not-installed second engine erroring is normal.
	return engineConn{}, "", prefErr == nil || otherErr == nil
}

// probe asks one engine for the compose project name of a running
// container whose working_dir label matches projectPath. It returns the
// engine's error so a caller that must distinguish "no such project" from
// "engine unreachable" can — findProject treats both as an empty name.
func (e *Engine) probe(ctx context.Context, conn engineConn, projectPath string) (string, error) {
	out, err := e.cmd.Output(
		ctx, conn.env, conn.bin, "ps",
		"--filter", "label="+workingDirLabel+"="+projectPath,
		"--format", `{{.Label "com.docker.compose.project"}}`,
	)
	if err != nil {
		return "", err
	}
	name, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return name, nil
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
