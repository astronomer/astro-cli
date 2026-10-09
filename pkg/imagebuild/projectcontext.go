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
// a builder known to read it (projectBuilder). The engine is asked before the
// first build, so a refusal costs nothing.
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

// errNoProjectBuilder refuses a ProjectContext build on an engine that cannot
// be shown to read the ignore file, rather than risk copying .env and the
// rest of what it leaves out into an image bound for a registry.
var errNoProjectBuilder = errors.New("this build copies the project into the image, and needs a builder that reads the ignore file keeping .env, airflow_settings.yaml and other local files out of it")

// projectBuilder is how the second build of a ProjectContext build runs on
// this engine.
type projectBuilder struct {
	podman bool
	env    []string
}

// args is the command line of the second build.
func (p projectBuilder) args(dockerfile, ignorePath string, req Request) []string {
	var args []string
	if p.podman {
		// Podman reads the ignore file it is pointed at; it does not look for
		// <Dockerfile>.dockerignore.
		args = []string{"build", "--tag", req.Tag, "--file", dockerfile, "--ignorefile", ignorePath}
	} else {
		// buildx, never the legacy builder: only BuildKit reads
		// <Dockerfile>.dockerignore, and `docker build` falls back to the
		// legacy builder under DOCKER_BUILDKIT=0, before Docker 23 on Linux,
		// or without the buildx plugin, where it would read the project's
		// .dockerignore alone. `docker buildx build` is BuildKit or nothing.
		// --load puts the image in the local store whatever the driver.
		args = []string{"buildx", "build", "--load", "--tag", req.Tag, "--file", dockerfile}
	}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	return append(args, req.ProjectContext)
}

// projectBuilder finds out how this engine runs the second build, and
// refuses one that cannot be shown to read its ignore file.
//
// Podman is recognized by what `<bin> --version` says, so the podman-docker
// shim, a `docker` that runs podman, is podman too; its build must offer
// --ignorefile. Docker must have the buildx plugin: `docker buildx version`
// has to answer.
func (b *Builder) projectBuilder(ctx context.Context, req Request) (projectBuilder, error) {
	var version strings.Builder
	if err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &version}, req.Bin, "--version"); err != nil {
		return projectBuilder{}, fmt.Errorf("%w; `%s --version` failed: %w", errNoProjectBuilder, req.Bin, err)
	}
	if strings.Contains(strings.ToLower(version.String()), "podman") {
		var help strings.Builder
		err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &help}, req.Bin, "build", "--help")
		if err != nil || !strings.Contains(help.String(), "--ignorefile") {
			return projectBuilder{}, fmt.Errorf("%w, and this podman's build has no --ignorefile; upgrade podman", errNoProjectBuilder)
		}
		return projectBuilder{podman: true}, nil
	}
	if err := b.cmd.Run(ctx, req.Env, rt.Stdio{}, req.Bin, "buildx", "version"); err != nil {
		return projectBuilder{}, fmt.Errorf("%w (Docker BuildKit), and `%s buildx version` failed; install the Docker buildx plugin, or use Docker Desktop or Docker Engine 23 or newer", errNoProjectBuilder, req.Bin)
	}
	return projectBuilder{env: []string{"DOCKER_BUILDKIT=1"}}, nil
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
