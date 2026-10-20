// Package imagebuild builds a deployable Airflow image from a manifest's
// fields. It assembles a small build context (the project's pyproject.toml and
// uv.lock, and the install helper that reads them) and builds it over the
// runtime's -base image, the build without ONBUILD triggers, with the install
// steps those triggers would run written out in the Dockerfile it generates
// (InstallDockerfile).
//
// A project may instead declare its own Dockerfile in the manifest, which the
// project design calls tier 3: the escape hatch for multi-stage builds and
// anything else a manifest cannot express, at the stated cost of portability.
// Request.Dockerfile switches to that mode, where the file is the build and
// nothing here is generated. Both modes are supported; neither is a ramp off
// the other.
//
// It was lifted out of internal/localdocker so the manifest deploy path can build the
// same image the local Docker engine builds (docs/deploy.md,
// section 1). Its inputs are manifest-shaped — Python dependencies, OS packages,
// a runtime base image, a tag — so it depends on neither localdocker nor a
// deploy package; each caller resolves the manifest to a Request and hands it
// over. Build itself takes an already-resolved BaseImage; RuntimeImage maps an
// Airflow version to that base and lives here now that both the local Docker
// engine and deploy resolve it the same way.
//
// The rest of an image's life on the container engine is here too, so every
// image action runs on one engine: Engine logs in to a registry, tags, pushes
// (returning the pushed digest when asked), saves to a tarball and reads a
// label, through the same
// Commander, Bin and Env a build takes. Which registry, which credential and
// which tag scheme stay with the caller.
//
// Per the layer rules (docs/architecture.md) it prints nothing and never
// exits: build output flows through rt.Callbacks, or raw to Request.Stdout
// and Request.Stderr when a caller sets them, and a failed install returns a
// named error.
package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// RuntimeImageRepo hosts Astro Runtime 3 images (Airflow 3). The tag is the
// runtime version, e.g. "3.1-2" or a floating "3.1" the registry serves.
const RuntimeImageRepo = "astrocrpublic.azurecr.io/runtime"

// RuntimeImage maps an Airflow version to the runtime base image a build starts
// FROM. The version is the pin a manifest's Airflow requirement states, reduced to its series by
// AirflowSeries: "3.1" and "3.1.2" both build FROM runtime:3.1, and a pinned
// runtime tag ("3.1-2") passes through as the tag. A bare major ("3") is
// refused, naming the pin. Only Airflow 3 ships as Astro Runtime 3, so anything
// else is refused; the built image's io.astronomer label gives back the exact
// version. Deploy, `astro package astro` and the local Docker engine all
// resolve an Airflow 3 base this way; an Airflow 2 one, whose tag is a runtime
// version, needs the catalog (RuntimeImageForPython).
func RuntimeImage(airflowVersion string) (string, error) {
	v := strings.TrimSpace(airflowVersion)
	if v == "" {
		return "", errors.New("no Airflow version was given; one is needed to pick a runtime image")
	}
	if major, _, _ := strings.Cut(v, "."); major != "3" {
		return "", fmt.Errorf("only Airflow 3 is supported in this release, not %q", v)
	}
	series, ok := AirflowSeries(v)
	if !ok {
		return "", fmt.Errorf("the Airflow pin %q in pyproject.toml names no minor version, so there is no runtime image to build from. Pin the apache-airflow requirement in [project] dependencies to a series such as apache-airflow==3.3.*", v)
	}
	return RuntimeImageRepo + ":" + series, nil
}

// RuntimeImageFor is RuntimeImage for a manifest that may name one runtime
// build ([tool.astro] runtime, manifest.Airflow().Runtime). With runtime empty
// it is RuntimeImage. With it set, the base is that build, runtime:<runtime>,
// rather than the newest build the pin's series tag serves, and the pin need
// only be an Airflow 3: a pin naming just the generation ("3") has no series
// tag to build from, but the build is named outright.
//
// The manifest has already held the build to the pin: an Airflow 3 tag of
// another series does not load. Whether the catalog lists the build, which
// exact Airflow it carries and whether it is yanked are
// runtimeversions.CheckRuntime's, which the caller runs where it can report
// them.
func RuntimeImageFor(airflowVersion, runtime string) (string, error) {
	runtime = strings.TrimSpace(runtime)
	if runtime == "" {
		return RuntimeImage(airflowVersion)
	}
	v := strings.TrimSpace(airflowVersion)
	if major, _, _ := strings.Cut(v, "."); major != "3" {
		return "", fmt.Errorf("only Airflow 3 is supported in this release, not %q", v)
	}
	return RuntimeImageRepo + ":" + runtime, nil
}

// AirflowSeries reduces a manifest's Airflow pin to the MAJOR.MINOR series a
// runtime image is published under, and reports whether the pin named one.
//
// pkg/manifest accepts "3", "3.1" and "3.1.2" as pins, but the runtime image
// tag is built by concatenation, and runtime:3.1.2 and runtime:3 are not
// published tags. Used as written, either fails the build with a registry
// manifest-not-found that says nothing about the manifest.
//
// So a patch pin resolves to its series: the patch is decided by the image the
// series tag serves, as it is for a Dockerfile's `FROM runtime:3.1`. A pin
// naming only a major returns ok=false, because there is no series to choose
// and guessing one would silently move a project between Airflow minors.
//
// Whitespace around the pin is trimmed, and the minor segment is kept whole,
// so a runtime tag such as "3.1-2" is its own series. Policy (which
// generations are accepted) stays with the caller.
func AirflowSeries(pin string) (series string, ok bool) {
	parts := strings.Split(strings.TrimSpace(pin), ".")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "." + parts[1], true
}

// ErrDockerfileNotAFile reports that a declared Dockerfile names something
// other than a regular file, a directory most often. A caller that knows the
// file by another name can tell it apart with errors.Is and say so in its own
// words; a Dockerfile that is missing or unreadable wraps the *fs.PathError
// of its stat instead.
var ErrDockerfileNotAFile = errors.New("not a file")

// ErrDockerfileBuild reports that the build of the project's own Dockerfile
// failed, rather than the install of a generated build.
var ErrDockerfileBuild = errors.New("building the project's Dockerfile failed; see the build output above")

const (
	// dockerfileName is the Dockerfile a generated build runs
	// (InstallDockerfile).
	dockerfileName = "Dockerfile.astro-local"
	// buildContextDir is the sub-directory of a Request's WorkDir that holds
	// the build context: the inputs the install helper reads, and the
	// project's files when the image ships.
	buildContextDir = "deps-build"
	// requirementsName and packagesName are the names the runtime's install
	// scripts read from their working directory, and lockConstraintsName the
	// file of uv.lock's versions requirements.txt names with -c. The install
	// helper writes all three (install-project.py's REQUIREMENTS, PACKAGES and
	// LOCK_CONSTRAINTS).
	requirementsName    = "requirements.txt"
	packagesName        = "packages.txt"
	lockConstraintsName = "uv-lock-constraints.txt"
	// baseTagSuffix turns a runtime tag into its -base tag (3.1-5 to
	// 3.1-5-base, 12.6.0-python-3.11 to 12.6.0-python-3.11-base), the same
	// image without the ONBUILD triggers. The runtime publishes one beside
	// every tag, in the same repository: scripts/check-runtime-base-tags.sh
	// checks each one the CLI can pick.
	baseTagSuffix = "-base"
	// contextDirPerm is owner-only, matching internal/localstate.
	contextDirPerm = 0o700
)

// Commander runs the container build. It is the only path to a container
// runtime, so callers inject one (localdocker's engine Commander satisfies it)
// and tests substitute a fake that never touches a real daemon. env is
// appended to the process environment — the DOCKER_HOST/CONTAINER_HOST pair
// that reaches a podman machine.
type Commander interface {
	// Run runs the command wired to the given stdio. Nil readers/writers are
	// allowed and mean "none"/discard.
	Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error
}

// DirCommander is a Commander that can also run a command in a given working
// directory, which a Request with Dir set needs. It is a separate interface so
// every existing Commander stays one.
type DirCommander interface {
	Commander
	// RunIn is Run with the command's working directory set to dir.
	RunIn(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error
}

// filePermRW is the mode for the files written into the build context:
// owner read/write. Was pkg/proxy's FilePermRW, borrowed for the constant alone —
// a file mode is not a contract worth a module dependency, and taking one on the
// proxy from an image builder was coupling with nothing behind it.
const filePermRW = 0o600

// Request describes an image to build from a manifest's fields. It carries no
// localdocker or deploy types, so either caller can fill it. ForManifest fills
// the fields that decide which image a manifest builds.
type Request struct {
	// WorkDir is the directory the build context and Dockerfile are written
	// under; the caller owns its location and lifetime. Unused in Dockerfile
	// mode, which writes nothing.
	WorkDir string
	// BaseImage is the resolved runtime image the build is of
	// (astrocrpublic.azurecr.io/runtime:<version>). The build starts FROM its
	// -base tag (baseTagSuffix); BaseImage itself stays the runtime's own tag,
	// which is what callers parse the runtime version out of and name in
	// messages. Empty in Dockerfile mode: the project's own file declares what
	// it builds on.
	BaseImage string
	// Secrets are docker build --secret specs ("id=mysecret[,src=/local/secret]"
	// or "id=mysecret,env=ENV_VAR"),
	// one per entry, forwarded verbatim in the order given.
	//
	// A generated build forwards only the manifest.RuntimeSecretID spec: the
	// one RUN that can read a secret is the Python install, which mounts that
	// one.
	//
	// The SPEC is forwarded, never a secret value: docker reads the value itself
	// from the src file or the named env var. So these strings are safe in a
	// command line and in the build log, which is where they end up.
	Secrets []string
	// Dockerfile and Context switch this into Dockerfile mode — the project
	// supplied a real Dockerfile ("tier 3" in the project design) and it, not
	// the manifest, is the build. Both are absolute, and set together.
	//
	// The two modes share little. This one writes nothing, because the file
	// owns its own installs; it takes its FROM from the file rather than
	// BaseImage; and it builds the PROJECT as its context, because a
	// multi-stage build COPYs from the repo. The generated mode's context is
	// synthetic: pyproject.toml, uv.lock and the install helper
	// (writeInstallInputs), plus, for an image that ships, a filtered copy of
	// the project (ProjectFiles), which the generated Dockerfile's `COPY . .`
	// lays down after both installs.
	//
	// Dependencies and Packages are ignored here rather than rejected. A caller
	// reading a manifest has them populated whichever tier the project
	// chose, and a Dockerfile project installs its own.
	Dockerfile string
	Context    string
	// Tag is the image reference the build produces.
	Tag string
	// ManifestDir is the project directory whose pyproject.toml, and uv.lock
	// when it has one, a generated build installs from: the install helper
	// reads them inside the build (InstallDockerfile), so the image holds every
	// package at the version uv.lock resolved. ForManifest sets it for a
	// generated build. Empty, or naming a directory with no pyproject.toml, the
	// build writes a pyproject.toml of Dependencies, FindLinks and Packages
	// instead, and installs them unpinned. Ignored in Dockerfile mode.
	ManifestDir string
	// Dependencies are the manifest's Python dependencies (PEP 508);
	// apache-airflow is dropped, since the runtime base already provides it.
	// With ManifestDir set they decide only whether there is anything to
	// install (Build's fast path); the install reads pyproject.toml.
	Dependencies []string
	// FindLinks are pages the Python install searches besides PyPI, written
	// into the generated requirements.txt as --find-links lines: the
	// per-package pages of the indexes a [tool.uv.sources] entry routes a
	// dependency to (manifest.UV.IndexPages). Ignored in Dockerfile mode, and
	// with ManifestDir set, whose pyproject.toml states them.
	FindLinks []string
	// Packages are the manifest's OS (apt) package names.
	Packages []string
	// ProjectFiles is the project directory whose files a generated build
	// carries into the image (/usr/local/airflow), for an image that is
	// shipped: include/, plugins/ and the rest of the project's code, which a
	// deploy does not otherwise send. ForManifest sets it; local Docker mode
	// runs the project mounted and leaves it empty, so its image holds only
	// the dependencies. Ignored in Dockerfile mode, whose context is already
	// the project. See copyProjectFiles for what is left out.
	ProjectFiles string
	// Labels are image labels ("key=value"), each passed to the build as a
	// --label, in the order given, in either mode. A deploy marks its image
	// with them for the control plane that reads them; nil adds none.
	Labels []string
	// Platform is the build platform (e.g. "linux/amd64"), or several joined
	// with commas ("linux/amd64,linux/arm64"), passed to --platform as given.
	// Empty builds for the host — what local Docker mode wants; the deploy
	// path pins linux/amd64.
	Platform string
	// Format is the image format podman builds (its --format: "docker" or
	// "oci"). Empty passes no --format, which is the only thing to pass to
	// docker, whose build has no such flag. The remote-execution client image
	// is built in docker format under podman, as the Astro CLI 1.x built it.
	Format string
	// Bin is the container CLI to shell out to; Env reaches its daemon.
	Bin string
	Env []string
	// Dir is the build command's working directory; empty is the process's.
	// A relative Dir, and the relative WorkDir, Dockerfile and Context of a
	// request that sets it, are resolved against the process's working
	// directory, not against Dir; the command line carries them absolute.
	// Docker resolves what the command line leaves relative against it — a
	// build secret's src above all — so a caller whose secrets are written
	// relative to the build context sets it there. It is not WorkDir, which
	// is where a generated build's files are written and says nothing about
	// where docker runs. Setting it needs a Commander that is a DirCommander,
	// as NewExecCommander's is.
	Dir string
	// Stdout and Stderr, when set, take the build's own stdout and stderr
	// raw, as the container CLI writes them, in place of the lines Build
	// otherwise hands to its callbacks. Either may be set alone; a stream
	// left nil reaches the callbacks as before.
	Stdout io.Writer
	Stderr io.Writer
}

// Builder builds images. cmd and now are seams: production values come from
// New, tests replace them so no build ever touches a real daemon.
type Builder struct {
	cmd Commander
	now func() time.Time
}

// New builds a Builder over the given command runner and clock.
func New(cmd Commander, now func() time.Time) *Builder {
	return &Builder{cmd: cmd, now: now}
}

// NewExecCommander returns the production Commander, backed by os/exec. A caller
// with no daemon seam of its own (the deploy path) uses it; localdocker keeps
// its own runner because it also needs command output.
func NewExecCommander() Commander { return execCommander{} }

type execCommander struct{}

func (c execCommander) Run(ctx context.Context, extraEnv []string, s rt.Stdio, name string, args ...string) error {
	return c.RunIn(ctx, "", extraEnv, s, name, args...)
}

func (execCommander) RunIn(ctx context.Context, dir string, extraEnv []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	// A nil reader/writer means "none": os/exec routes a nil Stdout/Stderr to the
	// null device, so a silent probe needs no extra plumbing.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.In, s.Out, s.Err
	return cmd.Run()
}

// Build produces the image to run and returns its reference.
//
// Two modes. With req.Dockerfile set the project's own file is the build, and it
// is run as-is against the project as context (see the field's doc for why they
// share so little). Otherwise the request's dependencies and OS packages are
// installed into a layer over its base's -base image (InstallDockerfile),
// with the project's files over them when the request carries ProjectFiles.
// With nothing to install, no project files and no labels it builds nothing
// and returns the -base image unchanged (the fast path).
//
// Build output streams to cb.OnLine (component "build") as lines, unless
// req.Stdout or req.Stderr is set, in which case that stream goes to it raw;
// a failed build returns a named error, never a hang.
func (b *Builder) Build(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	return b.buildImage(ctx, req, cb, true)
}

// buildImage is Build, with the generated mode's fast path optional: with
// fastPath false a request with nothing to install still builds, producing
// req.Tag over the base rather than returning the base itself.
func (b *Builder) buildImage(ctx context.Context, req Request, cb rt.Callbacks, fastPath bool) (string, error) {
	if req.Dir != "" {
		var err error
		if req, err = absPaths(req); err != nil {
			return "", err
		}
	}
	if req.Dockerfile != "" {
		// Checked here rather than in each caller, because this is where they
		// meet: localdocker reaches it through the rt.ImageBuilder seam, and
		// deploy and package call it directly. A declaration that names nothing
		// is the one failure this mode can have that is entirely the manifest's
		// fault, and left to docker it arrives as build output that never
		// mentions the pyproject.toml key the user has to fix.
		//
		// A regular FILE, not merely something that exists: pkg/manifest
		// validates the declared path with filepath.IsLocal, which is lexical
		// and says yes to ".", so a Stat alone would pass the project directory
		// and the failure would land on `docker build -f <a directory>`.
		switch info, err := os.Stat(req.Dockerfile); {
		case err != nil:
			return "", fmt.Errorf("the Dockerfile this project declares (%s) could not be read: %w", req.Dockerfile, err)
		case !info.Mode().IsRegular():
			return "", fmt.Errorf("the Dockerfile this project declares (%s) is %w", req.Dockerfile, ErrDockerfileNotAFile)
		}
		// No fast path here. An unchanged Dockerfile is the daemon's layer cache
		// to short-circuit, not ours to skip: we cannot tell from the outside
		// whether the file's own steps would produce something new.
		return b.build(ctx, req, req.Dockerfile, req.Context, cb)
	}

	deps := runtimeDeps(req.Dependencies)
	from := baseOf(req.BaseImage)
	// The base image already provides Airflow, so a project with nothing beyond
	// Airflow and no OS packages needs no build and runs the base as-is: the
	// -base image, so a project pulls one tag whether it builds or not.
	// An image carrying the project's files, or labels of its own, is never
	// the bare base.
	if fastPath && len(deps) == 0 && len(req.Packages) == 0 && req.ProjectFiles == "" && len(req.Labels) == 0 {
		return from, nil
	}

	contextDir := filepath.Join(req.WorkDir, buildContextDir)
	// Started empty, so a file deleted from the project since an earlier build
	// in the same WorkDir is not carried into this one, and an install input
	// an earlier build wrote (a uv.lock since deleted) is not read by this one.
	// The directory is the build's own; local Docker mode reuses it every start.
	if err := os.RemoveAll(contextDir); err != nil {
		return "", fmt.Errorf("clearing %s: %w", contextDir, err)
	}
	if err := os.MkdirAll(contextDir, contextDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", contextDir, err)
	}
	if req.ProjectFiles != "" {
		if err := copyProjectFiles(req.ProjectFiles, contextDir); err != nil {
			return "", err
		}
	}
	// After the project's files, so the CLI's helper replaces a copy the
	// project carries.
	if err := writeInstallInputs(contextDir, req); err != nil {
		return "", err
	}
	// The Dockerfile sits outside the context so the trailing `COPY . .` does
	// not bake it in.
	dfPath := filepath.Join(req.WorkDir, dockerfileName)
	if err := os.WriteFile(dfPath, []byte(InstallDockerfile(from)), filePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", dfPath, err)
	}

	if _, err := b.build(ctx, req, dfPath, contextDir, cb); err != nil {
		return "", err
	}
	return req.Tag, nil
}

// build runs the container build for a resolved Dockerfile and context, and is
// the one place either mode shells out. Both want the same tag, the same
// platform rule and the same streaming; only the file, the context and whether
// the base is pulled differ, so keeping one call site is what stops the two
// modes drifting on flags.
func (b *Builder) build(ctx context.Context, req Request, dockerfile, contextDir string, cb rt.Callbacks) (string, error) {
	w := &rt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(rt.LogLine{Component: "build", Time: b.now(), Text: line})
		}
	}}
	// --pull keeps the base fresh for a floating tag; the daemon still caches
	// the install layer when the requirements file is unchanged. It is not
	// unconditional, because a declared Dockerfile's FROM may name something no
	// registry can serve — see shouldPull. A pinned platform (deploy wants
	// linux/amd64) is added only when set, so the host-platform local build
	// keeps its exact command.
	args := []string{"build", "--tag", req.Tag, "--file", dockerfile}
	if req.Format != "" {
		args = append(args, "--format", req.Format)
	}
	if shouldPull(req.Dockerfile, dockerfile) {
		args = append(args, "--pull")
	}
	for _, secret := range req.buildSecrets() {
		args = append(args, "--secret", secret)
	}
	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	args = append(args, contextDir)
	stdio := rt.Stdio{Out: w, Err: w}
	if req.Stdout != nil {
		stdio.Out = req.Stdout
	}
	if req.Stderr != nil {
		stdio.Err = req.Stderr
	}
	var err error
	if req.Dir == "" {
		err = b.cmd.Run(ctx, req.Env, stdio, req.Bin, args...)
	} else if dc, ok := b.cmd.(DirCommander); ok {
		err = dc.RunIn(ctx, req.Dir, req.Env, stdio, req.Bin, args...)
	} else {
		return "", fmt.Errorf("the build has to run in %s, and its runner cannot set a working directory", req.Dir)
	}
	w.Flush()
	if err != nil {
		// The two modes fail for different reasons and a user reading this has
		// to know which file to open: ours, or theirs.
		if req.Dockerfile != "" {
			return "", fmt.Errorf("%w: %w", ErrDockerfileBuild, err)
		}
		return "", fmt.Errorf("installing the project's dependencies into the runtime image failed; see the build output above: %w", err)
	}
	return req.Tag, nil
}

// absPaths makes a request that runs in Dir carry absolute paths: Dir itself,
// WorkDir, Dockerfile and Context, each resolved against the process's
// working directory as the caller wrote them. Left relative, the file checks
// here would read them against the process's directory while docker, running
// in Dir, read the same strings against Dir.
func absPaths(req Request) (Request, error) {
	for _, p := range []*string{&req.Dir, &req.WorkDir, &req.Dockerfile, &req.Context} {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return req, fmt.Errorf("resolving %s: %w", *p, err)
		}
		*p = abs
	}
	return req, nil
}

// buildSecrets are the Secrets the build can read. A declared Dockerfile
// mounts whichever it names. A generated build's Dockerfile mounts
// manifest.RuntimeSecretID alone (InstallDockerfile), so any other secret is
// dropped rather than handed to docker, which would read its source for
// nothing.
func (r *Request) buildSecrets() []string {
	if r.FromDeclaredDockerfile() {
		return r.Secrets
	}
	var out []string
	for _, spec := range r.Secrets {
		if s, err := manifest.ParseBuildSecret(spec); err == nil && s.ID == manifest.RuntimeSecretID {
			out = append(out, spec)
		}
	}
	return out
}

// baseOf is the image a generated build starts FROM for the runtime base ref:
// its -base tag, the same image as the runtime's tag without the ONBUILD
// triggers, whose steps InstallDockerfile writes out instead. It is the one
// place the suffix is added, after whichever rule resolved the base (the
// catalog, a platform's offer, a [tool.astro] runtime), so every caller of
// Build gets it. A ref already on a -base tag is returned as it is.
func baseOf(ref string) string {
	if strings.HasSuffix(ref, baseTagSuffix) {
		return ref
	}
	return ref + baseTagSuffix
}

// shouldPull reports whether the build may force-refresh its base images.
//
// Always, for a generated build: the Dockerfile is one this package wrote, its
// FROM is the runtime base, and the tag floats — so pulling is how a project
// picks up a new patch.
//
// For a DECLARED Dockerfile the FROM belongs to the user, and forcing a pull
// breaks any base that is not in a registry this machine can reach: a locally
// built image, or a private registry the daemon is not logged into. A plain
// `docker build` succeeds there and this failed, which is not a tradeoff a
// tier-3 escape hatch gets to make.
//
// The rule follows v1's image handler, which skipped --pull as soon as ANY FROM
// named something other than an Astro base. Any is the right quantifier rather
// than the final stage — a builder stage on an unreachable image fails the
// build just as hard. It does not read every Dockerfile as v1 did: v1 matched
// a case-sensitive "FROM " prefix at the start of the line against bare
// registry prefixes, so a lowercase or indented from, a FROM followed by a tab,
// a `FROM --platform=…` stage and a lookalike repository
// (quay.io/astronomerfoo/…) are each read differently here.
func shouldPull(declared, dockerfile string) bool {
	if declared == "" {
		return true
	}
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		// Unreadable is Build's guard to report, not this function's to guess
		// about; the safe answer is the one that adds no failure mode.
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		// Tokenized rather than prefix-matched. docker splits on any run of
		// whitespace, so `FROM<tab>image` is a valid instruction that a "FROM "
		// prefix misses — and a missed FROM is not a missed opportunity here, it
		// silently turns a user's unreachable base into a forced pull.
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		// Per-stage flags come before the reference: `FROM --platform=$BUILDPLATFORM
		// <image>` is common enough that treating the flag as the image would drop
		// base freshness for Dockerfiles that are on an Astro base.
		rest := fields[1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			continue
		}
		if !isAstroBase(rest[0]) {
			return false
		}
	}
	return true
}

// isAstroBase reports whether a FROM reference names an image published to one
// of the registries Astro Runtime comes from.
//
// Deliberately not airflowrt.IsAstroRuntimeRef, which also accepts a mirror
// (a repository named runtime or astro-runtime, on a Runtime version tag). That
// answers "is this an Astro Runtime image"; this answers "can a pull be forced
// on it", and a mirror lives in exactly the kind of registry shouldPull exists
// to leave alone — a private one this daemon may not be logged into, or a copy
// retagged locally for an offline build. A mirror builds as a plain
// `docker build` would, from what the daemon has. An image under one of
// Astronomer's own names is still pulled, `docker load`ed or not.
//
// Matched at a path boundary, not as a bare prefix: quay.io/astronomerfake/x
// starts with quay.io/astronomer and is somebody else's registry, and treating
// it as ours would force a pull the user did not ask for.
func isAstroBase(ref string) bool {
	for _, base := range []string{astroRegistryHost, quayAstronomerRepo} {
		if ref == base || strings.HasPrefix(ref, base+"/") || strings.HasPrefix(ref, base+":") {
			return true
		}
	}
	return false
}

// The registries an Astro Runtime base comes from. Spelled here rather than
// imported from airflow/: that is shell code, and pkg/* does not depend on it.
const (
	astroRegistryHost  = "astrocrpublic.azurecr.io"
	quayAstronomerRepo = "quay.io/astronomer"
)

// runtimeDeps drops the requirements that state the Airflow version,
// apache-airflow and apache-airflow-core (manifest.WithoutAirflow), from the
// manifest dependencies, because the runtime base image already is Airflow
// (its tag is the Airflow version) and so is the one authority on it:
//
//   - apache-airflow the image's install script refuses outright ("Do not
//     upgrade by specifying 'apache-airflow' in your requirements.txt, change
//     the base image instead!").
//   - apache-airflow-core it does not refuse. A core requirement the image's
//     own core satisfies installs nothing, and one it does not, a different
//     series or a different patch of the same one, fails the build with a long
//     uv "No solution found" that names neither the base image nor the
//     manifest.
//
// A bare apache-airflow-task-sdk goes with them; one with a version, and every
// other dependency — providers, pandas, and the rest — installs normally.
func runtimeDeps(deps []string) []string {
	return manifest.WithoutAirflow(deps)
}
