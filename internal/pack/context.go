package pack

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// writeContext writes the build context of the generated image breq describes
// into req.ContextDir, and reports the commands that build it: the escape
// hatch for an engine, a builder or a CI the CLI does not drive. The context
// is the one the target builds, its lists already resolved, so a plain
// `docker build` of it makes the same image, with nothing from the CLI.
//
// For a project's own Dockerfile built FROM its install base it writes two
// contexts (imagebuild.WriteDeclaredContext), the install base and the
// project, whose Dockerfile names that exact base, and each engine's command
// builds the base and then the project.
func writeContext(req Request, breq imagebuild.Request, name string) (Result, error) {
	declared := breq.UsesInstallBase()
	if breq.FromDeclaredDockerfile() && !declared {
		return Result{}, fmt.Errorf("the project declares its own Dockerfile (%s), so the project is its build context: build it with docker build -f %s %s",
			req.Manifest.Astro.Dockerfile, req.Manifest.Astro.Dockerfile, req.ProjectDir)
	}
	dir, err := filepath.Abs(req.ContextDir)
	if err != nil {
		return Result{}, err
	}
	breq.Platform, breq.Secrets = req.Platform, req.BuildSecrets
	write := imagebuild.WriteContext
	if declared {
		write = imagebuild.WriteDeclaredContext
	}
	if err := write(dir, breq); err != nil {
		if errors.Is(err, imagebuild.ErrContextDirNotEmpty) {
			return Result{}, fmt.Errorf("%w; --context-dir writes only into a new or empty directory", err)
		}
		return Result{}, err
	}
	tag := req.Tag
	if tag == "" {
		tag = fmt.Sprintf("astro-package/%s:latest", name)
	}
	steps := make([]string, 0, 2)
	for _, engine := range []string{"docker", "podman"} {
		if !declared {
			steps = append(steps, buildCommand(engine, req.Platform, tag, netrcOnly(req.BuildSecrets), "", dir))
			continue
		}
		project := filepath.Join(dir, imagebuild.DeclaredProjectDir)
		file := ""
		if rel := req.Manifest.Astro.Dockerfile; rel != "Dockerfile" {
			file = filepath.Join(project, filepath.FromSlash(rel))
		}
		steps = append(steps, buildCommand(engine, req.Platform, breq.InstallBaseTag(), netrcOnly(req.BuildSecrets), "", filepath.Join(dir, imagebuild.DeclaredBaseDir))+
			" && "+buildCommand(engine, req.Platform, tag, req.BuildSecrets, file, project))
	}
	return Result{Target: TargetAstro, Kind: KindContext, ContextDir: dir, NextSteps: steps}, nil
}

// netrcOnly is the netrc secret among secrets, the one an install mounts (as
// Build forwards only it to a build of the CLI's own).
func netrcOnly(secrets []string) []string {
	var out []string
	for _, s := range secrets {
		if parsed, err := manifest.ParseBuildSecret(s); err == nil && parsed.ID == manifest.RuntimeSecretID {
			out = append(out, s)
		}
	}
	return out
}

// buildCommand is the engine's command that builds the context at dir for
// platform, tagged tag, with each of secrets, from the Dockerfile file ("" for
// dir's own). Each argument is quoted for a POSIX shell when it needs it (a
// path with a space), so the line runs as printed.
func buildCommand(engine, platform, tag string, secrets []string, file, dir string) string {
	args := []string{engine, "build"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, "--tag", tag)
	if file != "" {
		args = append(args, "--file", file)
	}
	for _, s := range secrets {
		args = append(args, "--secret", s)
	}
	args = append(args, dir)
	for i, a := range args {
		args[i] = scaffold.ShellQuote(a)
	}
	return strings.Join(args, " ")
}
