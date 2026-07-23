package localdocker

import (
	"context"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The build itself — assembling the requirements.txt/packages.txt context over
// the runtime base image and running the container build — lives in
// internal/imagebuild, so the coming v2 deploy path builds the same image (see
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

// removeBuiltImage deletes the project's dependency layer. It is best-effort:
// a project started with no extra dependencies never built one, and an image
// another running project shares must not block the clean.
func (e *Engine) removeBuiltImage(ctx context.Context, conn engineConn, projectName string) {
	//nolint:errcheck // best-effort clean-up; a missing or in-use image is fine
	_ = e.cmd.Run(ctx, conn.env, localrt.Stdio{}, conn.bin, "image", "rm", "--force", builtImageTag(projectName))
}
