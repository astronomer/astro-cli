package pack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// runtimeVersionLabel is the label the runtime base image carries with its
// exact version (e.g. "3.1-17"), the same label the deploy path reads to
// validate against a deployment (airflow.RuntimeImageLabel). airflowVersionLabel
// is the older fallback the v1 code drops to when the runtime label is absent.
// package reads them to name the artifact and to report the version.
const (
	runtimeVersionLabel = "io.astronomer.docker.runtime.version"
	airflowVersionLabel = "io.astronomer.docker.airflow.version"
)

// ErrNoDocker reports that the astro target could not reach a container engine.
// The astro artifact is an image, and building one needs Docker; the message
// says so plainly and points at the Docker-free path (docs/v2-deploy.md,
// section 5).
var ErrNoDocker = errors.New("building the astro package image needs Docker, but no engine is reachable; start Docker and try again (a dags-only `astro deploy --dags` needs no Docker, and remote builds are coming)")

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
	// bin is the container CLI (docker); env reaches its daemon.
	bin string
	env []string
}

// NewAstroTarget builds the target over a builder and a container command
// runner. cmd wires both to os/exec; a test injects fakes.
func NewAstroTarget(builder ImageBuilder, docker imagebuild.Commander, bin string, env []string) *AstroTarget {
	return &AstroTarget{builder: builder, docker: docker, bin: bin, env: env}
}

func (t *AstroTarget) Name() string { return TargetAstro }

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
	airflowVersion := req.Manifest.Astro.AirflowVersion
	// Which image the manifest builds is imagebuild's rule, the one deploy
	// follows too, so the artifact is the image a deploy of the same project
	// would build.
	breq, err := imagebuild.ForManifest(imagebuild.ManifestBuild{
		ProjectDir:     req.ProjectDir,
		AirflowVersion: airflowVersion,
		Dockerfile:     req.Manifest.Astro.Dockerfile,
		Dependencies:   req.Manifest.Project.Dependencies,
		Packages:       req.Manifest.Astro.Packages,
	})
	if err != nil {
		return Result{}, err
	}
	declared := breq.Dockerfile

	// The astro artifact is an image, so Docker is required. Probe the engine up
	// front for a plain message rather than an opaque build failure.
	if err := t.probeDocker(ctx); err != nil {
		return Result{}, ErrNoDocker
	}

	hash, err := contentHash(breq.BaseImage, req.Platform, breq.Dependencies, breq.Packages, declaredDockerfile{
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
	// with nothing to install it still builds the one-line image over the base.
	workingTag := fmt.Sprintf("astro-package/%s:src-%s", name, hash)
	breq.WorkDir = workDir
	breq.Tag = workingTag
	breq.Platform = req.Platform
	breq.Bin = t.bin
	breq.Env = t.env
	built, err := t.builder.BuildLocal(ctx, breq, cb)
	if err != nil {
		return Result{}, err
	}
	// Drop the working tag once the final names point at the image, so package
	// leaves a clean set of tags. Best effort; an orphaned tag is harmless.
	// Compared rather than assumed, so a builder that returns another reference
	// never has that reference untagged.
	if built == workingTag {
		defer t.untag(ctx, workingTag)
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
	runtimeV, airflowLabel, inspected := t.readVersionLabels(ctx, built)
	runtimeVersion := runtimeV
	if runtimeVersion == "" {
		runtimeVersion = airflowLabel
	}
	if runtimeVersion == "" {
		runtimeVersion = airflowVersion
	}
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
		// The same shape this target's other lines use (see emit below), rather
		// than a second mechanism for one message.
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{
				Component: "package",
				Time:      time.Now(),
				Text: fmt.Sprintf("warning: the image built from %s carries no %s label, so it is not based on Astro Runtime. It cannot be deployed with astro deploy; build it FROM an Astro Runtime image if that is the goal",
					req.Manifest.Astro.Dockerfile, runtimeVersionLabel),
			})
		}
	}

	finalTag := req.Tag
	if finalTag == "" {
		finalTag = fmt.Sprintf("astro-package/%s:%s-%s", name, tagSafe(runtimeVersion), hash)
	}
	if err := t.tag(ctx, built, finalTag); err != nil {
		return Result{}, err
	}
	// The moving :latest tag is part of the default scheme; a caller that pins
	// its own --tag owns its naming, so skip it there.
	if req.Tag == "" {
		if err := t.tag(ctx, built, fmt.Sprintf("astro-package/%s:latest", name)); err != nil {
			return Result{}, err
		}
	}

	res := Result{
		Target:         TargetAstro,
		Kind:           KindImage,
		Image:          finalTag,
		RuntimeVersion: runtimeVersion,
	}
	if req.Save != "" {
		if err := t.save(ctx, finalTag, req.Save, cb); err != nil {
			return Result{}, err
		}
		res.SavedPath = req.Save
		if info, statErr := os.Stat(req.Save); statErr == nil {
			res.Size = info.Size()
		}
	}
	return res, nil
}

// probeDocker reports whether a container engine is reachable. `docker version`
// contacts the daemon and returns non-zero when it is down, so a failure here
// (or a missing binary) means no engine.
func (t *AstroTarget) probeDocker(ctx context.Context) error {
	return t.docker.Run(ctx, t.env, localrt.Stdio{}, t.bin, "version")
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
// The third return distinguishes "the label is absent" from "the inspect
// failed", which the combined form flattens into "". A daemon restart mid-build
// is not a statement about the image's base, and reporting it as one is a
// confident wrong diagnosis.
func (t *AstroTarget) readVersionLabels(ctx context.Context, image string) (runtimeV, airflowV string, ok bool) {
	var out bytes.Buffer
	format := fmt.Sprintf("{{ index .Config.Labels %q }}\t{{ index .Config.Labels %q }}", runtimeVersionLabel, airflowVersionLabel)
	if err := t.docker.Run(ctx, t.env, localrt.Stdio{Out: &out}, t.bin, "image", "inspect", "--format", format, image); err != nil {
		return "", "", false
	}
	rawRuntime, rawAirflow, _ := strings.Cut(strings.TrimSpace(out.String()), "\t")
	return cleanLabel(rawRuntime), cleanLabel(rawAirflow), true
}

// cleanLabel normalizes one label value: docker prints "<no value>" for an
// absent label, which reads as empty.
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

// tag points dst at the same image as src.
func (t *AstroTarget) tag(ctx context.Context, src, dst string) error {
	if err := t.docker.Run(ctx, t.env, localrt.Stdio{}, t.bin, "tag", src, dst); err != nil {
		return fmt.Errorf("tagging %s as %s: %w", src, dst, err)
	}
	return nil
}

// untag drops a tag reference. It runs after the final tags point at the image,
// so it removes only the name, not the image. Failure is ignored: a leftover
// working tag is harmless.
func (t *AstroTarget) untag(ctx context.Context, ref string) {
	//nolint:errcheck // best-effort cleanup; a leftover tag changes nothing
	t.docker.Run(ctx, t.env, localrt.Stdio{}, t.bin, "image", "rm", "--no-prune", ref)
}

// save writes the image to a tarball with `docker save`, streaming its output
// (progress lands on stderr) through cb under the "package" component.
func (t *AstroTarget) save(ctx context.Context, image, path string, cb localrt.Callbacks) error {
	w := &localrt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{Component: "package", Time: time.Now(), Text: line})
		}
	}}
	err := t.docker.Run(ctx, t.env, localrt.Stdio{Err: w}, t.bin, "save", "--output", path, image)
	w.Flush()
	if err != nil {
		return fmt.Errorf("saving %s to %s: %w", image, path, err)
	}
	return nil
}

// contentHash is a short digest of the inputs that change the image, so the
// same inputs produce the same tag and a rebuild is a cache hit. Dependencies
// and packages are sorted first, so reordering the manifest does not move the
// tag.
// contentHash is the content address of the image this request would build.
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
// project directory and had to be withdrawn: `astro init` writes no
// .dockerignore, standalone provisions <project>/.venv, and AIRFLOW_HOME is
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
// Reading the file makes this fallible, which is the honest signature: a
// declaration naming something unreadable cannot be content-addressed, and
// imagebuild.Build refuses it a moment later anyway.
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

func contentHash(base, platform string, deps, packages []string, df declaredDockerfile) (string, error) {
	h := sha256.New()
	writeField := func(label, v string) {
		fmt.Fprintf(h, "%s\x00%s\x00", label, v)
	}
	writeField("base", base)
	writeField("platform", platform)
	for _, d := range sortedCopy(deps) {
		writeField("dep", d)
	}
	for _, p := range sortedCopy(packages) {
		writeField("pkg", p)
	}
	if df.abs != "" {
		writeField("dockerfile", filepath.ToSlash(df.rel))
		body, err := os.ReadFile(df.abs)
		if err != nil {
			return "", fmt.Errorf("reading the Dockerfile this project declares (%s): %w", df.rel, err)
		}
		writeField("dockerfile-body", fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:7], nil
}
