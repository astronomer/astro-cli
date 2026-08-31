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
// seam onto internal/imagebuild; *imagebuild.Builder satisfies it, and a test
// substitutes a fake that never touches a daemon.
type ImageBuilder interface {
	Build(ctx context.Context, req imagebuild.Request, cb localrt.Callbacks) (string, error)
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
	// A declared Dockerfile IS the build, so no base is resolved for it — the
	// same rule local docker mode and deploy follow. Without this, packaging a
	// tier-3 project produced an artifact built over the runtime base with none
	// of the project's own build in it.
	declared := ""
	if req.Manifest.Astro.Dockerfile != "" {
		declared = filepath.Join(req.ProjectDir, filepath.FromSlash(req.Manifest.Astro.Dockerfile))
	}
	base := ""
	if declared == "" {
		var err error
		base, err = imagebuild.RuntimeImage(airflowVersion)
		if err != nil {
			return Result{}, err
		}
	}

	// The astro artifact is an image, so Docker is required. Probe the engine up
	// front for a plain message rather than an opaque build failure.
	if err := t.probeDocker(ctx); err != nil {
		return Result{}, ErrNoDocker
	}

	deps := req.Manifest.Project.Dependencies
	packages := req.Manifest.Astro.Packages
	hash, err := contentHash(base, req.Platform, deps, packages, req.Manifest.Astro.Dockerfile, declared)
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

	// Build into a source-hash working tag. With nothing to install the builder
	// returns the base image unchanged, so `built` is whichever image the final
	// tag should point at.
	workingTag := fmt.Sprintf("astro-package/%s:src-%s", name, hash)
	built, err := t.builder.Build(ctx, imagebuild.Request{
		WorkDir:      workDir,
		BaseImage:    base,
		Tag:          workingTag,
		Dependencies: deps,
		Packages:     packages,
		// Set together: Dockerfile mode builds the project's file against the
		// project as context, and imagebuild ignores Dependencies and Packages
		// there rather than rejecting them.
		Dockerfile: declared,
		Context:    dockerfileContext(declared, req.ProjectDir),
		Platform:   req.Platform,
		Bin:        t.bin,
		Env:        t.env,
	}, cb)
	if err != nil {
		return Result{}, err
	}
	// The builder tags the image under workingTag only when it actually builds;
	// with nothing to install it returns the base image untouched. Drop the
	// working tag once the final names point at the image, so package leaves a
	// clean set of tags — best effort, an orphaned tag is harmless.
	if built == workingTag {
		defer t.untag(ctx, workingTag)
	}

	// The exact runtime version comes off the built image's label, the same
	// value the deploy path validates against; fall back to the manifest pin if
	// the label is absent.
	runtimeVersion := t.readRuntimeVersion(ctx, built)
	if runtimeVersion == "" {
		runtimeVersion = airflowVersion
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

// readRuntimeVersion reads the runtime-version label off an image, dropping to
// the older airflow-version label the way the v1 deploy path does. A missing
// label or a failed inspect returns "" — the caller falls back to the manifest
// pin — so a non-runtime base never breaks the build.
func (t *AstroTarget) readRuntimeVersion(ctx context.Context, image string) string {
	var out bytes.Buffer
	format := fmt.Sprintf("{{ index .Config.Labels %q }}\t{{ index .Config.Labels %q }}", runtimeVersionLabel, airflowVersionLabel)
	if err := t.docker.Run(ctx, t.env, localrt.Stdio{Out: &out}, t.bin, "image", "inspect", "--format", format, image); err != nil {
		return ""
	}
	runtimeV, airflowV, _ := strings.Cut(strings.TrimSpace(out.String()), "\t")
	if v := cleanLabel(runtimeV); v != "" {
		return v
	}
	return cleanLabel(airflowV)
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
func contentHash(base, platform string, deps, packages []string, declaredRel, declaredAbs string) (string, error) {
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
	if declaredAbs != "" {
		writeField("dockerfile", filepath.ToSlash(declaredRel))
		body, err := os.ReadFile(declaredAbs)
		if err != nil {
			return "", fmt.Errorf("reading the Dockerfile this project declares (%s): %w", declaredRel, err)
		}
		writeField("dockerfile-body", fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:7], nil
}

// dockerfileContext is the build context for a declared Dockerfile, and empty
// for a generated build (which builds its own context under WorkDir).
//
// Paired in one function because the two fields are only correct together:
// Context without Dockerfile changes nothing, and Dockerfile without Context
// builds the project's file against imagebuild's generated directory, where none
// of the project's own COPY paths exist.
func dockerfileContext(declared, projectDir string) string {
	if declared == "" {
		return ""
	}
	return projectDir
}
