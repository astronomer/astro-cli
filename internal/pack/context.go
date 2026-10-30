package pack

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// writeContext writes the build context of the generated image breq describes
// into req.ContextDir, and reports the commands that build it: the escape
// hatch for an engine, a builder or a CI the CLI does not drive. The context
// is the one the target builds, its lists already resolved, so a plain
// `docker build` of it makes the same image, with nothing from the CLI.
func writeContext(req Request, breq imagebuild.Request, name string) (Result, error) {
	if breq.FromDeclaredDockerfile() {
		return Result{}, fmt.Errorf("the project declares its own Dockerfile (%s), so the project is its build context: build it with docker build -f %s %s",
			req.Manifest.Astro.Dockerfile, req.Manifest.Astro.Dockerfile, req.ProjectDir)
	}
	dir, err := filepath.Abs(req.ContextDir)
	if err != nil {
		return Result{}, err
	}
	breq.Platform = req.Platform
	if err := imagebuild.WriteContext(dir, breq); err != nil {
		if errors.Is(err, imagebuild.ErrContextDirNotEmpty) {
			return Result{}, fmt.Errorf("%w; --context-dir writes only into a new or empty directory", err)
		}
		return Result{}, err
	}
	tag := req.Tag
	if tag == "" {
		tag = fmt.Sprintf("astro-package/%s:latest", name)
	}
	return Result{
		Target:     TargetAstro,
		Kind:       KindContext,
		ContextDir: dir,
		NextSteps: []string{
			buildCommand("docker", req.Platform, tag, req.BuildSecrets, dir),
			buildCommand("podman", req.Platform, tag, req.BuildSecrets, dir),
		},
	}, nil
}

// buildCommand is the engine's command that builds the context at dir for
// platform, tagged tag, with the netrc secret among secrets, the one the
// generated Dockerfile mounts (as Build forwards only it).
func buildCommand(engine, platform, tag string, secrets []string, dir string) string {
	args := []string{engine, "build"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, "--tag", tag)
	for _, s := range secrets {
		if parsed, err := manifest.ParseBuildSecret(s); err == nil && parsed.ID == manifest.RuntimeSecretID {
			args = append(args, "--secret", s)
		}
	}
	return strings.Join(append(args, dir), " ")
}
