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
	// containerIgnoreFile is podman's and buildah's own name for it, which
	// they read before .dockerignore.
	containerIgnoreFile = ".containerignore"
)

// ProjectBuilder is how this engine runs a ProjectContext build, as
// CanShipProject found it. Its zero value is "not asked yet".
type ProjectBuilder struct {
	ok     bool
	podman bool
	// builder is the buildx builder both Docker builds run on.
	builder string
	env     []string
}

// projectIgnoreRules are left out of every ProjectContext copy, after the
// project's own rules, so a "!" there cannot bring one back.
//
//   - Everything the 1.x CLI's default .dockerignore left out (`astro dev
//     init`'s template: astro, .git, .env, airflow_settings.yaml, logs/,
//     .venv, airflow.db, airflow.cfg), so a project converted from 1.x ships
//     no less safely than it did.
//   - The per-machine files v2's tools and Python write: .astro/ (standalone
//     Airflow's state, local overrides, Otto's tokens), .venv, .env, .env.*,
//     .envrc, bytecode, and the pickling fix the standalone engine drops into
//     plugins/ for Airflow 2 on macOS (pkg/scaffold's pickleFixRule).
//   - The project's own requirements.txt and packages.txt, which a 1.x layout
//     may still carry. The generated ones are what the image installed, and
//     stay in AIRFLOW_HOME.
//
// What can hold a secret or a machine's state wherever it sits is left out
// at any depth (**/): a .git directory or a submodule's .git file (a remote
// URL with a token in it), airflow_settings.yaml (connection secrets in clear
// text), .astro, .env files, virtualenvs and bytecode. The rest stay anchored
// at the root, as 1.x had them: astro, logs, airflow.db and airflow.cfg are
// what a 1.x project kept at its root as AIRFLOW_HOME, while a logs/
// directory or an airflow.cfg template under include/ is the project's own
// data, which 1.x shipped.
//
// No rule starts with "/": .dockerignore patterns are anchored at the context
// root already, and buildah's parser and Docker's agree on that spelling.
var projectIgnoreRules = []string{
	"astro",
	"logs",
	"airflow.db",
	"airflow.cfg",
	"**/.git",
	"**/airflow_settings.yaml",
	"**/.astro",
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

// ProjectIgnore is the ignore file a ProjectContext copy is built with on
// this engine: the project's own rules, then projectIgnoreRules, then
// excludes. It is written beside the generated Dockerfile as
// <Dockerfile>.dockerignore, which BuildKit reads instead of the context's
// own, and handed to podman with --ignorefile, so the project's file is never
// edited. A deploy and `astro package astro` survey the project under it.
//
// The project's own rules are the file the engine would read itself: Docker's
// .dockerignore, and on podman .containerignore when there is one, which
// podman and buildah read before .dockerignore.
func (p ProjectBuilder) ProjectIgnore(projectDir string, excludes []string) (string, error) {
	name := ignoreFile
	if p.podman {
		if _, err := os.Stat(filepath.Join(projectDir, containerIgnoreFile)); err == nil {
			name = containerIgnoreFile
		}
	}
	own, err := os.ReadFile(filepath.Join(projectDir, name))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading the project's %s: %w", name, err)
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
// a builder shown to read it (CanShipProject), asked before the first build,
// so a refusal costs nothing. A caller that asked already hands the answer in
// as req.Builder, and the engine is not asked again. On Docker both builds run on the
// docker-driver builder of the current context, which reads the local image
// store, so the second finds the first.
//
// The intermediate tag is removed once the second build has run, whether or
// not it succeeded, and even when ctx was canceled; its layers stay, as the
// final image's parent.
func (b *Builder) buildProject(ctx context.Context, req Request, dfPath, contextDir string, cb rt.Callbacks) (string, error) {
	pb := req.Builder
	if !pb.ok {
		var err error
		if pb, err = b.CanShipProject(ctx, req); err != nil {
			return "", err
		}
	}
	deps := req
	deps.Tag = depsTag(req.Tag)
	deps.builder = pb.builder
	if _, err := b.build(ctx, deps, dfPath, contextDir, cb); err != nil {
		return "", err
	}
	defer b.untag(context.WithoutCancel(ctx), req, pb, deps.Tag)

	ignore, err := pb.ProjectIgnore(req.ProjectContext, req.ProjectExcludes)
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

	// The project may have changed during the dependency build, which takes
	// minutes: the caller looks again, at what is about to be sent.
	if req.BeforeProjectCopy != nil {
		if err := req.BeforeProjectCopy(); err != nil {
			return "", err
		}
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
// lets a caller find out first.
var ErrNoProjectBuilder = errors.New("copying the project into the image needs a builder that reads the ignore file keeping .env, airflow_settings.yaml and other local files out of it")

// args is the command line of the second build.
func (p ProjectBuilder) args(dockerfile, ignorePath string, req Request) []string {
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

// CanShipProject finds out how this engine runs a ProjectContext build, and
// refuses one that cannot be shown to read its ignore file (wrapping
// ErrNoProjectBuilder). req needs only Bin and Env. Hand the answer to the
// build as Request.Builder, so the engine is asked once.
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
// shim, a `docker` that runs podman, is podman too, and its build must offer
// --ignorefile.
//
// Then, on either, a check build proves the ignore file is honored, rather
// than a version taken on trust (probeIgnoreFile).
func (b *Builder) CanShipProject(ctx context.Context, req Request) (ProjectBuilder, error) {
	out := func(args ...string) (string, error) {
		var o strings.Builder
		err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &o}, req.Bin, args...)
		return strings.TrimSpace(o.String()), err
	}
	version, err := out("--version")
	if err != nil {
		return ProjectBuilder{}, fmt.Errorf("%w; `%s --version` failed: %w", ErrNoProjectBuilder, req.Bin, err)
	}
	var pb ProjectBuilder
	if strings.Contains(strings.ToLower(version), "podman") {
		help, err := out("build", "--help")
		if err != nil || !strings.Contains(help, "--ignorefile") {
			return ProjectBuilder{}, fmt.Errorf("%w, and this podman's build has no --ignorefile; upgrade podman", ErrNoProjectBuilder)
		}
		pb = ProjectBuilder{podman: true}
	} else {
		if _, err := out("buildx", "version"); err != nil {
			return ProjectBuilder{}, fmt.Errorf("%w (Docker BuildKit), and `%s buildx version` failed. Install the Docker buildx plugin (docker-buildx), or use Docker Desktop or Docker Engine 23 or newer", ErrNoProjectBuilder, req.Bin)
		}
		name, err := out("context", "show")
		if err != nil || name == "" {
			return ProjectBuilder{}, fmt.Errorf("%w, and `%s context show` named no Docker context", ErrNoProjectBuilder, req.Bin)
		}
		inspect, err := out("buildx", "inspect", name)
		if err != nil || inspectDriver(inspect) != "docker" {
			return ProjectBuilder{}, fmt.Errorf("%w, and the buildx builder %q of the current Docker context is not one with the docker driver", ErrNoProjectBuilder, name)
		}
		pb = ProjectBuilder{builder: name, env: []string{"DOCKER_BUILDKIT=1"}}
	}
	if err := b.probeIgnoreFile(ctx, req, pb); err != nil {
		return ProjectBuilder{}, err
	}
	pb.ok = true
	return pb, nil
}

// probe file names: the check build's context holds both at its root and in
// a subdirectory, and its ignore file leaves out the second at any depth.
const (
	probeKeep   = "keep"
	probeMarker = "left-out"
	probeSub    = "sub"
)

// probeIgnoreFile runs a check build the way the second build runs, and fails
// unless the builder applied the ignore file the CLI hands it.
//
// The context holds a kept file and a marker, at its root and in a
// subdirectory, and an ignore file of its own (.dockerignore, and on podman
// .containerignore too) that leaves out neither. The CLI's file
// (<Dockerfile>.dockerignore, or --ignorefile) leaves out the marker with a
// "**/" rule. The result, exported to a directory rather than an image, must
// hold both kept files and neither marker: proof that the CLI's file is read,
// that it wins over the context's own, and that "**/" rules match at the root
// and below it.
//
// It proves what a version number would only suggest, on this engine, this
// builder and this connection (a remote podman machine included). It costs a
// FROM scratch build of a few tiny files, a fraction of a second, writes no
// image, and runs once per build: the answer travels with Request.Builder.
func (b *Builder) probeIgnoreFile(ctx context.Context, req Request, pb ProjectBuilder) error {
	dir, err := os.MkdirTemp("", "astro-ignore-check-")
	if err != nil {
		return fmt.Errorf("%w; making the check build's directory: %w", ErrNoProjectBuilder, err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best-effort removal of the check build
	ctxDir, outDir := filepath.Join(dir, "context"), filepath.Join(dir, "out")
	df := filepath.Join(dir, "Dockerfile.check")
	files := map[string]string{
		filepath.Join(ctxDir, probeKeep):             "kept\n",
		filepath.Join(ctxDir, probeMarker):           "left out\n",
		filepath.Join(ctxDir, probeSub, probeKeep):   "kept\n",
		filepath.Join(ctxDir, probeSub, probeMarker): "left out\n",
		filepath.Join(ctxDir, ignoreFile):            "unrelated\n",
		filepath.Join(ctxDir, containerIgnoreFile):   "unrelated\n",
		df:              "FROM scratch\nCOPY . /\n",
		df + ignoreFile: "**/" + probeMarker + "\n",
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), contextDirPerm); err != nil {
			return fmt.Errorf("%w; writing the check build: %w", ErrNoProjectBuilder, err)
		}
		if err := os.WriteFile(path, []byte(body), filePermRW); err != nil {
			return fmt.Errorf("%w; writing the check build: %w", ErrNoProjectBuilder, err)
		}
	}
	args := []string{"buildx", "build", "--builder", pb.builder, "--file", df}
	if pb.podman {
		args = []string{"build", "--file", df, "--ignorefile", df + ignoreFile}
	}
	args = append(args, "--output", "type=local,dest="+outDir, ctxDir)
	env := append(append([]string{}, req.Env...), pb.env...)
	var output strings.Builder
	if err := b.cmd.Run(ctx, env, rt.Stdio{Out: &output, Err: &output}, req.Bin, args...); err != nil {
		return fmt.Errorf("%w; a check build of the ignore file failed: %w: %s", ErrNoProjectBuilder, err, strings.TrimSpace(output.String()))
	}
	for _, kept := range []string{probeKeep, filepath.Join(probeSub, probeKeep)} {
		if _, err := os.Stat(filepath.Join(outDir, kept)); err != nil {
			return fmt.Errorf("%w; a check build of the ignore file did not copy %s", ErrNoProjectBuilder, filepath.ToSlash(kept))
		}
	}
	for _, left := range []string{probeMarker, filepath.Join(probeSub, probeMarker)} {
		if _, err := os.Stat(filepath.Join(outDir, left)); err == nil {
			return fmt.Errorf("%w; a check build copied %s, which the ignore file the CLI handed it leaves out, so this builder does not read that file", ErrNoProjectBuilder, filepath.ToSlash(left))
		}
	}
	return nil
}

// IsPodman reports whether the engine req.Bin runs is podman, the
// podman-docker shim included, by what `--version` says. req needs only Bin
// and Env.
func (b *Builder) IsPodman(ctx context.Context, req Request) bool {
	var o strings.Builder
	if err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: &o}, req.Bin, "--version"); err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(o.String()), "podman")
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
func (b *Builder) untag(ctx context.Context, req Request, pb ProjectBuilder, ref string) {
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
