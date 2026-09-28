// Package imagebuild builds a deployable Airflow image from a manifest's
// fields. It assembles a small build context (requirements.txt, packages.txt)
// over Astronomer's runtime base image and runs the container build, leaning
// on the runtime image's ONBUILD triggers to install the project's
// dependencies and OS packages — the same path `astro dev` builds through.
//
// A project may instead declare its own Dockerfile in the manifest, which the
// project design calls tier 3: the escape hatch for multi-stage builds and
// anything else a manifest cannot express, at the stated cost of portability.
// Request.Dockerfile switches to that mode, where the file is the build and
// nothing here is generated. Both modes are supported; neither is a ramp off
// the other.
//
// It was lifted out of internal/localdocker so the v2 deploy path can build the
// same image the local Docker engine builds (docs/v2-deploy.md, decision 7 and
// section 1). Its inputs are manifest-shaped — Python dependencies, OS packages,
// a runtime base image, a tag — so it depends on neither localdocker nor a
// deploy package; each caller resolves the manifest to a Request and hands it
// over. Build itself takes an already-resolved BaseImage; RuntimeImage maps an
// Airflow version to that base and lives here now that both the local Docker
// engine and deploy resolve it the same way.
//
// Per the layer rules (docs/v2-architecture.md) it prints nothing and never
// exits: build output flows through rt.Callbacks and a failed install
// returns a named error.
package imagebuild

import (
	"context"
	"errors"
	"fmt"
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
// version. Deploy, `astro package astro` and the local Docker engine's Airflow 3
// path all resolve the base this way.
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

// ErrDockerfileBuild reports that the build of the project's own Dockerfile
// failed, rather than the install of a generated build.
var ErrDockerfileBuild = errors.New("building the project's Dockerfile failed; see the build output above")

const (
	// dockerfileName is the one-line `FROM <base>` Dockerfile the build runs;
	// the runtime image's ONBUILD triggers do the install.
	dockerfileName = "Dockerfile.astro-local"
	// buildContextDir is the sub-directory of a Request's WorkDir that holds
	// the build context — the two files the ONBUILD triggers copy.
	buildContextDir = "deps-build"
	// requirementsName and packagesName are the fixed names the runtime image's
	// ONBUILD triggers copy from the build context.
	requirementsName = "requirements.txt"
	packagesName     = "packages.txt"
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
	// BaseImage is the resolved runtime image the build starts FROM
	// (astrocrpublic.azurecr.io/runtime:<version>). Empty in Dockerfile mode:
	// the project's own file declares what it builds on.
	BaseImage string
	// Secrets are docker build --secret specs ("id=mysecret[,src=/local/secret]"
	// or "id=mysecret,env=ENV_VAR"),
	// one per entry, forwarded verbatim in the order given.
	//
	// Only meaningful in Dockerfile mode: a generated build's Dockerfile is
	// `FROM <base>` and the install happens in the runtime image's own ONBUILD
	// triggers, so there is no RUN of the project's for a secret to be mounted
	// into. Callers refuse the combination rather than passing secrets that
	// could not be read; FromDeclaredDockerfile is the check.
	//
	// The SPEC is forwarded, never a secret value: docker reads the value itself
	// from the src file or the named env var. So these strings are safe in a
	// command line and in the build log, which is where they end up.
	Secrets []string
	// Dockerfile and Context switch this into Dockerfile mode — the project
	// supplied a real Dockerfile ("tier 3" in the project design) and it, not
	// the manifest, is the build. Both are absolute, and set together.
	//
	// The two modes share little. This one writes no requirements.txt or
	// packages.txt, because the file owns its own installs; it takes its FROM
	// from the file rather than BaseImage; and it builds the PROJECT as its
	// context, because a multi-stage build COPYs from the repo. That last part
	// is also why the generated mode cannot just add the project to its
	// context: that context is synthetic and deliberately tiny, and the runtime
	// image's ONBUILD `COPY . .` would otherwise bake the whole repo into the
	// dependency layer.
	//
	// Dependencies and Packages are ignored here rather than rejected. A caller
	// reading a v2 manifest has them populated whichever tier the project
	// chose, and a Dockerfile project installs its own.
	Dockerfile string
	Context    string
	// Tag is the image reference the build produces.
	Tag string
	// Dependencies are the manifest's Python dependencies (PEP 508);
	// apache-airflow is dropped, since the runtime base already provides it.
	Dependencies []string
	// Packages are the manifest's OS (apt) package names.
	Packages []string
	// Platform is the build platform (e.g. "linux/amd64"). Empty builds for the
	// host — what local Docker mode wants; the deploy path pins linux/amd64.
	Platform string
	// Bin is the container CLI to shell out to; Env reaches its daemon.
	Bin string
	Env []string
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

func (execCommander) Run(ctx context.Context, extraEnv []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
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
// installed into a layer over its base image through the runtime image's ONBUILD
// triggers, and with nothing to install it builds nothing and returns the base
// image unchanged (the fast path).
//
// Build output streams to cb.OnLine (component "build"); a failed build returns
// a named error, never a hang.
func (b *Builder) Build(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	return b.buildImage(ctx, req, cb, true)
}

// buildImage is Build, with the generated mode's fast path optional: with
// fastPath false a request with nothing to install still builds, producing
// req.Tag over the base rather than returning the base itself.
func (b *Builder) buildImage(ctx context.Context, req Request, cb rt.Callbacks, fastPath bool) (string, error) {
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
			return "", fmt.Errorf("the Dockerfile this project declares (%s) is not a file", req.Dockerfile)
		}
		// No fast path here. An unchanged Dockerfile is the daemon's layer cache
		// to short-circuit, not ours to skip: we cannot tell from the outside
		// whether the file's own steps would produce something new.
		return b.build(ctx, req, req.Dockerfile, req.Context, cb)
	}

	deps := runtimeDeps(req.Dependencies)
	// The base image already provides Airflow, so a project with nothing beyond
	// Airflow and no OS packages needs no build and runs the base as-is.
	if fastPath && len(deps) == 0 && len(req.Packages) == 0 {
		return req.BaseImage, nil
	}

	contextDir := filepath.Join(req.WorkDir, buildContextDir)
	if err := os.MkdirAll(contextDir, contextDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", contextDir, err)
	}
	// requirements.txt carries the deps; packages.txt carries the OS packages,
	// one apt name per line. Both must exist even when empty, or the image's
	// `ONBUILD COPY <file> .` fails, so an empty list still writes the file.
	if err := os.WriteFile(filepath.Join(contextDir, requirementsName), []byte(strings.Join(deps, "\n")+"\n"), filePermRW); err != nil {
		return "", fmt.Errorf("writing the generated requirements file: %w", err)
	}
	var packagesData []byte
	if len(req.Packages) > 0 {
		packagesData = []byte(strings.Join(req.Packages, "\n") + "\n")
	}
	if err := os.WriteFile(filepath.Join(contextDir, packagesName), packagesData, filePermRW); err != nil {
		return "", fmt.Errorf("writing the generated packages file: %w", err)
	}
	// The Dockerfile is only `FROM <base>`; the ONBUILD triggers do the
	// install. It sits outside the context so the trailing `COPY . .` does not
	// bake it in.
	dfPath := filepath.Join(req.WorkDir, dockerfileName)
	if err := os.WriteFile(dfPath, []byte("FROM "+req.BaseImage+"\n"), filePermRW); err != nil {
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
	if shouldPull(req.Dockerfile, dockerfile) {
		args = append(args, "--pull")
	}
	// Gated on Dockerfile mode, not just left to the caller. build() is shared,
	// and a generated build's Dockerfile is one this package wrote — `FROM
	// <base>`, with the install in the runtime image's ONBUILD triggers — so a
	// secret has nothing of the project's to be mounted into. Callers refuse the
	// combination; this makes the invariant hold whether or not they do, rather
	// than handing docker a flag that cannot work.
	if req.FromDeclaredDockerfile() {
		for _, secret := range req.Secrets {
			args = append(args, "--secret", secret)
		}
	}
	if req.Platform != "" {
		args = append(args, "--platform", req.Platform)
	}
	args = append(args, contextDir)
	err := b.cmd.Run(ctx, req.Env, rt.Stdio{Out: w, Err: w}, req.Bin, args...)
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
// The rule is v1's, deliberately: airflow/docker_image.go's shouldAddPullFlag
// skips --pull as soon as ANY FROM names something other than an Astro base, so
// a project moving to v2 keeps the behavior it had. Any is the right quantifier
// rather than the final stage — a builder stage on an unreachable image fails
// the build just as hard.
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
// imported from airflow/: that is the v1 tree, and pkg/* does not depend on it.
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
