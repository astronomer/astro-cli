package localdocker

import (
	"context"
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
	pref, err := e.preferred()
	if err != nil {
		pref = engineConn{bin: "docker"}
	}
	if name := e.probe(ctx, pref, projectPath); name != "" {
		return pref, name
	}
	other := "podman"
	if pref.bin == "podman" {
		other = "docker"
	}
	conn = e.connFor(other)
	if name := e.probe(ctx, conn, projectPath); name != "" {
		return conn, name
	}
	return engineConn{}, ""
}

// probe asks one engine for the compose project name of a running
// container whose working_dir label matches projectPath.
func (e *Engine) probe(ctx context.Context, conn engineConn, projectPath string) string {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "ps",
		"--filter", "label="+workingDirLabel+"="+projectPath,
		"--format", `{{.Label "com.docker.compose.project"}}`,
	)
	if err != nil {
		return ""
	}
	name, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return name
}
