package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

const (
	// projectDockerfileName is the Dockerfile of the step that copies the
	// project in. It sits in WorkDir, outside the project, beside its ignore
	// file.
	projectDockerfileName = "Dockerfile.astro-project"
	// ignoreFile is the project's own ignore file, at its root.
	ignoreFile = ".dockerignore"
)

// projectIgnoreRules are left out of every ProjectContext copy, after the
// project's own rules, so a "!" there cannot bring one back.
//
//   - Everything the 1.x CLI's default .dockerignore left out (`astro dev
//     init`'s template: astro, .git, .env, airflow_settings.yaml, logs/,
//     .venv, airflow.db, airflow.cfg), so a project converted from 1.x ships
//     no less safely than it did. airflow_settings.yaml holds connection
//     secrets in clear text.
//   - The per-machine files v2's tools and Python write: .astro/ (standalone
//     Airflow's state, local overrides, Otto's tokens), any .venv, .env, .env.*
//     and .envrc at any depth, bytecode, and the pickling fix the standalone
//     engine drops into plugins/ for Airflow 2 on macOS (pkg/scaffold's
//     pickleFixRule).
//   - The project's own requirements.txt and packages.txt, which a 1.x layout
//     may still carry. The generated ones are what the image installed, and
//     stay in AIRFLOW_HOME.
//
// No rule starts with "/": .dockerignore patterns are anchored at the context
// root already, and buildah's parser and Docker's agree on that spelling.
var projectIgnoreRules = []string{
	"astro",
	".git",
	"airflow_settings.yaml",
	"logs",
	"airflow.db",
	"airflow.cfg",
	".astro",
	"**/.venv",
	"**/.env",
	"**/.env.*",
	"**/.envrc",
	"**/__pycache__",
	"**/*.pyc",
	"plugins/fix_local_executor_pickle.py",
	requirementsName,
	packagesName,
}

const projectIgnoreHeader = "# Added by the astro CLI for this build: per-machine files, and the generated dependency files.\n"

// ShipProject asks the generated build req describes to copy the project at
// projectDir into the image, with dags/ left out unless withDags: the one rule
// a deploy and `astro package astro` share. A declared Dockerfile is left as
// it is, since its context is the project already and its own COPY lines
// decide.
func ShipProject(req *Request, projectDir string, withDags bool) {
	if req.FromDeclaredDockerfile() {
		return
	}
	req.ProjectContext = projectDir
	req.ProjectExcludes = nil
	if !withDags {
		req.ProjectExcludes = []string{"dags"}
	}
}

// ProjectIgnore is the ignore file a ProjectContext copy is built with: the
// project's own .dockerignore, then projectIgnoreRules, then excludes. It is
// written beside the generated Dockerfile as <Dockerfile>.dockerignore, which
// BuildKit reads instead of the context's .dockerignore, so the project's file
// is never edited. `astro package astro` reads it to address the image by
// what it copies.
func ProjectIgnore(projectDir string, excludes []string) (string, error) {
	own, err := os.ReadFile(filepath.Join(projectDir, ignoreFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading the project's %s: %w", ignoreFile, err)
	}
	var b strings.Builder
	b.Write(own)
	if len(own) > 0 && own[len(own)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(projectIgnoreHeader)
	for _, rule := range append(append([]string{}, projectIgnoreRules...), excludes...) {
		b.WriteString(rule + "\n")
	}
	return b.String(), nil
}

// buildProject is a generated build that ships req.ProjectContext.
//
// Two builds, because one cannot do it. The runtime image's ONBUILD triggers
// copy requirements.txt and packages.txt from the MAIN build context, so the
// first build is the usual one over the generated context, tagged depsTag.
// The second starts FROM it (a child's triggers have already run, so none fire
// again) and copies the project in with the project as its main context, as
// the 1.x CLI built it: Docker applies the ignore rules and its own symlink
// semantics, and nothing outside the project can be reached. The ignore file
// is <Dockerfile>.dockerignore beside the generated Dockerfile, so the
// project's .dockerignore is read, not rewritten.
//
// The ignore file is what keeps .env, airflow_settings.yaml and the rest out
// of an image that is pushed to a registry, so the second build only runs on
// a builder known to read it (projectBuilder), and it is asked before the
// first build, so a refusal costs nothing. On Docker both builds run on the
// docker-driver builder of the current context, which reads the local image
// store, so the second finds the first.
//
// The intermediate tag is removed once the second build has run, whether or
// not it succeeded, and even when ctx was canceled; its layers stay, as the
// final image's parent.
func (b *Builder) buildProject(ctx context.Context, req Request, dfPath, contextDir string, cb rt.Callbacks) (string, error) {
	pb, err := b.projectBuilder(ctx, req)
	if err != nil {
		return "", err
	}
	deps := req
	deps.Tag = depsTag(req.Tag)
	deps.builder = pb.builder
	if _, err := b.build(ctx, deps, dfPath, contextDir, cb); err != nil {
		return "", err
	}
	defer b.untag(context.WithoutCancel(ctx), req, pb, deps.Tag)

	ignore, err := ProjectIgnore(req.ProjectContext, req.ProjectExcludes)
	if err != nil {
		return "", err
	}
	projectDF := filepath.Join(req.WorkDir, projectDockerfileName)
	if err := os.WriteFile(projectDF, []byte("FROM "+deps.Tag+"\nCOPY --chown=astro:0 . .\n"), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", projectDF, err)
	}
	ignorePath := projectDF + ignoreFile
	if err := os.WriteFile(ignorePath, []byte(ignore), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", ignorePath, err)
	}

	// No --pull: the base is the image just built, which no registry has. No
	// secrets: this step runs nothing.
	args := pb.args(projectDF, ignorePath, req)
	run := req
	run.Env = append(append([]string{}, req.Env...), pb.env...)
	if err := b.stream(ctx, run, args, cb); err != nil {
		return "", fmt.Errorf("copying the project into the image failed; see the build output above: %w", err)
	}
	return req.Tag, nil
}

// ErrNoProjectBuilder reports an engine that cannot be shown to read the
// ignore file a ProjectContext build copies the project under, which keeps
// .env, airflow_settings.yaml and the rest out of an image bound for a
// registry. Such a build is refused, never run without it; CanShipProject
// lets a caller find out first and fall back to a build that ships no project
// files.
var ErrNoProjectBuilder = errors.New("copying the project into the image needs a builder that reads the ignore file keeping .env, airflow_settings.yaml and other local files out of it")

// CanShipProject reports whether this engine can run a ProjectContext build,
// and why not when it cannot (wrapping ErrNoProjectBuilder). req needs only
// Bin and Env.
func (b *Builder) CanShipProject(ctx context.Context, req Request) error {
	_, err := b.projectBuilder(ctx, req)
	return err
}

// projectBuilder is how a ProjectContext build runs on this engine.
type projectBuilder struct {
	podman bool
	// builder is the buildx builder both Docker builds run on.
	builder string
	env     []string
}

// args is the command line of the second build.
func (p projectBuilder) args(dockerfile, ignorePath string, req Request) []string {
	var args []string
	if p.podman {
		// Podman reads the ignore file it is pointed at; it does not look for
		// <Dockerfile>.dockerignore.
		args = []string{"build", "--tag", req.Tag, "--file", dockerfile, "--ignorefile", ignorePath}
	} else {
		args = append(buildxArgs(p.builder), "--tag", req.Tag, "--file", dockerfile)
	}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	return append(args, req.ProjectContext)
}

// buildxArgs starts a build on the named buildx builder: BuildKit or nothing,
// never the legacy builder, which ignores <Dockerfile>.dockerignore (it is
// what `docker build` runs under DOCKER_BUILDKIT=0, before Docker 23 on Linux,
// or without the buildx plugin). --load puts the image in the local store.
func buildxArgs(builder string) []string {
	return []string{"buildx", "build", "--builder", builder, "--load"}
}

// projectBuilder finds out how this engine runs a ProjectContext build, and
// refuses one that cannot be shown to read its ignore file.
//
// Docker needs the buildx plugin, and runs both builds on the builder of the
// current Docker context (`docker context show`), whose driver must be
// "docker". That builder exists for every context and shares the engine's
// image store, so the second build can start FROM the first. The user's
// selected builder may not: a docker-container, remote or kubernetes builder
// (what docker/setup-buildx-action selects in CI) cannot see a local image.
// The builder named "default" is not the answer either: it belongs to the
// context called default, not to the one in use.
//
// Podman is recognized by what `<bin> --version` says, so the podman-docker
// shim, a `docker` that runs podman, is podman too. It must be local
// (`podman info` reports no remote service) and its build must offer
// --ignorefile. A remote client, which is how podman runs on macOS and
// Windows, sends the build context to the machine, and whether it applies
// --ignorefile when it does is not something this can check, so it is
// treated as unable to.
func (b *Builder) projectBuilder(ctx context.Context, req Request) (projectBuilder, error) {
	out := func(args ...string) (string, error) {
		var o strings.Builder
		err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &o}, req.Bin, args...)
		return strings.TrimSpace(o.String()), err
	}
	version, err := out("--version")
	if err != nil {
		return projectBuilder{}, fmt.Errorf("%w; `%s --version` failed: %w", ErrNoProjectBuilder, req.Bin, err)
	}
	if strings.Contains(strings.ToLower(version), "podman") {
		return podmanBuilder(out)
	}
	if _, err := out("buildx", "version"); err != nil {
		return projectBuilder{}, fmt.Errorf("%w (Docker BuildKit), and `%s buildx version` failed. Install the Docker buildx plugin (docker-buildx), or use Docker Desktop or Docker Engine 23 or newer", ErrNoProjectBuilder, req.Bin)
	}
	name, err := out("context", "show")
	if err != nil || name == "" {
		return projectBuilder{}, fmt.Errorf("%w, and `%s context show` named no Docker context", ErrNoProjectBuilder, req.Bin)
	}
	inspect, err := out("buildx", "inspect", name)
	if err != nil || inspectDriver(inspect) != "docker" {
		return projectBuilder{}, fmt.Errorf("%w, and the buildx builder %q of the current Docker context is not one with the docker driver", ErrNoProjectBuilder, name)
	}
	return projectBuilder{builder: name, env: []string{"DOCKER_BUILDKIT=1"}}, nil
}

// podmanBuilder is projectBuilder for podman, out running podman commands.
func podmanBuilder(out func(args ...string) (string, error)) (projectBuilder, error) {
	remote, err := out("info", "--format", "{{.Host.ServiceIsRemote}}")
	if err != nil || remote != "false" {
		return projectBuilder{}, fmt.Errorf("%w, and this podman is a remote client (a podman machine), which may not apply --ignorefile to the context it sends. Install the Docker buildx plugin and use Docker, or build on Linux with a local podman", ErrNoProjectBuilder)
	}
	help, err := out("build", "--help")
	if err != nil || !strings.Contains(help, "--ignorefile") {
		return projectBuilder{}, fmt.Errorf("%w, and this podman's build has no --ignorefile; upgrade podman", ErrNoProjectBuilder)
	}
	return projectBuilder{podman: true}, nil
}

// inspectDriver reads the Driver line of `docker buildx inspect`.
func inspectDriver(inspect string) string {
	for _, line := range strings.Split(inspect, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Driver" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// depsTag is the intermediate tag of a ProjectContext build's dependency
// step: the final tag with "-deps" on its tag part, in the same repository.
func depsTag(tag string) string {
	name, t := tag, "latest"
	if i := strings.LastIndex(tag, ":"); i > strings.LastIndex(tag, "/") {
		name, t = tag[:i], tag[i+1:]
	}
	return name + ":" + t + "-deps"
}

// untagTimeout bounds the removal of the intermediate tag, which runs after
// the build whether or not the build's context was canceled.
const untagTimeout = 30 * time.Second

// untag drops ref, an intermediate tag, leaving the image it names in place
// as the parent of what was built over it. It runs on a context of its own,
// so an interrupted build does not leave a runtime-sized tag behind. Best
// effort: a leftover tag changes nothing.
func (b *Builder) untag(ctx context.Context, req Request, pb projectBuilder, ref string) {
	ctx, cancel := context.WithTimeout(ctx, untagTimeout)
	defer cancel()
	args := []string{"image", "rm", "--no-prune", ref}
	if pb.podman {
		// podman refuses to remove an image another one is built on, even by
		// name, and drops the name with untag instead.
		args = []string{"untag", ref, ref}
	}
	//nolint:errcheck // best effort; a leftover tag changes nothing
	b.cmd.Run(ctx, req.Env, rt.Stdio{}, req.Bin, args...)
}

// gitignoredShown caps how many gitignored files GitignoredWarning names.
const gitignoredShown = 10

// GitignoredWarning is the warning for project files a build will copy into
// the image although git ignores them: the image takes the project as a
// docker context does, under .dockerignore, and .gitignore is not that file.
// paths are project-relative; "" when there are none.
func GitignoredWarning(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	shown := paths
	more := ""
	if len(shown) > gitignoredShown {
		shown = shown[:gitignoredShown]
		more = fmt.Sprintf(" and %d more", len(paths)-gitignoredShown)
	}
	return fmt.Sprintf("the image will carry %d file(s) that .gitignore ignores and .dockerignore does not: %s%s. Add them to .dockerignore to keep them out of the image",
		len(paths), strings.Join(shown, ", "), more)
}
