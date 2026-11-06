package localdocker

import (
	"context"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The build itself — assembling the requirements.txt/packages.txt context over
// the runtime base image and running the container build — lives in
// internal/imagebuild, so the manifest deploy path builds the same image (see
// Start, which calls it). This file keeps only what is docker-mode's own: the
// tag the built layer carries and its clean-up.

// builtImagePrefix namespaces the per-project image built over the runtime
// base. The tag derives from the compose project name, so clean-up can find
// and remove it from the record alone.
const builtImagePrefix = "astro-local/"

// builtImageTag is the deterministic tag for a project's dependency layer.
func builtImageTag(projectName string) string {
	return builtImagePrefix + projectName
}

// builtImageID is the ID of the image the project's built tag names, "" when
// it names none (no build yet, or a project whose start needs none) or the
// engine could not say.
func (e *Engine) builtImageID(ctx context.Context, conn engineConn, projectName string) string {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "image", "inspect", "--format", "{{.Id}}", builtImageTag(projectName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// removeSupersededImage deletes the image the project's built tag named
// before this start, previous, when the start's build moved the tag to
// another. Without it every start whose dependencies changed leaves the last
// image behind untagged, about a gigabyte and a half each, and nothing ever
// removes those: a `--watch` session that restarts on each edit piles them up.
//
// Called once Airflow runs on the new image, since until then the old one may
// be what a rolled-back start falls back to. Best-effort, and without --force,
// so an image something still uses (another project whose identical
// dependencies built the same image, a container not yet gone) is left alone.
func (e *Engine) removeSupersededImage(ctx context.Context, conn engineConn, projectName, previous string) {
	if previous == "" {
		return
	}
	if now := e.builtImageID(ctx, conn, projectName); now == "" || now == previous {
		return
	}
	//nolint:errcheck // best-effort clean-up; an image still in use stays, which is the point of no --force
	_ = e.cmd.Run(ctx, conn.env, rt.Stdio{}, conn.bin, "image", "rm", previous)
}

// removeBuiltImage deletes the project's dependency layer. It is best-effort:
// a project started with no extra dependencies never built one, and an image
// another running project shares must not block the clean.
func (e *Engine) removeBuiltImage(ctx context.Context, conn engineConn, projectName string) {
	//nolint:errcheck // best-effort clean-up; a missing or in-use image is fine
	_ = e.cmd.Run(ctx, conn.env, rt.Stdio{}, conn.bin, "image", "rm", "--force", builtImageTag(projectName))
}
