package pack

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/shipcheck"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/util"
)

// airflowVersionLabel is the older fallback to the runtime base image's
// airflowrt.RuntimeVersionLabel (its exact version, e.g. "3.1-17"), which the
// shell's deploy code drops to when the runtime label is absent. package reads
// them to name the artifact and to report the version.
const (
	airflowVersionLabel = "io.astronomer.docker.airflow.version"
)

// ErrNoDocker reports that the astro target could not reach a container engine.
// The astro artifact is an image, and building one needs Docker; the message
// says so plainly and points at the Docker-free path (docs/deploy.md,
// section 5).
var ErrNoDocker = errors.New("building the astro package image needs Docker, but no engine is reachable; start Docker and try again (a dags-only astro deploy --dags needs no Docker, and remote builds are coming)")

// ImageBuilder builds a deployable image from a manifest's fields. It is the
// seam onto pkg/imagebuild; *imagebuild.Builder satisfies it, and a test
// substitutes a fake that never touches a daemon.
//
// BuildLocal rather than Build: the target inspects, tags and saves the image
// it gets back, so it has to be a single-platform image in the local store,
// built at the requested platform even when there is nothing to install.
type ImageBuilder interface {
	BuildLocal(ctx context.Context, req imagebuild.Request, cb localrt.Callbacks) (string, error)
}

// AstroTarget builds the Astro artifact: a container image over Astronomer's
// runtime base with the project's dependencies and OS packages installed, the
// image build from section 1 with the shipping steps removed. By default the
// image lands in the local Docker store; --save also writes it to a tarball so
// another CI job can load it and `astro deploy --image-name` consume it.
type AstroTarget struct {
	// builder turns the manifest into the dependency-installed image.
	builder ImageBuilder
	// docker runs the tag/save/probe commands the build itself does not: it
	// reads the runtime label, retags to the content-addressed name, and saves.
	docker imagebuild.Commander
	// engine picks the container CLI for the project being packaged and the
	// env that reaches its daemon.
	engine EngineResolver
}

// EngineResolver returns the container CLI ("docker" or "podman") for the
// project at projectDir and the env that reaches its daemon, erroring when no
// engine is installed. A func of the project, called only when an image is
// built, because container.binary can be set per project and the other
// targets need no engine at all.
type EngineResolver func(projectDir string) (bin string, env []string, err error)

// NewAstroTarget builds the target over a builder, a container command runner,
// and the engine resolver. cmd wires them to os/exec and container.binary; a
// test injects fakes.
func NewAstroTarget(builder ImageBuilder, docker imagebuild.Commander, engine EngineResolver) *AstroTarget {
	return &AstroTarget{builder: builder, docker: docker, engine: engine}
}

func (t *AstroTarget) Name() string { return TargetAstro }

// checkManifest runs the checks on the manifest that come before any build:
// the runtime build against its pin, then the declared environment, which
// must parse and which the image does not carry, so it comes back as the
// values the Deployment must supply (deploymentEnv).
func checkManifest(ctx context.Context, req Request, airflow manifest.Airflow) ([]string, []DeploymentValue, error) {
	warnings, err := checkRuntimeBuild(ctx, req, airflow)
	if err != nil {
		return nil, nil, err
	}
	env, err := deploymentEnv(req.Manifest.Astro.Env)
	if err != nil {
		return nil, nil, err
	}
	if w := scaffold.StaleHelperWarning(req.ProjectDir, req.Manifest.Astro.Dockerfile); w != "" {
		warnings = append(warnings, w)
	}
	// One astro init would move off its ONBUILD tag was refused when the
	// manifest was loaded (loadCheckedManifest); one it would not builds as
	// before, and says so.
	switch w, err := scaffold.CheckDockerfileOnBuild(req.ProjectDir, req.Manifest); {
	case err != nil:
		return nil, nil, err
	case w != "":
		warnings = append(warnings, w)
	}
	return warnings, env, nil
}

// checkRuntimeBuild holds the manifest's runtime build to its pin before
// anything is built (Request.CheckRuntime): a build of another series refuses
// the package, and what the catalog warns about comes back to ride on the
// result.
func checkRuntimeBuild(ctx context.Context, req Request, airflow manifest.Airflow) ([]string, error) {
	if req.CheckRuntime == nil || airflow.Runtime == "" {
		return nil, nil
	}
	found, err := req.CheckRuntime(ctx, airflow.Runtime, airflow.Pin)
	if err != nil {
		return nil, err
	}
	var warnings []string
	for _, f := range found {
		warnings = append(warnings, f.Message)
	}
	return warnings, nil
}

func runtimeCatalog(ctx context.Context, req Request) func() *runtimeversions.Catalog {
	if req.RuntimeCatalog == nil {
		return nil
	}
	return func() *runtimeversions.Catalog { return req.RuntimeCatalog(ctx) }
}

// checkSecrets runs before the image build. It refuses a build secret whose
// variable is unset, and warns about each secret a declared Dockerfile mounts
// that no build secret supplies, returning those for a failed build to name
// again.
func checkSecrets(req Request, cb localrt.Callbacks) (imagebuild.MissingSecrets, error) {
	dockerfile := req.Manifest.Astro.Dockerfile
	missing, err := imagebuild.CheckBuildSecrets(req.ProjectDir, dockerfile, req.BuildSecrets)
	if dockerfile != "" {
		missing.Hint = util.BuildSecretHint
	}
	warn := req.warn(cb)
	for _, w := range missing.Warnings() {
		warn(w)
	}
	return missing, err
}

// reachEngineAndCheck reaches the engine (reachEngine), then holds the
// packaged image, which is pushed to a registry as a deploy's is, to the same
// check of what it carries (shipcheck.Survey). The check waits for the engine
// because its kind decides the ignore file a declared Dockerfile's build
// reads.
func (t *AstroTarget) reachEngineAndCheck(ctx context.Context, req Request, cb localrt.Callbacks) (imagebuild.Engine, error) {
	engine, err := t.reachEngine(ctx, req.ProjectDir)
	if err != nil {
		return imagebuild.Engine{}, err
	}
	_, err = shipcheck.Survey(req.ProjectDir, req.Manifest.Astro.Dockerfile, imagebuild.IsPodman(ctx, engine.Cmd, engine.Bin, engine.Env), req.warn(cb))
	return engine, err
}

// Build resolves the runtime base, builds the image, reads its runtime-version
// label, tags it to the content-addressed name (astro-package/<name>:<version>-
// <hash> plus a moving :latest), optionally saves it, and reports the tag so
// `astro deploy --image-name` consumes it.
func (t *AstroTarget) Build(ctx context.Context, req Request, cb localrt.Callbacks) (Result, error) {
	if req.Manifest == nil {
		return Result{}, errors.New("no manifest to package")
	}
	name := req.Manifest.Project.Name
	if name == "" {
		return Result{}, errors.New("the project has no name; set [project] name in pyproject.toml")
	}
	// Every warning this target gives goes through one path: shown as it is
	// found (Request.Warn, or a progress line), and kept for the result, so
	// -o json's result carries every one, as a deploy's does.
	var warnings []string
	show := req.warn(cb)
	req.Warn = func(w string) {
		warnings = append(warnings, w)
		show(w)
	}
	airflow := req.Manifest.Airflow()
	found, deployEnv, err := checkManifest(ctx, req, airflow)
	if err != nil {
		return Result{}, err
	}
	for _, w := range found {
		req.Warn(w)
	}
	// Which image the manifest builds is imagebuild's rule, the one deploy
	// follows too, so the artifact is the image a deploy of the same project
	// would build.
	breq, err := imagebuild.ForManifest(imagebuild.ManifestBuildOf(req.ProjectDir, req.Manifest), runtimeCatalog(ctx, req))
	if err != nil {
		return Result{}, err
	}
	declared := breq.Dockerfile
	// What a generated image installs is resolved here, from the project's
	// lock, before the content address that covers it.
	if err := req.resolveInstall(ctx, &breq); err != nil {
		return Result{}, err
	}
	if res, err, done := t.buildNoImage(ctx, req, breq, name, warnings, cb); done {
		return res, err
	}
	missingSecrets, err := checkSecrets(req, cb)
	if err != nil {
		return Result{}, err
	}

	// The astro artifact is an image, so a container engine is required.
	engine, err := t.reachEngineAndCheck(ctx, req, cb)
	if err != nil {
		return Result{}, err
	}

	hash, err := contentHash(breq.BaseImage, req.Platform, generatedRecipe(breq), breq, declaredDockerfile{
		rel: req.Manifest.Astro.Dockerfile,
		abs: declared,
	})
	if err != nil {
		return Result{}, err
	}

	// The build context lives in a scratch dir the target owns unless the caller
	// pins one (a test). A pinned WorkDir is left in place; a made one is removed.
	workDir := req.WorkDir
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "astro-package-")
		if err != nil {
			return Result{}, fmt.Errorf("creating a build directory: %w", err)
		}
		defer os.RemoveAll(workDir) //nolint:errcheck // scratch dir; a leftover temp dir is harmless
	}

	// Build into a source-hash working tag, which BuildLocal always returns:
	// with nothing to install it still builds the generated Dockerfile over the
	// runtime's -base image, its install steps installing nothing.
	workingTag := fmt.Sprintf("astro-package/%s:src-%s", name, hash)
	breq.WorkDir = workDir
	breq.Tag = workingTag
	breq.Platform = req.Platform
	breq.Secrets = req.BuildSecrets
	breq.Bin = engine.Bin
	breq.Env = engine.Env
	built, err := t.builder.BuildLocal(ctx, breq, cb)
	if err != nil {
		return Result{}, missingSecrets.Explain(err)
	}
	// The exact runtime version comes off the built image's label, the same value
	// the deploy path validates against.
	//
	// A DECLARED Dockerfile with no label is a hard error, because the artifact
	// this produces cannot be deployed. cloud/deploy refuses exactly this image
	// on exactly this label, in both its build and its --image-name arm — so
	// falling back to the manifest pin here produced a tag asserting an Astro
	// Runtime version the image does not have, handed the user an artifact whose
	// "whole purpose is to be the thing that ships", and let them discover it at
	// deploy time. The two paths agree now, and this one fails first.
	//
	// The fallback stays for a GENERATED build, where it is defensive rather than
	// load-bearing: that base is an Astro runtime by construction, so a missing
	// label means something odd about the image rather than a choice the user made.
	runtimeV, airflowLabel, inspected := readVersionLabels(ctx, engine, built)
	runtimeVersion := cmp.Or(runtimeV, airflowLabel, airflow.Pin)
	// A declared Dockerfile that did not produce an Astro Runtime image is
	// REPORTED, not refused.
	//
	// The gap was silence: the tag took the manifest pin and asserted an Astro
	// Runtime version the image does not have, and cloud/deploy then refused it
	// on this label — so the artifact whose purpose is to ship could not, and
	// nothing said so until deploy. A hard error closed that and closed a real
	// workflow with it: a Dockerfile on a plain python base, packaged with --tag
	// and --save for a self-hosted Airflow, worked before, and the `oss` target
	// that ought to serve it is still a stub. Warning keeps both.
	//
	// Gated on the RUNTIME label alone, which is the test deploy applies, and on
	// the inspect having actually succeeded — a daemon that failed to answer says
	// nothing about the image's base.
	if declared != "" && inspected && runtimeV == "" {
		req.Warn(fmt.Sprintf("the image built from %s carries no %s label, so it is not based on Astro Runtime. It cannot be deployed with astro deploy; build it FROM an Astro Runtime image if that is the goal",
			req.Manifest.Astro.Dockerfile, airflowrt.RuntimeVersionLabel))
	}

	finalTag := req.Tag
	if finalTag == "" {
		finalTag = fmt.Sprintf("astro-package/%s:%s-%s", name, tagSafe(runtimeVersion), hash)
	}
	if err := engine.Tag(ctx, built, finalTag); err != nil {
		return Result{}, err
	}
	// The moving :latest tag is part of the default scheme; a caller that pins
	// its own --tag owns its naming, so skip it there.
	if req.Tag == "" {
		if err := engine.Tag(ctx, built, fmt.Sprintf("astro-package/%s:latest", name)); err != nil {
			return Result{}, err
		}
	}
	// Drop the working tag now that the final names point at the image, so
	// package leaves a clean set of tags. Only here, after every Tag succeeded:
	// rmi of an image's last name removes the image and can prune its dangling
	// parent layers (podman; docker without BuildKit), so a failed tag leaves
	// the working tag and the image in place instead. With another name on the
	// image, rmi drops only the name. Best effort; an orphaned tag is harmless.
	// Compared rather than assumed, so a builder that returns another reference
	// never has that reference untagged.
	if built == workingTag {
		engine.Untag(ctx, workingTag) //nolint:errcheck // best-effort cleanup; a leftover tag changes nothing
	}

	res := Result{
		Target:         TargetAstro,
		Kind:           KindImage,
		Image:          finalTag,
		RuntimeVersion: runtimeVersion,
		Warnings:       warnings,
		DeploymentEnv:  deployEnv,
	}
	if req.Save != "" {
		if err := save(ctx, engine, finalTag, req.Save, cb); err != nil {
			return Result{}, err
		}
		res.SavedPath = req.Save
		if info, statErr := os.Stat(req.Save); statErr == nil {
			res.Size = info.Size()
		}
	}
	return res, nil
}

// reachEngine resolves the container engine for the project and probes it up
// front (deploybuild.ReachEngine, the check an image deploy makes), so an
// engine that is missing or down is a plain ErrNoDocker rather than an opaque
// build failure — or, for podman with no machine up, the podman command that
// brings one up, which "start Docker" would not be.
//
// The engine is a value per Build rather than a field on the target, so a
// target never carries one project's engine into another's build.
func (t *AstroTarget) reachEngine(ctx context.Context, projectDir string) (imagebuild.Engine, error) {
	engine, err := deploybuild.ReachEngine(ctx, t.docker, func() (string, []string, error) { return t.engine(projectDir) })
	if errors.Is(err, container.ErrMachineNotRunning) {
		return imagebuild.Engine{}, fmt.Errorf("building the astro package image needs a running container engine, but %w", err)
	}
	if err != nil {
		return imagebuild.Engine{}, ErrNoDocker
	}
	return engine, nil
}

// readVersionLabels reads both version labels off an image, keeping them apart.
//
// Apart, because they answer different questions and one caller needs the
// difference. The tag may fall back to the airflow label — an older Astro image
// carries it and no runtime label — but "is this deployable to Astro" is about
// the RUNTIME label alone, which is the only one cloud/deploy reads. A check
// built on a combined runtime-or-airflow answer passes a
// `FROM astronomerinc/ap-airflow:...-onbuild` image that deploy then refuses.
//
// The third return distinguishes "the label is absent" from "an inspect
// failed", which the combined form flattens into "". A daemon restart mid-build
// is not a statement about the image's base, and reporting it as one is a
// confident wrong diagnosis. ok is whether the RUNTIME label was read, the one
// the base check rests on: a failed runtime read reports both as unread, while
// a failed airflow read after a good runtime one keeps the runtime value and
// reports the airflow label as absent.
func readVersionLabels(ctx context.Context, engine imagebuild.Engine, image string) (runtimeV, airflowV string, ok bool) {
	rawRuntime, err := engine.Label(ctx, image, airflowrt.RuntimeVersionLabel)
	if err != nil {
		return "", "", false
	}
	runtimeV = cleanLabel(rawRuntime)
	rawAirflow, err := engine.Label(ctx, image, airflowVersionLabel)
	if err != nil {
		return runtimeV, "", true
	}
	return runtimeV, cleanLabel(rawAirflow), true
}

// cleanLabel normalizes one label value. imagebuild.Engine.Label's
// `index .Config.Labels` format prints "" for an absent label on docker and
// podman; the "<no value>" branch is defensive, for an engine that renders a
// missing map key the way Go's templates do by default.
func cleanLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "<no value>" {
		return ""
	}
	return v
}

// tagSafe replaces characters a Docker tag forbids so a version like
// "3.1.8+astro.4" (the airflow-label fallback carries a '+') still tags. A tag
// allows letters, digits, and _.-; a leading . or - is dropped.
func tagSafe(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			b.WriteByte(byte(r))
		default:
			b.WriteByte('-')
		}
	}
	out := strings.TrimLeft(b.String(), ".-")
	if out == "" {
		return "unknown"
	}
	return out
}

// save writes the image to a tarball (Engine.Save), streaming the engine's
// progress through cb under the "package" component.
func save(ctx context.Context, engine imagebuild.Engine, image, path string, cb localrt.Callbacks) error {
	w := &localrt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{Component: "package", Time: time.Now(), Text: line})
		}
	}}
	err := engine.Save(ctx, image, path, w)
	w.Flush()
	if err != nil {
		return fmt.Errorf("saving %s to %s: %w", image, path, err)
	}
	return nil
}

// declaredDockerfile pairs the manifest's own slash-separated value with the
// absolute path it resolves to.
//
// A struct rather than two adjacent string parameters, because the two are
// interchangeable to the compiler and are NOT interchangeable to the hash: the
// relative value is hashed (so the address does not depend on where the checkout
// sits) and the absolute one is read. Transposed, contentHash would put an
// absolute path into the field kept relative on purpose and try to read a
// project-relative path — the first fails silently into a wrong hash, and no test
// could see it.
type declaredDockerfile struct {
	rel string // the manifest value, slash-separated; "" when none is declared
	abs string // rel resolved against the project; "" when none is declared
}

// generatedRecipe is the Dockerfile a generated build of breq runs, as its
// content address takes it, or "" for a declared Dockerfile, which is
// addressed by its own bytes. It is the recipe with the netrc mount, whether or
// not this build has the secret: a secret decides what the install can reach,
// not what a successful one installs, so a laptop and a CI runner holding a
// token get the same tag.
func generatedRecipe(breq imagebuild.Request) string {
	if breq.FromDeclaredDockerfile() {
		return ""
	}
	return imagebuild.GeneratedDockerfile(breq.BaseImage, true)
}

// contentHash is the content address of the image breq builds over base for
// platform: a short digest of the inputs that change the image, so the same
// inputs produce the same tag and a rebuild is a cache hit.
//
// Generated or declared is decided once, by recipe (generatedRecipe), "" being
// a declared Dockerfile.
//
// A generated build contributes its recipe, the Dockerfile imagebuild writes,
// and the lists its install reads (InstallLists.Digest: requirements.txt, the
// lock's constraints and packages.txt, as the CLI resolved them from uv.lock).
// The dependencies, packages and index pages are all in those, so no list of
// them is hashed besides.
//
// A generated image also carries the project's files (breq.ProjectFiles), so
// the digest of those files is part of the address: editing include/ changes
// the image, and has to change the tag. The digest covers exactly the files the
// image carries — the project's .dockerignore applied, and .venv, .git and
// .astro/standalone never among them — so it does not hit the problems the
// declared-Dockerfile context below did.
//
// A declared Dockerfile contributes the DECLARED path and the file's BYTES.
//
// Its bytes, because in Dockerfile mode the file is the whole build and
// imagebuild ignores base, deps and packages — so a hash built only from those
// gave two different images the same content-addressed tag, and editing the
// Dockerfile republished under the tag the previous image already held.
//
// NOT the build CONTEXT, which the Dockerfile can also COPY from, so this
// address is incomplete and knowingly so. A first attempt hashed the whole
// project directory and had to be withdrawn: `astro init` wrote no
// .dockerignore then, standalone provisions <project>/.venv, and AIRFLOW_HOME is
// <project>/.astro/standalone — so the walk read a virtualenv full of
// host-absolute symlinks, a .git directory full of timestamps, and a live SQLite
// database. The tag then differed between a laptop and CI for a byte-identical
// image, moved every time the scheduler wrote a heartbeat, and failed outright
// when a WAL file vanished mid-walk. Closing a collision in an uncommon shape is
// not worth non-determinism and intermittent failure in the common one.
//
// Doing it properly means honoring .dockerignore, which this repo already
// implements in cloud/deploy's dockerignoreSkipFunc over the vendored
// moby/patternmatcher — plus file modes (docker's context carries the executable
// bit, so `chmod +x` changes the image), symlink targets recorded relative to the
// context, streaming reads, and a context.Context to cancel on. That is its own
// change, not a rider on this one. Until then a project whose Dockerfile COPYs
// its own files can reuse a tag after editing them.
//
// The DECLARED path, meaning the manifest's own slash-separated value, not the
// absolute path it resolves to. Hashing the absolute path would make the content
// address depend on where the project happens to sit, so the same commit built
// in two checkouts — or in CI — would produce different tags for an identical
// image. Caught by the test for this, which builds the same project twice from
// two temp dirs.
//
// Reading the files makes this fallible, which is the honest signature: a
// declaration naming something unreadable cannot be content-addressed, and
// imagebuild.Build refuses it a moment later anyway.
func contentHash(base, platform, recipe string, breq imagebuild.Request, df declaredDockerfile) (string, error) {
	h := sha256.New()
	writeField := func(label, v string) {
		fmt.Fprintf(h, "%s\x00%s\x00", label, v)
	}
	writeField("base", base)
	writeField("platform", platform)
	if recipe != "" {
		writeField("generated-dockerfile", fmt.Sprintf("%x", sha256.Sum256([]byte(recipe))))
		writeField("install-lists", breq.Lists().Digest())
	} else {
		writeField("dockerfile", filepath.ToSlash(df.rel))
		body, err := os.ReadFile(df.abs)
		if err != nil {
			return "", fmt.Errorf("reading the Dockerfile this project declares (%s): %w", df.rel, err)
		}
		writeField("dockerfile-body", fmt.Sprintf("%x", sha256.Sum256(body)))
		// A declared Dockerfile built on the install base (as astro init
		// writes a converted one) installs the lists the CLI resolved from the
		// project's lock, over the runtime pyproject.toml picks: the base's tag
		// covers both, so an edit to the lock or the runtime moves the
		// address, as it does for a generated build.
		if breq.UsesInstallBase() {
			writeField("install-base", breq.InstallBaseTag())
		}
		// The files the Dockerfile's build reads, under the rules a generated
		// build copies them by, so an edit to a Dag or a plugin moves the tag.
		digest, err := imagebuild.ProjectFilesDigest(breq.Context)
		if err != nil {
			return "", err
		}
		writeField("project-files", digest)
	}
	if projectFiles := breq.ProjectFiles; projectFiles != "" {
		digest, err := imagebuild.ProjectFilesDigest(projectFiles)
		if err != nil {
			return "", err
		}
		writeField("project-files", digest)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:7], nil
}

// buildInstallBase builds the install base the project's own Dockerfile
// builds on (Request.BaseOnly) and reports its local tag, for a caller that
// then builds the Dockerfile itself with `--build-arg ASTRO_BASE=<tag>`.
func (t *AstroTarget) buildInstallBase(ctx context.Context, req Request, breq imagebuild.Request, warnings []string, cb localrt.Callbacks) (Result, error) {
	switch {
	case !breq.FromDeclaredDockerfile():
		return Result{}, errors.New("--base builds the install base a project's own Dockerfile builds on, and this project declares no Dockerfile: astro package builds its image directly")
	case !breq.UsesInstallBase():
		return Result{}, fmt.Errorf("--base builds the install base a project's own Dockerfile builds FROM %s on, and %s names its own FROM. Run astro init to move it onto the install base",
			airflowrt.BaseArgFrom, req.Manifest.Astro.Dockerfile)
	}
	engine, err := t.reachEngine(ctx, req.ProjectDir)
	if err != nil {
		return Result{}, err
	}
	workDir := req.WorkDir
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "astro-package-")
		if err != nil {
			return Result{}, fmt.Errorf("creating a build directory: %w", err)
		}
		defer os.RemoveAll(workDir) //nolint:errcheck // scratch dir; a leftover temp dir is harmless
	}
	breq.WorkDir = workDir
	breq.Platform = req.Platform
	breq.Secrets = req.BuildSecrets
	breq.Bin = engine.Bin
	breq.Env = engine.Env
	breq.InstallBaseOnly = true
	tag, err := t.builder.BuildLocal(ctx, breq, cb)
	if err != nil {
		return Result{}, err
	}
	return Result{Target: TargetAstro, Kind: KindInstallBase, Image: tag, Warnings: warnings}, nil
}

// buildNoImage is Build for the two requests that do not build the image: the
// install base alone (Request.BaseOnly), and a build context written for
// someone else to build (Request.ContextDir), which is built later, by whoever
// runs the commands it prints, so the secrets it names need not be readable
// now. done is false for any other request.
func (t *AstroTarget) buildNoImage(ctx context.Context, req Request, breq imagebuild.Request, name string, warnings []string, cb localrt.Callbacks) (res Result, err error, done bool) {
	switch {
	case req.BaseOnly:
		res, err = t.buildInstallBase(ctx, req, breq, warnings, cb)
		return res, err, true
	case req.ContextDir != "":
		res, err = writeContext(req, breq, name)
		res.Warnings = warnings
		return res, err, true
	}
	return Result{}, nil, false
}
